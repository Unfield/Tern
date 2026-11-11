package signals

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"html"
	"io"
	"math/bits"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/kit"
)

// The client check: the page's script gets a challenge, finds a nonce
// whose SHA-256 with it starts with Difficulty zero bits, and sends it
// with what it noticed of the browser. A solution is good once, from the
// address the challenge was given to, within five minutes. The result is
// a signed cookie, good for a day.

const (
	checkPrefix   = "/__tern/signals/"
	checkCookie   = "tern_check"
	checkedCookie = "tern_checked" // readable by the script, so it knows not to check again
	challengeFor  = 5 * time.Minute
	checkFor      = 24 * time.Hour
)

//go:embed check.js
var checkJS string

func (p *Plugin) setupCheck(s *kit.Setup) {
	s.Handle("GET "+checkPrefix+"check.js", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = io.WriteString(w, checkJS)
	}))
	s.Handle("GET "+checkPrefix+"challenge", http.HandlerFunc(p.challenge))
	s.Handle("POST "+checkPrefix+"check", http.HandlerFunc(p.verify))
	s.HeadFunc(func(c *kit.Ctx, nonce string) string {
		if _, ok := c.Cookie(checkedCookie); ok {
			return ""
		}
		attr := ""
		if nonce != "" {
			attr = ` nonce="` + html.EscapeString(nonce) + `"`
		}
		return `<script src="` + html.EscapeString(c.Path(checkPrefix+"check.js")) + `" data-base="` + html.EscapeString(c.Path(checkPrefix)) + `" defer` + attr + `></script>`
	})
}

func (p *Plugin) sign(parts ...string) string {
	m := hmac.New(sha256.New, p.secret)
	for _, s := range parts {
		_, _ = io.WriteString(m, s)
		_, _ = m.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:16])
}

// challenge is random bytes and an expiry, signed for the client's address.
func (p *Plugin) challenge(w http.ResponseWriter, r *http.Request) {
	var b [24]byte
	_, _ = rand.Read(b[:16])
	binary.BigEndian.PutUint64(b[16:], uint64(time.Now().Add(challengeFor).Unix()))
	body := base64.RawURLEncoding.EncodeToString(b[:])
	c := body + "." + p.sign("challenge", body, kit.ClientIP(r))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"challenge": c, "difficulty": p.o.Difficulty})
}

// checkReport is what the script sends.
type checkReport struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
	// What the script noticed: navigator.webdriver, a desktop Chrome
	// without window.chrome, no languages. (A window of no size isn't
	// one: embedded browsers have it.)
	Webdriver bool `json:"webdriver"`
	NoChrome  bool `json:"noChrome"`
	NoLang    bool `json:"noLanguages"`
}

func (p *Plugin) verify(w http.ResponseWriter, r *http.Request) {
	var rep checkReport
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&rep); err != nil {
		http.Error(w, "bad report", http.StatusBadRequest)
		return
	}
	body, sig, ok := strings.Cut(rep.Challenge, ".")
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if !ok || err != nil || len(raw) != 24 || !hmac.Equal([]byte(sig), []byte(p.sign("challenge", body, kit.ClientIP(r)))) {
		http.Error(w, "bad challenge", http.StatusForbidden)
		return
	}
	exp := time.Unix(int64(binary.BigEndian.Uint64(raw[16:])), 0)
	if time.Now().After(exp) {
		http.Error(w, "challenge expired", http.StatusForbidden)
		return
	}
	if len(rep.Nonce) > 32 || zeroBits(sha256.Sum256([]byte(rep.Challenge+":"+rep.Nonce))) < p.o.Difficulty {
		http.Error(w, "wrong solution", http.StatusForbidden)
		return
	}
	if !p.redeem(body, exp) {
		http.Error(w, "challenge used", http.StatusForbidden)
		return
	}
	var flags []string
	for _, f := range []struct {
		on   bool
		name string
	}{{rep.Webdriver, "webdriver"}, {rep.NoChrome, "no-window-chrome"}, {rep.NoLang, "no-languages"}} {
		if f.on {
			flags = append(flags, f.name)
		}
	}
	until := time.Now().Add(checkFor)
	value := strconv.FormatInt(until.Unix(), 10) + "." + strings.Join(flags, ",")
	value += "." + p.sign("check", value)
	secure := kit.IsHTTPS(r)
	http.SetCookie(w, &http.Cookie{Name: checkCookie, Value: value, Path: "/", Expires: until, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: checkedCookie, Value: "1", Path: "/", Expires: until, Secure: secure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

// redeem marks a challenge used; false if it was.
func (p *Plugin) redeem(body string, exp time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if len(p.used) > 1024 {
		for k, e := range p.used {
			if now.After(e) {
				delete(p.used, k)
			}
		}
	}
	if _, ok := p.used[body]; ok {
		return false
	}
	p.used[body] = exp
	return true
}

// checked reads the check cookie: whether it is valid and clean, and the
// reasons for what the script noticed.
func (p *Plugin) checked(r *http.Request) (bool, []string) {
	ck, err := r.Cookie(checkCookie)
	if err != nil {
		return false, nil
	}
	i := strings.LastIndexByte(ck.Value, '.')
	if i < 0 || !hmac.Equal([]byte(ck.Value[i+1:]), []byte(p.sign("check", ck.Value[:i]))) {
		return false, nil
	}
	until, flags, _ := strings.Cut(ck.Value[:i], ".")
	t, err := strconv.ParseInt(until, 10, 64)
	if err != nil || time.Now().Unix() > t {
		return false, nil
	}
	if flags == "" {
		return true, nil
	}
	var reasons []string
	for _, f := range strings.Split(flags, ",") {
		reasons = append(reasons, "client:"+f)
	}
	return false, reasons
}

func zeroBits(sum [32]byte) int {
	n := 0
	for _, b := range sum {
		if b != 0 {
			return n + bits.LeadingZeros8(b)
		}
		n += 8
	}
	return n
}
