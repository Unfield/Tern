package signals

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/http2/hpack"
)

// conn watches what a client sends on a connection: the order of its
// HTTP/1 header lines, or its HTTP/2 SETTINGS, WINDOW_UPDATE, PRIORITY and
// header blocks. net/http keeps neither: headers become a map.
type conn struct {
	net.Conn
	p *Plugin

	mu     sync.Mutex
	hello  *helloInfo
	helloK bool // looked the hello up
	// heads are the request heads seen and not yet claimed by a request,
	// oldest first.
	heads []head
	h2fp  string // the HTTP/2 fingerprint, once the first HEADERS frame is in

	// parser state
	buf  []byte
	mode int // modeDetect, modeH1, modeH2, modeOff
	skip int64
	h2   *h2state
}

const (
	modeDetect = iota
	modeH1
	modeH2
	modeOff
)

// maxHead bounds the bytes kept of one request head or HTTP/2 frame.
const maxHead = 64 << 10

// maxHeads bounds the heads kept unclaimed, such as for requests a
// middleware before this plugin answered.
const maxHeads = 64

// head is one request's head as sent.
type head struct {
	method, target string
	proto          string   // HTTP/1.1, HTTP/1.0 or HTTP/2.0
	names          []string // header names in order, as sent (HTTP/2: lowercase, no pseudo-headers)
	pseudo         string   // HTTP/2's pseudo-header order, "m,a,s,p"
}

var h2Preface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

func (c *conn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.mu.Lock()
		c.feed(b[:n])
		c.mu.Unlock()
	}
	return n, err
}

// NetConn returns the connection this one wraps.
func (c *conn) NetConn() net.Conn { return c.Conn }

func (c *conn) feed(p []byte) {
	for len(p) > 0 && c.mode != modeOff {
		if c.skip > 0 {
			n := int(min(int64(len(p)), c.skip))
			c.skip -= int64(n)
			p = p[n:]
			continue
		}
		switch c.mode {
		case modeDetect:
			c.buf = append(c.buf, p...)
			p = nil
			if len(c.buf) < len(h2Preface) && bytes.HasPrefix(h2Preface, c.buf) {
				return // could still be HTTP/2's preface
			}
			if bytes.HasPrefix(c.buf, h2Preface) {
				c.mode, c.h2 = modeH2, newH2State()
				p, c.buf = c.buf[len(h2Preface):], nil
			} else {
				c.mode = modeH1
				p, c.buf = c.buf, nil
			}
		case modeH1:
			p = c.feedH1(p)
		case modeH2:
			c.feedH2(p)
			p = nil
		}
	}
}

// feedH1 gathers a request head, parses it, and skips its body when its
// length is known. A chunked body or an upgrade ends the watching: the
// heads seen so far remain.
func (c *conn) feedH1(p []byte) []byte {
	old := len(c.buf)
	c.buf = append(c.buf, p...)
	// Search from a little before the new bytes, for a split \r\n\r\n.
	from := max(old-3, 0)
	i := bytes.Index(c.buf[from:], []byte("\r\n\r\n"))
	if i < 0 {
		if len(c.buf) > maxHead {
			c.mode, c.buf = modeOff, nil
		}
		return nil
	}
	end := from + i + 4
	rest := c.buf[end:]
	h, length, ok := parseH1(c.buf[:end])
	c.buf = nil
	if !ok {
		c.mode = modeOff
		return nil
	}
	c.addHead(h)
	switch {
	case length < 0:
		c.mode = modeOff
		return nil
	case length > 0:
		c.skip = length
	}
	// rest is in the old buffer, which isn't reused: safe to return.
	return rest
}

// parseH1 reads a request head. length is the body's, -1 when unknown
// (chunked, or the connection upgrades).
func parseH1(b []byte) (head, int64, bool) {
	lines := strings.Split(strings.TrimSuffix(string(b), "\r\n\r\n"), "\r\n")
	// Browsers may send a stray CRLF before a request.
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 {
		return head{}, 0, false
	}
	parts := strings.Split(lines[0], " ")
	if len(parts) != 3 || !strings.HasPrefix(parts[2], "HTTP/") {
		return head{}, 0, false
	}
	h := head{method: parts[0], target: parts[1], proto: parts[2]}
	var length int64
	for _, l := range lines[1:] {
		name, value, ok := strings.Cut(l, ":")
		if !ok || name == "" {
			continue
		}
		h.names = append(h.names, name)
		value = strings.TrimSpace(value)
		switch strings.ToLower(name) {
		case "content-length":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= 0 {
				length = n
			}
		case "transfer-encoding":
			if !strings.EqualFold(value, "identity") {
				length = -1
			}
		case "upgrade":
			length = -1
		}
	}
	return h, length, true
}

func (c *conn) addHead(h head) {
	if len(c.heads) >= maxHeads {
		c.heads = c.heads[1:]
	}
	c.heads = append(c.heads, h)
}

// claim returns and removes the head of the request with this method and
// target, the oldest if several match.
func (c *conn) claim(method, target string) (head, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, h := range c.heads {
		if h.method == method && h.target == target {
			c.heads = append(c.heads[:i], c.heads[i+1:]...)
			return h, true
		}
	}
	return head{}, false
}

// h2state follows the client's side of an HTTP/2 connection.
type h2state struct {
	dec *hpack.Decoder
	// settings, windowUpdate and priorities make the fingerprint, from the
	// frames before the first HEADERS frame.
	settings     []string
	windowUpdate uint32
	priorities   []string
	done         bool // the fingerprint is complete

	// block gathers a header block across CONTINUATION frames.
	block  []byte
	stream uint32
}

