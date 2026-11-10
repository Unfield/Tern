package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit"
)

// buckets returns a disk bucket, and an S3 bucket when TERN_TEST_S3 holds
// "endpoint|region|bucket|access|secret" (CI runs MinIO).
func buckets(t *testing.T) map[string]Bucket {
	disk, err := Disk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Bucket{"disk": disk}
	if spec := os.Getenv("TERN_TEST_S3"); spec != "" {
		p := strings.Split(spec, "|")
		s3, err := NewS3(S3Options{Endpoint: p[0], Region: p[1], Bucket: p[2], AccessKey: p[3], SecretKey: p[4], PathStyle: true})
		if err != nil {
			t.Fatal(err)
		}
		out["s3"] = s3
	}
	return out
}

func TestBuckets(t *testing.T) {
	for name, b := range buckets(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			prefix := "t" + time.Now().Format("150405.000000") + "/"
			key := prefix + "docs/report one.txt"
			if err := b.Put(ctx, key, strings.NewReader("hello world"), 11, "text/plain"); err != nil {
				t.Fatal(err)
			}
			rc, obj, err := b.Get(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(rc)
			_ = rc.Close()
			if string(body) != "hello world" || obj.Size != 11 || !strings.HasPrefix(obj.ContentType, "text/plain") {
				t.Errorf("Get = %q %+v", body, obj)
			}
			if st, err := b.Stat(ctx, key); err != nil || st.Size != 11 {
				t.Errorf("Stat = %+v, %v", st, err)
			}
			// Unknown size is fine too.
			if err := b.Put(ctx, prefix+"b.bin", io.MultiReader(strings.NewReader("ab"), strings.NewReader("c")), -1, ""); err != nil {
				t.Fatal(err)
			}
			list, err := b.List(ctx, prefix)
			if err != nil || len(list) != 2 || list[0].Key != prefix+"b.bin" || list[1].Key != key {
				t.Errorf("List = %+v, %v", list, err)
			}
			if err := b.Delete(ctx, key); err != nil {
				t.Fatal(err)
			}
			if _, _, err := b.Get(ctx, key); !errors.Is(err, ErrNotFound) {
				t.Errorf("Get after Delete: %v", err)
			}
			if _, err := b.Stat(ctx, prefix+"missing"); !errors.Is(err, ErrNotFound) {
				t.Errorf("Stat of a missing key: %v", err)
			}
			if err := b.Delete(ctx, prefix+"missing"); err != nil {
				t.Errorf("deleting a missing key: %v", err)
			}
			_ = b.Delete(ctx, prefix+"b.bin")
		})
	}
}

