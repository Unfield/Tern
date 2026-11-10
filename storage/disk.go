package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// DiskBucket stores files in a directory. Content types are kept next to
// the files, in a .tern-meta directory.
type DiskBucket struct{ dir string }

// Disk returns a bucket in dir, creating it if needed.
func Disk(dir string) (*DiskBucket, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &DiskBucket{dir: abs}, nil
}

func (b *DiskBucket) file(key string) (string, error) {
	if err := ValidKey(key); err != nil {
		return "", err
	}
	return filepath.Join(b.dir, filepath.FromSlash(key)), nil
}

func (b *DiskBucket) meta(key string) string {
	return filepath.Join(b.dir, ".tern-meta", filepath.FromSlash(key)+".json")
}

// Put implements Bucket. The file appears complete or not at all.
func (b *DiskBucket) Put(ctx context.Context, key string, r io.Reader, _ int64, contentType string) error {
	name, err := b.file(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := io.Copy(tmp, readerWithContext(ctx, r)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(key))
	}
	if err := os.MkdirAll(filepath.Dir(b.meta(key)), 0o755); err != nil {
		return err
	}
	m, _ := json.Marshal(map[string]string{"contentType": contentType})
	if err := os.WriteFile(b.meta(key), m, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func readerWithContext(ctx context.Context, r io.Reader) io.Reader { return ctxReader{ctx, r} }

func (b *DiskBucket) object(key string, info fs.FileInfo) Object {
	o := Object{Key: key, Size: info.Size(), Modified: info.ModTime()}
	if raw, err := os.ReadFile(b.meta(key)); err == nil {
		var m struct {
			ContentType string `json:"contentType"`
		}
		if json.Unmarshal(raw, &m) == nil {
			o.ContentType = m.ContentType
		}
	}
	if o.ContentType == "" {
		o.ContentType = mime.TypeByExtension(path.Ext(key))
	}
	if o.ContentType == "" {
		o.ContentType = "application/octet-stream"
	}
	return o
}

// Get implements Bucket; the reader is an *os.File.
func (b *DiskBucket) Get(_ context.Context, key string) (io.ReadCloser, Object, error) {
	name, err := b.file(key)
	if err != nil {
		return nil, Object{}, err
	}
	f, err := os.Open(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Object{}, ErrNotFound
	}
	if err != nil {
		return nil, Object{}, err
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		_ = f.Close()
		return nil, Object{}, ErrNotFound
	}
	return f, b.object(key, info), nil
}

// Stat implements Bucket.
func (b *DiskBucket) Stat(_ context.Context, key string) (Object, error) {
	name, err := b.file(key)
	if err != nil {
		return Object{}, err
	}
	info, err := os.Stat(name)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir()) {
		return Object{}, ErrNotFound
	}
	if err != nil {
		return Object{}, err
	}
	return b.object(key, info), nil
}

// Delete implements Bucket.
func (b *DiskBucket) Delete(_ context.Context, key string) error {
	name, err := b.file(key)
	if err != nil {
		return err
	}
	_ = os.Remove(b.meta(key))
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Remove directories the file leaves empty.
	for dir := filepath.Dir(name); dir != b.dir && strings.HasPrefix(dir, b.dir); dir = filepath.Dir(dir) {
		if !removeEmpty(dir) {
			break
		}
	}
	return nil
}

// removeEmpty removes dir if it is empty, and reports whether it did.
func removeEmpty(dir string) bool { return os.Remove(dir) == nil }

// List implements Bucket.
func (b *DiskBucket) List(_ context.Context, prefix string) ([]Object, error) {
	out := []Object{}
	err := filepath.WalkDir(b.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(b.dir, p)
		key := filepath.ToSlash(rel)
		if d.IsDir() {
			if key == ".tern-meta" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".upload-") || !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, b.object(key, info))
		return nil
	})
	slices.SortFunc(out, func(a, c Object) int { return strings.Compare(a.Key, c.Key) })
	return out, err
}
