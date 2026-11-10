package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Signer makes links to private files that expire.
type Signer struct{ key []byte }

// NewSigner returns a signer; secret should be at least 32 bytes.
func NewSigner(secret string) *Signer {
	sum := sha256.Sum256([]byte("tern/storage-url\x00" + secret))
	return &Signer{key: sum[:]}
}

func (s *Signer) mac(key string, exp int64) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(key + "\n" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// URL returns a link to key under the path prefix Serve uses, valid for ttl.
func (s *Signer) URL(prefix, key string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	return strings.TrimRight(prefix, "/") + "/" + uriEncode(key, false) + "?exp=" + strconv.FormatInt(exp, 10) + "&sig=" + s.mac(key, exp)
}

// Valid reports whether a link's exp and sig parameters are valid for key.
func (s *Signer) Valid(key string, q url.Values) bool {
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(q.Get("sig")), []byte(s.mac(key, exp)))
}

// ServeOptions configure Serve.
type ServeOptions struct {
	// Signer, if set, makes every file private: only links from
	// Signer.URL work. Without it, files are public.
	Signer *Signer
	// CacheControl is sent with files; the default is
	// "private, max-age=3600" for signed files and "public, max-age=3600"
	// otherwise.
	CacheControl string
	// Inline lists content types shown in the browser rather than
	// downloaded. The default is images (except SVG), video, audio, PDF and
	// plain text; everything else is sent as an attachment, so uploaded
	// HTML or SVG can't run scripts on your site.
	Inline []string
}

var defaultInline = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "video/*", "audio/*", "application/pdf", "text/plain"}

// Serve returns a plugin that serves bucket's files under prefix, such as
// "/files/". Files are sent with nosniff and a sandboxing Content Security
// Policy; disk files support range requests, and buckets that can presign
// (S3) are redirected to directly.
func Serve(prefix string, b Bucket, opts ServeOptions) kit.Plugin {
	prefix = "/" + strings.Trim(prefix, "/") + "/"
	if opts.Inline == nil {
		opts.Inline = defaultInline
	}
	return &server{prefix: prefix, b: b, opts: opts}
}

type server struct {
	prefix string
	b      Bucket
	opts   ServeOptions
}

func (s *server) Name() string { return "storage" }

func (s *server) Setup(set *kit.Setup) error {
	set.Handle("GET "+s.prefix, s)
	set.Handle("HEAD "+s.prefix, s)
	return nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, s.prefix)
	if ValidKey(key) != nil {
		http.NotFound(w, r)
		return
	}
	if s.opts.Signer != nil && !s.opts.Signer.Valid(key, r.URL.Query()) {
		http.Error(w, "This link is invalid or has expired.", http.StatusForbidden)
		return
	}
	if p, ok := s.b.(Presigner); ok {
		ttl := 15 * time.Minute
		u, err := p.PresignGet(key, ttl)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=600")
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	rc, obj, err := s.b.Get(r.Context(), key)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Could not read the file", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()
	h := w.Header()
	h.Set("Content-Type", obj.ContentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; sandbox")
	cc := s.opts.CacheControl
	if cc == "" {
		cc = "public, max-age=3600"
		if s.opts.Signer != nil {
			cc = "private, max-age=3600"
		}
	}
	h.Set("Cache-Control", cc)
	disposition := "attachment"
	if inline(obj.ContentType, s.opts.Inline) {
		disposition = "inline"
	}
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": lastSegment(key)}))
	if rs, ok := rc.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "", obj.Modified, rs)
		return
	}
	h.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, rc)
}

func inline(ct string, allowed []string) bool {
	mt, _, _ := mime.ParseMediaType(ct)
	for _, a := range allowed {
		if a == mt || (strings.HasSuffix(a, "/*") && strings.HasPrefix(mt, strings.TrimSuffix(a, "*"))) {
			return true
		}
	}
	return false
}

func lastSegment(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}
