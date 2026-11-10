package images_test

import (
	"bytes"
	"crypto/tls"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/images"
)

func origin(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	opaque := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for x := range 2000 {
		opaque.Set(x, 500, color.RGBA{R: uint8(x), A: 255})
	}
	for y := range 1000 {
		for x := range 2000 {
			if y != 500 {
				opaque.Set(x, y, color.RGBA{B: 200, A: 255})
			}
		}
	}
	var jpg, alpha bytes.Buffer
	_ = jpeg.Encode(&jpg, opaque, nil)
	clear := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	_ = png.Encode(&alpha, clear)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/photo.jpg":
			_, _ = w.Write(jpg.Bytes())
		case "/logo.png":
			_, _ = w.Write(alpha.Bytes())
		case "/bomb.png":
			// A tiny file claiming a 100000x100000 image.
			var b bytes.Buffer
			_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)))
			raw := b.Bytes()
			copy(raw[16:24], []byte{0, 1, 0x86, 0xa0, 0, 1, 0x86, 0xa0})
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestRemoteImages(t *testing.T) {
	srv, hits := origin(t)
	u, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // the test server's certificate
	a, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`},
		kit.WithPlugins(images.New(images.Options{Hosts: []string{u.Hostname()}, CacheDir: t.TempDir(), HTTPClient: client})))
	if err != nil {
		t.Fatal(err)
	}
	get := func(src string, w int) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, images.Path+"?url="+url.QueryEscape(src)+"&w="+itoa(w), nil))
		return rec
	}

	rec := get(srv.URL+"/photo.jpg", 640)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("resize: %d %v %s", rec.Code, rec.Header(), rec.Body.String()[:min(80, rec.Body.Len())])
	}
	img, _, err := image.Decode(rec.Body)
	if err != nil || img.Bounds().Dx() != 640 || img.Bounds().Dy() != 320 {
		t.Errorf("resized to %v, %v", img.Bounds(), err)
	}
	before := hits.Load()
	if rec := get(srv.URL+"/photo.jpg", 640); rec.Code != 200 || hits.Load() != before {
		t.Errorf("the second request was not served from the cache: %d, %d fetches", rec.Code, hits.Load()-before)
	}
	// Transparent images stay PNG, and are never enlarged.
	rec = get(srv.URL+"/logo.png", 1080)
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("transparent image: %v", rec.Header())
	}
	if img, _, _ := image.Decode(rec.Body); img == nil || img.Bounds().Dx() != 400 {
		t.Errorf("a small image was enlarged")
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"another host": get("https://evil.example/x.jpg", 640),
		"plain http":   get(strings.Replace(srv.URL, "https", "http", 1)+"/photo.jpg", 640),
		"odd width":    get(srv.URL+"/photo.jpg", 641),
		"credentials":  get("https://user:pw@"+u.Host+"/photo.jpg", 640),
		"missing":      get(srv.URL+"/nope.jpg", 640),
		"pixel bomb":   get(srv.URL+"/bomb.png", 640),
	} {
		if rec.Code == 200 {
			t.Errorf("%s: served", name)
		}
	}
}

func itoa(n int) string {
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
