package mail

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	raw, rcpts, err := Encode(&Message{
		From:    "Acme <hello@acme.test>",
		To:      []string{"Ada Lovelace <ada@example.test>"},
		Cc:      []string{"cc@example.test"},
		Bcc:     []string{"secret@example.test"},
		ReplyTo: "support@acme.test",
		Subject: "Grüße\r\nBcc: injected@evil.test",
		HTML:    "<p>Héllo <b>Ada</b></p>",
		Text:    "Héllo Ada",
		Headers: map[string]string{"List-Unsubscribe": "<https://acme.test/unsubscribe>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rcpts, ",") != "ada@example.test,cc@example.test,secret@example.test" {
		t.Errorf("recipients = %v", rcpts)
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Get("Bcc") != "" || strings.Contains(string(raw), "secret@example.test") {
		t.Error("the Bcc recipient is visible in the message")
	}
	subject, _ := (&mime.WordDecoder{}).DecodeHeader(m.Header.Get("Subject"))
	if subject != "Grüße  Bcc: injected@evil.test" {
		t.Errorf("subject = %q (a newline must not start a new header)", subject)
	}
	if m.Header.Get("Reply-To") != "<support@acme.test>" || m.Header.Get("List-Unsubscribe") == "" || !strings.HasSuffix(m.Header.Get("Message-ID"), "@acme.test>") {
		t.Errorf("headers = %v", m.Header)
	}
	mediaType, params, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if mediaType != "multipart/alternative" {
		t.Fatalf("content type = %s", mediaType)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var parts []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(p) // multipart decodes quoted-printable
		parts = append(parts, p.Header.Get("Content-Type")+": "+string(body))
	}
	if len(parts) != 2 || parts[0] != "text/plain; charset=utf-8: Héllo Ada" || parts[1] != "text/html; charset=utf-8: <p>Héllo <b>Ada</b></p>" {
		t.Errorf("parts = %q", parts)
	}

	if _, _, err := Encode(&Message{From: "a@b.test", Subject: "x", Text: "y"}); err == nil {
		t.Error("a message without recipients was encoded")
	}
	if _, _, err := Encode(&Message{From: "a@b.test", To: []string{"not an address"}, Text: "y"}); err == nil {
		t.Error("an invalid address was accepted")
	}
}

// fakeSMTP accepts one message and returns what it received.
func fakeSMTP(t *testing.T, startTLS bool) (port int, got chan string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		var log strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			log.WriteString(strings.TrimSpace(line) + "\n")
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				if startTLS {
					say("250-fake")
					say("250 STARTTLS")
				} else {
					say("250-fake")
					say("250 AUTH PLAIN")
				}
			case strings.HasPrefix(cmd, "AUTH"), strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				if strings.HasPrefix(cmd, "AUTH") {
					say("235 ok")
				} else {
					say("250 ok")
				}
			case cmd == "DATA":
				say("354 go ahead")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					log.WriteString(l)
				}
				say("250 queued")
			case cmd == "QUIT":
				say("221 bye")
				got <- log.String()
				return
			default:
				say("502 no")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, got
}

func TestSMTP(t *testing.T) {
	port, got := fakeSMTP(t, false)
	m := New(SMTP{Host: "127.0.0.1", Port: port, Username: "u", Password: "p", Insecure: true}, "Acme <hello@acme.test>")
	err := m.Send(context.Background(), &Message{To: []string{"ada@example.test"}, Subject: "Hi", Text: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	session := <-got
	for _, want := range []string{"MAIL FROM:<hello@acme.test>", "RCPT TO:<ada@example.test>", "Subject: Hi", "Hello"} {
		if !strings.Contains(session, want) {
			t.Errorf("missing %q in\n%s", want, session)
		}
	}
	// PLAIN auth over an unencrypted connection is refused by net/smtp unless
	// the server is local, which this one is.
	if !strings.Contains(session, "AUTH PLAIN") {
		t.Errorf("no AUTH in\n%s", session)
	}
}

func TestSMTPRequiresTLS(t *testing.T) {
	port, _ := fakeSMTP(t, false)
	m := New(SMTP{Host: "127.0.0.1", Port: port}, "hello@acme.test")
	err := m.Send(context.Background(), &Message{To: []string{"ada@example.test"}, Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("sending without TLS: %v", err)
	}
}

func TestLog(t *testing.T) {
	l := &Log{}
	m := New(l, "hello@acme.test")
	if err := m.Send(context.Background(), &Message{To: []string{"a@b.test"}, Subject: "Hi", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if s := l.Sent(); len(s) != 1 || s[0].To[0] != "a@b.test" || !bytes.Contains(s[0].Raw, []byte("Subject: Hi")) {
		t.Errorf("sent = %+v", s)
	}
}
