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
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
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
	part := func(contentType, body string) {
		fmt.Fprintf(&b, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", contentType)
		w := quotedprintable.NewWriter(&b)
		_, _ = w.Write([]byte(body))
		_ = w.Close()
		b.WriteString("\r\n")
	}
	switch {
	case msg.HTML != "" && msg.Text != "":
		boundary := "tern-" + randomID()
		header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		part("text/plain", msg.Text)
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		part("text/html", msg.HTML)
		fmt.Fprintf(&b, "--%s--\r\n", boundary)
	case msg.HTML != "":
		part("text/html", msg.HTML)
	default:
		part("text/plain", msg.Text)
	}
	return b.Bytes(), rcpts, nil
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