func TestKeys(t *testing.T) {
	for _, bad := range []string{"", "/abs", "a/../b", "a//b", "..", ".", "a/./b", "a\\b", "x\x00", ".tern-meta/x", strings.Repeat("a", 1025)} {
		if ValidKey(bad) == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	disk, _ := Disk(t.TempDir())
	if err := disk.Put(context.Background(), "../escape.txt", strings.NewReader("x"), 1, ""); err == nil {
		t.Error("a key escaped the directory")
	}
	k := NewKey("avatars", "My Photo (1).JPG")
	if !strings.HasPrefix(k, "avatars/"+time.Now().UTC().Format("2006/01")+"/") || !strings.HasSuffix(k, "-my-photo-1.jpg") || ValidKey(k) != nil {
		t.Errorf("NewKey = %q", k)
	}
	if k := NewKey("", "../../etc/passwd"); strings.Contains(k, "..") || ValidKey(k) != nil {
		t.Errorf("NewKey = %q", k)
	}
}

func app(t *testing.T, plugins []kit.Plugin, routes ...kit.RouteDef) *kit.App {
	t.Helper()
	a, err := kit.New(&kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: routes}, kit.WithPlugins(plugins...), kit.WithBodyLimit(1024))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestServe(t *testing.T) {
	disk, _ := Disk(t.TempDir())
	ctx := context.Background()
	_ = disk.Put(ctx, "a/pic.png", bytes.NewReader([]byte("\x89PNG\r\n\x1a\nxxxxxxxx")), 16, "image/png")
	_ = disk.Put(ctx, "a/page.html", strings.NewReader("<script>alert(1)</script>"), 25, "text/html")
	signer := NewSigner(strings.Repeat("k", 32))
	public := app(t, []kit.Plugin{Serve("/files/", disk, ServeOptions{})})
	private := app(t, []kit.Plugin{Serve("/files/", disk, ServeOptions{Signer: signer})})
	get := func(a http.Handler, u string, h ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, u, nil)
		for i := 0; i+1 < len(h); i += 2 {
			r.Header.Set(h[i], h[i+1])
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}

	w := get(public, "/files/a/pic.png")
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("png: %d %v", w.Code, w.Header())
	}
	// Uploaded HTML is downloaded, never rendered on the site.
	w = get(public, "/files/a/page.html")
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("html: %v", w.Header())
	}
	if w := get(public, "/files/a/pic.png", "Range", "bytes=0-3"); w.Code != http.StatusPartialContent || w.Body.String() != "\x89PNG" {
		t.Errorf("range: %d %q", w.Code, w.Body)
	}
	if w := get(public, "/files/../secret"); w.Code != 404 {
		t.Errorf("traversal: %d", w.Code)
	}
	if w := get(public, "/files/nope.png"); w.Code != 404 {
		t.Errorf("missing: %d", w.Code)
	}

	if w := get(private, "/files/a/pic.png"); w.Code != 403 {
		t.Errorf("unsigned private file: %d", w.Code)
	}
	if w := get(private, signer.URL("/files/", "a/pic.png", time.Minute)); w.Code != 200 {
		t.Errorf("signed: %d", w.Code)
	}
	if w := get(private, signer.URL("/files/", "a/pic.png", -time.Minute)); w.Code != 403 {
		t.Errorf("expired: %d", w.Code)
	}
	if w := get(private, strings.Replace(signer.URL("/files/", "a/pic.png", time.Minute), "pic.png", "page.html", 1)); w.Code != 403 {
		t.Errorf("a signature for another file: %d", w.Code)
	}
}

func multipartBody(files map[string]string) (string, *bytes.Buffer) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("title", "hi")
	for name, content := range files {
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte(content))
	}
	_ = mw.Close()
	return mw.FormDataContentType(), &buf
}

func TestReceive(t *testing.T) {
	disk, _ := Disk(t.TempDir())
	var saved string
	a := app(t, nil, kit.RouteDef{ID: "/upload", Endpoint: map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error {
		ups, err := Receive(c, "file", ReceiveOptions{MaxSize: 4096, Types: []string{"image/*"}, Required: true})
		var invalid *InvalidUpload
		if errors.As(err, &invalid) {
			return c.Text(400, invalid.Message)
		}
		if err != nil {
			return err
		}
		key := NewKey("u", ups[0].Filename)
		if err := ups[0].Save(c, disk, key); err != nil {
			return err
		}
		saved = key
		form, _ := c.Form()
		return c.Text(200, ups[0].ContentType+" "+form.Get("title"))
	}}})
	post := func(files map[string]string) *httptest.ResponseRecorder {
		ct, body := multipartBody(files)
		r := httptest.NewRequest(http.MethodPost, "/upload", body)
		r.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 2000) // more than the app's 1 KB body limit
	if w := post(map[string]string{"photo.png": png}); w.Code != 200 || w.Body.String() != "image/png hi" {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	if obj, err := disk.Stat(context.Background(), saved); err != nil || obj.Size != int64(len(png)) || obj.ContentType != "image/png" {
		t.Errorf("saved %s: %+v %v", saved, obj, err)
	}
	// The type comes from the contents, not the name.
	if w := post(map[string]string{"fake.png": "<html><script>x</script>"}); w.Code != 400 || !strings.Contains(w.Body.String(), "not an accepted kind") {
		t.Errorf("disguised html: %d %s", w.Code, w.Body)
	}
	if w := post(map[string]string{"big.png": "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 5000)}); w.Code != 400 || !strings.Contains(w.Body.String(), "larger than 4 KB") {
		t.Errorf("too big: %d %s", w.Code, w.Body)
	}
	if w := post(map[string]string{}); w.Code != 400 || w.Body.String() != "Choose a file." {
		t.Errorf("missing: %d %s", w.Code, w.Body)
	}
	if w := post(map[string]string{"huge.png": strings.Repeat("x", 3<<20)}); w.Code != 400 || !strings.Contains(w.Body.String(), "too large") {
		t.Errorf("over the request limit: %d %s", w.Code, w.Body)
	}
}
