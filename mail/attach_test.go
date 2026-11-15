package mail

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// part is a decoded leaf of a MIME tree, with the path of multipart types above it.
type part struct {
	path        string
	contentType string
	disposition string
	filename    string
	contentID   string
	body        []byte
}

func walk(t *testing.T, contentType string, body io.Reader, path string) []part {
	t.Helper()
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(mt, "multipart/") {
		t.Fatalf("not multipart: %s", mt)
	}
	path += "/" + strings.TrimPrefix(mt, "multipart/")
	r := multipart.NewReader(body, params["boundary"])
	var out []part
	for {
		p, err := r.NextRawPart()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		ct := p.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/") {
			out = append(out, walk(t, ct, p, path)...)
			continue
		}
		data, _ := io.ReadAll(p)
		if p.Header.Get("Content-Transfer-Encoding") == "base64" {
			for _, line := range strings.Split(string(data), "\r\n") {
				if len(line) > 76 {
					t.Errorf("a base64 line is %d characters", len(line))
				}
			}
			data, err = base64.StdEncoding.DecodeString(strings.ReplaceAll(string(data), "\r\n", ""))
			if err != nil {
				t.Fatal(err)
			}
		}
		disp, dparams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		ctype, _, _ := mime.ParseMediaType(ct)
		out = append(out, part{path, ctype, disp, dparams["filename"], p.Header.Get("Content-ID"), data})
	}
}

func TestAttachments(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0, 1, 2, 250}, 100)...)
	png := []byte("\x89PNG\r\n\x1a\n rest of the image")
	msg := &Message{From: "hello@acme.test", To: []string{"ada@example.test"}, Subject: "Invoice", Text: "See attached.", HTML: `<p>See attached.</p>`}
	msg.Attach("../../etc/Rechnung März.pdf", pdf)
	cid := msg.Inline("chart.png", png)
	msg.HTML += `<img src="cid:` + cid + `">`
	msg.Attachments = append(msg.Attachments, Attachment{Filename: "notes\r\nBcc: evil@evil.test", Data: []byte("plain notes")})

	raw, _, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "\r\nBcc:") {
		t.Fatal("an attachment's name injected a header")
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	parts := walk(t, m.Header.Get("Content-Type"), m.Body, "")
	want := []struct{ path, ctype, disp, filename string }{
		{"/mixed/related/alternative", "text/plain", "", ""},
		{"/mixed/related/alternative", "text/html", "", ""},
		{"/mixed/related", "image/png", "inline", "chart.png"},
		{"/mixed", "application/pdf", "attachment", "Rechnung März.pdf"},
		{"/mixed", "text/plain", "attachment", "notesBcc: evil@evil.test"},
	}
	if len(parts) != len(want) {
		t.Fatalf("got %d parts, want %d: %+v", len(parts), len(want), parts)
	}
	for i, w := range want {
		p := parts[i]
		if p.path != w.path || p.contentType != w.ctype || p.disposition != w.disp || p.filename != w.filename {
			t.Errorf("part %d = %s %s %s %q, want %s %s %s %q", i, p.path, p.contentType, p.disposition, p.filename, w.path, w.ctype, w.disp, w.filename)
		}
	}
	if !bytes.Equal(parts[3].body, pdf) || !bytes.Equal(parts[2].body, png) {
		t.Error("an attachment's contents changed")
	}
	if parts[2].contentID != "<"+cid+">" {
		t.Errorf("Content-ID = %q, want <%s>", parts[2].contentID, cid)
	}
}

func TestAttachmentOnlyFiles(t *testing.T) {
	msg := &Message{From: "hello@acme.test", To: []string{"ada@example.test"}, Text: "Hi"}
	msg.Attach("data.json", []byte(`{"a":1}`))
	raw, _, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := mail.ReadMessage(bytes.NewReader(raw))
	parts := walk(t, m.Header.Get("Content-Type"), m.Body, "")
	if len(parts) != 2 || parts[0].path != "/mixed" || parts[0].contentType != "text/plain" || parts[1].contentType != "application/json" {
		t.Errorf("parts = %+v", parts)
	}
}

func TestAttachmentChecks(t *testing.T) {
	base := func() *Message {
		return &Message{From: "hello@acme.test", To: []string{"ada@example.test"}, Text: "Hi"}
	}
	for name, at := range map[string]Attachment{
		"no name":        {Filename: "", Data: []byte("x")},
		"bad type":       {Filename: "a.bin", ContentType: "not a type", Data: []byte("x")},
		"bad content ID": {Filename: "a.png", ContentID: "a>b", Data: []byte("x")},
	} {
		msg := base()
		msg.Attachments = []Attachment{at}
		if _, _, err := Encode(msg); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	msg := base()
	msg.Attach("big.bin", make([]byte, MaxSize))
	if _, _, err := Encode(msg); err == nil || !strings.Contains(err.Error(), "MB") {
		t.Errorf("a message over MaxSize: %v", err)
	}
}

// Messages go through the jobs queue as JSON, attachments included.
func TestAttachmentJSON(t *testing.T) {
	msg := Message{To: []string{"a@b.test"}, Text: "x"}
	msg.Attach("a.pdf", []byte{0, 1, 2})
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var back Message
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Attachments) != 1 || !bytes.Equal(back.Attachments[0].Data, []byte{0, 1, 2}) {
		t.Errorf("round trip = %+v", back.Attachments)
	}
}
