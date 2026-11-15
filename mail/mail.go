// Package mail sends email, written as Svelte components in src/emails:
//
//	mailer := mail.New(mail.SMTP{Host: "smtp.example.com", Port: 587, Username: user, Password: pass}, "Acme <hello@acme.com>")
//
//	msg, err := mailer.Template(ctx, app, "Welcome", map[string]any{"name": u.Name, "link": confirmURL})
//	msg.To = []string{u.Email}
//	err = mailer.Send(ctx, msg)
//
// Under tern dev, use mail.Log to print messages instead of sending them.
// To send in the background with retries, use Job with a jobs queue:
//
//	sendMail := mail.Job(q, mailer)
//	_, err = sendMail.Enqueue(ctx, *msg)
//
// Attach files with Attach, or show an image in the HTML with Inline:
//
//	msg.Attach("invoice-1042.pdf", pdf)
//	cid := msg.Inline("chart.png", png) // <img src="cid:...">
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Message is an email.
type Message struct {
	From    string   `json:"from,omitempty"` // the Mailer's default when empty
	To      []string `json:"to"`
	Cc      []string `json:"cc,omitempty"`
	Bcc     []string `json:"bcc,omitempty"`
	ReplyTo string   `json:"replyTo,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	// Headers are extra headers, such as List-Unsubscribe.
	Headers map[string]string `json:"headers,omitempty"`
	// Attachments are files sent with the message: see Attach and Inline.
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment is a file sent with a message.
type Attachment struct {
	// Filename is the name the recipient sees; any directories are dropped.
	Filename string `json:"filename"`
	// ContentType, such as application/pdf, is guessed from the name and
	// the contents when empty.
	ContentType string `json:"contentType,omitempty"`
	Data        []byte `json:"data"`
	// ContentID makes it an inline part, for an image the HTML shows with
	// <img src="cid:ContentID">, instead of a file in the attachment list.
	ContentID string `json:"contentId,omitempty"`
}

// MaxSize is the largest message Encode builds, attachments included:
// 25 MB, which most mail providers accept. Files are a third larger once
// encoded, so the attachments of a message can total about 18 MB.
const MaxSize = 25 << 20

// Attach adds a file to the message.
func (m *Message) Attach(filename string, data []byte) {
	m.Attachments = append(m.Attachments, Attachment{Filename: filename, Data: data})
}

// Inline adds an image for the HTML to show, and returns the content ID
// to use in it: <img src="cid:{{id}}">.
func (m *Message) Inline(filename string, data []byte) string {
	id := randomID() + "@tern"
	m.Attachments = append(m.Attachments, Attachment{Filename: filename, Data: data, ContentID: id})
	return id
}

// Transport delivers messages: SMTP, Log, or your own for an email API.
type Transport interface {
	Send(ctx context.Context, from string, to []string, raw []byte) error
}

// Mailer builds and sends messages.
type Mailer struct {
	transport Transport
	from      string
}

// New returns a mailer that sends with t from the default address from.
func New(t Transport, from string) *Mailer { return &Mailer{transport: t, from: from} }

// Template renders src/emails/<name>.svelte with props into a message,
// with the template's subject and HTML and a plain-text version.
func (m *Mailer) Template(ctx context.Context, app *kit.App, name string, props any) (*Message, error) {
	e, err := app.RenderEmail(ctx, name, props)
	if err != nil {
		return nil, err
	}
	return &Message{Subject: e.Subject, HTML: e.HTML, Text: e.Text}, nil
}

// Send delivers msg.
func (m *Mailer) Send(ctx context.Context, msg *Message) error {
	if msg.From == "" {
		msg.From = m.from
	}
	raw, rcpts, err := Encode(msg)
	if err != nil {
		return err
	}
	from, err := mail.ParseAddress(msg.From)
	if err != nil {
		return fmt.Errorf("mail: From: %w", err)
	}
	return m.transport.Send(ctx, from.Address, rcpts, raw)
}

// Encode builds the MIME message and returns it with every recipient.
func Encode(msg *Message) ([]byte, []string, error) {
	if len(msg.To)+len(msg.Cc)+len(msg.Bcc) == 0 {
		return nil, nil, errors.New("mail: no recipients")
	}
	if msg.HTML == "" && msg.Text == "" {
		return nil, nil, errors.New("mail: the message has no body")
	}
	var rcpts []string
	addrs := func(field string, list []string) (string, error) {
		var out []string
		for _, a := range list {
			p, err := mail.ParseAddress(a)
			if err != nil {
				return "", fmt.Errorf("mail: %s %q: %w", field, a, err)
			}
			rcpts = append(rcpts, p.Address)
			out = append(out, p.String())
		}
		return strings.Join(out, ", "), nil
	}
	from, err := mail.ParseAddress(msg.From)
	if err != nil {
		return nil, nil, fmt.Errorf("mail: From %q: %w", msg.From, err)
	}
	var b bytes.Buffer
	header := func(k, v string) {
		if strings.ContainsAny(v, "\r\n") {
			v = strings.NewReplacer("\r", " ", "\n", " ").Replace(v) // no header injection
		}
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	header("From", from.String())
	for _, f := range []struct {
		name string
		list []string
	}{{"To", msg.To}, {"Cc", msg.Cc}} {
		if len(f.list) > 0 {
			v, err := addrs(f.name, f.list)
			if err != nil {
				return nil, nil, err
			}
			header(f.name, v)
		}
	}
	if _, err := addrs("Bcc", msg.Bcc); err != nil { // recipients only, never a header
		return nil, nil, err
	}
	if msg.ReplyTo != "" {
		p, err := mail.ParseAddress(msg.ReplyTo)
		if err != nil {
			return nil, nil, fmt.Errorf("mail: Reply-To: %w", err)
		}
		header("Reply-To", p.String())
	}
	header("Subject", mime.QEncoding.Encode("utf-8", strings.NewReplacer("\r", " ", "\n", " ").Replace(msg.Subject)))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", "<"+randomID()+"@"+domain(from.Address)+">")
	header("MIME-Version", "1.0")
	for k, v := range msg.Headers {
		header(k, v)
	}
	var inline, files []Attachment
	for i := range msg.Attachments {
		at, err := checkAttachment(msg.Attachments[i])
		if err != nil {
			return nil, nil, err
		}
		if at.ContentID != "" {
			inline = append(inline, at)
		} else {
			files = append(files, at)
		}
	}

	// The body, nested as mail clients expect:
	// mixed(related(alternative(text, html), inline images), files).
	text := func(w *bytes.Buffer, contentType, body string) {
		fmt.Fprintf(w, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", contentType)
		q := quotedprintable.NewWriter(w)
		_, _ = q.Write([]byte(body))
		_ = q.Close()
		w.WriteString("\r\n")
	}
	multipart := func(w *bytes.Buffer, kind string, parts ...func(*bytes.Buffer)) {
		boundary := "tern-" + randomID()
		fmt.Fprintf(w, "Content-Type: multipart/%s; boundary=\"%s\"\r\n\r\n", kind, boundary)
		for _, p := range parts {
			fmt.Fprintf(w, "--%s\r\n", boundary)
			p(w)
		}
		fmt.Fprintf(w, "--%s--\r\n", boundary)
	}
	alternative := func(w *bytes.Buffer) {
		switch {
		case msg.HTML != "" && msg.Text != "":
			multipart(w, "alternative",
				func(w *bytes.Buffer) { text(w, "text/plain", msg.Text) },
				func(w *bytes.Buffer) { text(w, "text/html", msg.HTML) })
		case msg.HTML != "":
			text(w, "text/html", msg.HTML)
		default:
			text(w, "text/plain", msg.Text)
		}
	}
	related := alternative
	if len(inline) > 0 {
		parts := []func(*bytes.Buffer){alternative}
		for _, at := range inline {
			parts = append(parts, func(w *bytes.Buffer) { attachment(w, at) })
		}
		related = func(w *bytes.Buffer) { multipart(w, "related", parts...) }
	}
	if len(files) > 0 {
		parts := []func(*bytes.Buffer){related}
		for _, at := range files {
			parts = append(parts, func(w *bytes.Buffer) { attachment(w, at) })
		}
		multipart(&b, "mixed", parts...)
	} else {
		related(&b)
	}
	if b.Len() > MaxSize {
		return nil, nil, fmt.Errorf("mail: the message is %d MB, over the %d MB most providers accept; link to large files instead of attaching them", b.Len()>>20, MaxSize>>20)
	}
	return b.Bytes(), rcpts, nil
}

// checkAttachment cleans an attachment's name and checks its type.
func checkAttachment(at Attachment) (Attachment, error) {
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1 // no header injection
		}
		return r
	}, at.Filename)
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return at, fmt.Errorf("mail: an attachment has no filename")
	}
	at.Filename = name
	if at.ContentType == "" {
		at.ContentType = mime.TypeByExtension(path.Ext(name))
		if at.ContentType == "" {
			at.ContentType = http.DetectContentType(at.Data)
		}
	}
	mt, params, err := mime.ParseMediaType(at.ContentType)
	if err != nil {
		return at, fmt.Errorf("mail: attachment %s: content type %q: %w", name, at.ContentType, err)
	}
	at.ContentType = mime.FormatMediaType(mt, params)
	if strings.ContainsAny(at.ContentID, "<>\r\n \t") {
		return at, fmt.Errorf("mail: attachment %s: content ID %q has characters it can't have", name, at.ContentID)
	}
	return at, nil
}

// attachment writes a base64-encoded part, in lines of 76 characters.
func attachment(w *bytes.Buffer, at Attachment) {
	disposition := "attachment"
	if at.ContentID != "" {
		disposition = "inline"
	}
	fmt.Fprintf(w, "Content-Type: %s\r\n", mime.FormatMediaType(mediaType(at.ContentType), mergeParams(at.ContentType, map[string]string{"name": at.Filename})))
	fmt.Fprintf(w, "Content-Disposition: %s\r\n", mime.FormatMediaType(disposition, map[string]string{"filename": at.Filename}))
	if at.ContentID != "" {
		fmt.Fprintf(w, "Content-ID: <%s>\r\n", at.ContentID)
	}
	w.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString(at.Data)
	for len(enc) > 76 {
		w.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	if enc != "" {
		w.WriteString(enc + "\r\n")
	}
}

func mediaType(contentType string) string {
	mt, _, _ := mime.ParseMediaType(contentType)
	return mt
}

func mergeParams(contentType string, extra map[string]string) map[string]string {
	_, params, _ := mime.ParseMediaType(contentType)
	if params == nil {
		params = map[string]string{}
	}
	for k, v := range extra {
		params[k] = v
	}
	return params
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func domain(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

// SMTP sends through an SMTP server. Port 465 uses implicit TLS; other
// ports upgrade with STARTTLS when the server offers it, which is required
// unless Insecure is set (for local test servers).
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	// Insecure allows sending without TLS, such as to a local Mailpit.
	Insecure bool
	// Timeout bounds a delivery; the default is 30 seconds.
	Timeout time.Duration
}

// Send implements Transport.
func (s SMTP) Send(ctx context.Context, from string, to []string, raw []byte) error {
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	d := net.Dialer{}
	var conn net.Conn
	var err error
	if s.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connecting to %s: %w", addr, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: %w", err)
	}
	defer func() { _ = c.Close() }()
	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("mail: STARTTLS: %w", err)
			}
		} else if !s.Insecure {
			return fmt.Errorf("mail: %s does not offer STARTTLS; set Insecure to send without encryption", addr)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: authenticating: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("mail: RCPT TO %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("mail: writing the message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: sending: %w", err)
	}
	return c.Quit()
}

// Log prints messages instead of sending them, for development. It keeps
// the last 50 for tests to inspect.
type Log struct {
	Logger *slog.Logger
	mu     sync.Mutex
	sent   []Sent
}

// Sent is a message the Log transport received.
type Sent struct {
	From string
	To   []string
	Raw  []byte
}

// Send implements Transport.
func (l *Log) Send(_ context.Context, from string, to []string, raw []byte) error {
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	subject := ""
	if m, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		subject, _ = (&mime.WordDecoder{}).DecodeHeader(m.Header.Get("Subject"))
	}
	logger.Info("mail: not sent (mail.Log)", "from", from, "to", strings.Join(to, ", "), "subject", subject, "bytes", len(raw))
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, Sent{From: from, To: to, Raw: raw})
	if len(l.sent) > 50 {
		l.sent = l.sent[1:]
	}
	return nil
}

// Sent returns the messages received so far.
func (l *Log) Sent() []Sent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Sent(nil), l.sent...)
}
