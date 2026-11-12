// Package images resizes remote images for <Image src="https://…">:
//
//	kit.WithPlugins(images.New(images.Options{Hosts: []string{"images.unsplash.com", "*.cdn.example.com"}}))
//
//	<Image src="https://images.unsplash.com/photo-123" width={800} height={600} alt="" />
//
// <Image> asks /_tern/image?url=…&w=… for each width in its srcset. Only
// HTTPS images on the listed hosts are fetched, at most 20 MB and 50
// megapixels; results are cached on disk. Images are re-encoded as JPEG,
// or PNG when they have transparency. (Imported images, with ?image, also
// get AVIF and WebP; Go has no pure-Go encoders for those.)
package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // decoders
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // decoder

	"golang.org/x/sync/singleflight"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/safehttp"
)

// Widths are the widths <Image> requests; others are refused, which keeps
// the cache from filling with every possible size.
var Widths = []int{64, 128, 256, 384, 640, 750, 828, 1080, 1200, 1920, 2048, 3840}

// Path is where the plugin serves resized images.
const Path = "/_tern/image"

// Options configure the plugin.
type Options struct {
	// Hosts lists the hosts images may come from, such as
	// "images.unsplash.com" or "*.example.com" for its subdomains.
	Hosts []string
	// CacheDir keeps resized images; the default is a directory in the
	// system's temporary directory.
	CacheDir string
	// Quality is the JPEG quality; the default is 80.
	Quality int
	// MaxAge is how long browsers cache resized images; the default is a day.
	MaxAge time.Duration
	// HTTPClient fetches the originals. The default, from safehttp, only
	// connects to public addresses, so a listed host that resolves to the
	// app's own network, or redirects there, is refused; pass a client of
	// your own for originals on a private network.
	HTTPClient *http.Client
}

// Plugin is the images plugin.
type Plugin struct {
	opts  Options
	log   *slog.Logger
	group singleflight.Group
	sem   chan struct{}
}

// New returns the plugin.
func New(opts Options) *Plugin {
	if opts.Quality <= 0 || opts.Quality > 100 {
		opts.Quality = 80
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 24 * time.Hour
	}
	if opts.CacheDir == "" {
		opts.CacheDir = filepath.Join(os.TempDir(), "tern-images")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = safehttp.New(safehttp.Options{Timeout: 15 * time.Second, MaxBody: 32 << 20})
	}
	return &Plugin{opts: opts, sem: make(chan struct{}, runtime.NumCPU())}
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "images" }

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	if len(p.opts.Hosts) == 0 {
		return errors.New("images: Options.Hosts lists no hosts")
	}
	if err := os.MkdirAll(p.opts.CacheDir, 0o755); err != nil {
		return fmt.Errorf("images: %w", err)
	}
	p.log = s.Logger()
	s.Handle("GET "+Path, p)
	return nil
}

func (p *Plugin) allowed(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range p.opts.Hosts {
		h = strings.ToLower(h)
		if suffix, ok := strings.CutPrefix(h, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
		} else if host == h {
			return true
		}
	}
	return false
}

type result struct {
	data        []byte
	contentType string
}

func (p *Plugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	src, err := url.Parse(q.Get("url"))
	if err != nil || !p.allowed(src) {
		http.Error(w, "This image host is not allowed.", http.StatusBadRequest)
		return
	}
	width, err := strconv.Atoi(q.Get("w"))
	if err != nil || !slices.Contains(Widths, width) {
		http.Error(w, "Unsupported width.", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\n%d\n%d", src.String(), width, p.opts.Quality)))
	key := hex.EncodeToString(sum[:16])
	etag := `"` + key + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	v, err, _ := p.group.Do(key, func() (any, error) { return p.get(context.WithoutCancel(r.Context()), key, src.String(), width) })
	if err != nil {
		p.log.Warn("images: could not resize", "url", src.String(), "err", err)
		http.Error(w, "The image could not be loaded.", http.StatusBadGateway)
		return
	}
	res := v.(result)
	h := w.Header()
	h.Set("Content-Type", res.contentType)
	h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(p.opts.MaxAge.Seconds())))
	h.Set("ETag", etag)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(res.data)))
	_, _ = w.Write(res.data)
}

func (p *Plugin) get(ctx context.Context, key, src string, width int) (result, error) {
	for _, ext := range []string{".jpg", ".png"} {
		if data, err := os.ReadFile(filepath.Join(p.opts.CacheDir, key+ext)); err == nil {
			return result{data: data, contentType: map[string]string{".jpg": "image/jpeg", ".png": "image/png"}[ext]}, nil
		}
	}
	p.sem <- struct{}{}
	defer func() { <-p.sem }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return result{}, err
	}
	req.Header.Set("Accept", "image/jpeg,image/png,image/webp,image/gif")
	res, err := p.opts.HTTPClient.Do(req)
	if err != nil {
		return result{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return result{}, fmt.Errorf("the origin answered HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 20<<20+1))
	if err != nil {
		return result{}, err
	}
	if len(raw) > 20<<20 {
		return result{}, errors.New("the original is larger than 20 MB")
	}
	// Check the size before decoding, so a small file can't claim a huge image.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return result{}, fmt.Errorf("not an image: %w", err)
	}
	if cfg.Width*cfg.Height > 50_000_000 || cfg.Width <= 0 || cfg.Height <= 0 {
		return result{}, errors.New("the image has too many pixels")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return result{}, err
	}
	b := img.Bounds()
	if width > b.Dx() {
		width = b.Dx() // never enlarge
	}
	height := max(1, b.Dy()*width/b.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)

	var out bytes.Buffer
	ext, ct := ".jpg", "image/jpeg"
	if opaque, ok := img.(interface{ Opaque() bool }); ok && !opaque.Opaque() {
		ext, ct = ".png", "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, dst)
	} else {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: p.opts.Quality})
	}
	if err != nil {
		return result{}, err
	}
	tmp := filepath.Join(p.opts.CacheDir, key+ext+".tmp")
	if err := os.WriteFile(tmp, out.Bytes(), 0o644); err == nil {
		_ = os.Rename(tmp, filepath.Join(p.opts.CacheDir, key+ext))
	}
	return result{data: out.Bytes(), contentType: ct}, nil
}