func newH2State() *h2state {
	return &h2state{dec: hpack.NewDecoder(4096, nil)}
}

const (
	frameHeaders      = 0x1
	framePriority     = 0x2
	frameSettings     = 0x4
	frameWindowUpdate = 0x8
	frameContinuation = 0x9

	flagEndHeaders = 0x4
	flagPadded     = 0x8
	flagPriority   = 0x20
	flagAck        = 0x1
)

// feedH2 reads whole frames; the payloads of frames it has no use for,
// such as DATA, are skipped without being kept.
func (c *conn) feedH2(p []byte) {
	c.buf = append(c.buf, p...)
	for {
		if len(c.buf) < 9 {
			return
		}
		length := int(c.buf[0])<<16 | int(c.buf[1])<<8 | int(c.buf[2])
		typ, flags := c.buf[3], c.buf[4]
		stream := binary.BigEndian.Uint32(c.buf[5:9]) & 0x7fffffff
		switch typ {
		case frameHeaders, frameContinuation, frameSettings, frameWindowUpdate, framePriority:
		default:
			// Skip the payload: what is buffered now, and the rest as it
			// comes.
			have := len(c.buf) - 9
			if have >= length {
				c.buf = c.buf[9+length:]
				continue
			}
			c.skip = int64(length - have)
			c.buf = nil
			return
		}
		if length > maxHead {
			c.mode, c.buf = modeOff, nil
			return
		}
		if len(c.buf) < 9+length {
			return
		}
		payload := c.buf[9 : 9+length]
		if !c.h2frame(typ, flags, stream, payload) {
			c.mode, c.buf = modeOff, nil
			return
		}
		c.buf = c.buf[9+length:]
		if len(c.buf) == 0 {
			c.buf = nil
		}
	}
}

// h2frame handles one frame; false gives up on the connection, as on a
// header block that doesn't decode.
func (c *conn) h2frame(typ, flags byte, stream uint32, payload []byte) bool {
	s := c.h2
	switch typ {
	case frameSettings:
		if flags&flagAck != 0 || s.done {
			return true
		}
		for i := 0; i+6 <= len(payload); i += 6 {
			id := binary.BigEndian.Uint16(payload[i:])
			v := binary.BigEndian.Uint32(payload[i+2:])
			s.settings = append(s.settings, strconv.Itoa(int(id))+":"+strconv.FormatUint(uint64(v), 10))
		}
	case frameWindowUpdate:
		if stream == 0 && !s.done && len(payload) >= 4 {
			s.windowUpdate = binary.BigEndian.Uint32(payload) & 0x7fffffff
		}
	case framePriority:
		if !s.done && len(payload) >= 5 {
			dep := binary.BigEndian.Uint32(payload)
			excl := dep >> 31
			s.priorities = append(s.priorities, strconv.FormatUint(uint64(stream), 10)+":"+strconv.FormatUint(uint64(excl), 10)+":"+strconv.FormatUint(uint64(dep&0x7fffffff), 10)+":"+strconv.Itoa(int(payload[4])+1))
		}
	case frameHeaders:
		pad := 0
		if flags&flagPadded != 0 {
			if len(payload) < 1 {
				return false
			}
			pad = int(payload[0])
			payload = payload[1:]
		}
		if flags&flagPriority != 0 {
			if len(payload) < 5 {
				return false
			}
			payload = payload[5:]
		}
		if pad > len(payload) {
			return false
		}
		s.block = append(s.block[:0], payload[:len(payload)-pad]...)
		s.stream = stream
		if flags&flagEndHeaders != 0 {
			return c.h2block()
		}
	case frameContinuation:
		if stream != s.stream {
			return false
		}
		s.block = append(s.block, payload...)
		if len(s.block) > maxHead {
			return false
		}
		if flags&flagEndHeaders != 0 {
			return c.h2block()
		}
	}
	return true
}

// h2block decodes a complete header block. Every block must be decoded,
// in order, to keep HPACK's table as the server's is.
func (c *conn) h2block() bool {
	s := c.h2
	fields, err := s.dec.DecodeFull(s.block)
	s.block = s.block[:0]
	if err != nil {
		return false
	}
	h := head{proto: "HTTP/2.0"}
	var pseudo []string
	for _, f := range fields {
		if strings.HasPrefix(f.Name, ":") {
			switch f.Name {
			case ":method":
				h.method = f.Value
				pseudo = append(pseudo, "m")
			case ":path":
				h.target = f.Value
				pseudo = append(pseudo, "p")
			case ":authority":
				pseudo = append(pseudo, "a")
			case ":scheme":
				pseudo = append(pseudo, "s")
			default:
				pseudo = append(pseudo, strings.TrimPrefix(f.Name, ":"))
			}
			continue
		}
		h.names = append(h.names, f.Name)
	}
	h.pseudo = strings.Join(pseudo, ",")
	if !s.done {
		s.done = true
		prio := "0"
		if len(s.priorities) > 0 {
			prio = strings.Join(s.priorities, ",")
		}
		c.h2fp = strings.Join(s.settings, ";") + "|" + strconv.FormatUint(uint64(s.windowUpdate), 10) + "|" + prio + "|" + h.pseudo
	}
	if h.method != "" { // not trailers
		c.addHead(h)
	}
	return true
}

// CloseWrite and ReadFrom pass through, so http.Server keeps a TCP
// connection's half-close and sendfile.
func (c *conn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// ReadFrom implements io.ReaderFrom.
func (c *conn) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{c.Conn}, r)
}
