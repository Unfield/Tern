// Package storage keeps files: uploads, generated exports, anything that
// doesn't belong in the database. A Bucket is a directory on disk or an
// S3-compatible bucket (AWS S3, Cloudflare R2, MinIO, Backblaze B2, ...):
//
//	files, err := storage.Disk("data/files")
//	// or: storage.NewS3(storage.S3Options{Endpoint: "https://s3.eu-central-1.amazonaws.com", Region: "eu-central-1", Bucket: "acme", AccessKey: id, SecretKey: secret})
//
//	// In an action:
//	uploads, err := storage.Receive(ctx, "avatar", storage.ReceiveOptions{MaxSize: 5 << 20, Types: []string{"image/*"}})
//	key := storage.NewKey("avatars", uploads[0].Filename)
//	err = uploads[0].Save(ctx, files, key)
//
//	// Serving them, with links that expire:
//	signer := storage.NewSigner(secret)
//	kit.WithPlugins(storage.Serve("/files/", files, storage.ServeOptions{Signer: signer}))
//	url := signer.URL("/files/", key, time.Hour)
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
)

// ErrNotFound is returned for keys that don't exist.
var ErrNotFound = errors.New("storage: not found")

// Object describes a stored file.
type Object struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	Modified    time.Time `json:"modified"`
}

// Bucket stores files by key. Keys are slash-separated paths such as
// "avatars/2026/11/a1b2c3.png"; see ValidKey.
type Bucket interface {
	// Put stores the contents of r under key, replacing any file there.
	// size is the length of r, or -1 if unknown.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get opens a file. The reader is an io.ReadSeeker when the bucket can
	// seek, which lets Serve answer range requests.
	Get(ctx context.Context, key string) (io.ReadCloser, Object, error)
	Stat(ctx context.Context, key string) (Object, error)
	// Delete removes a file; deleting a missing file is not an error.
	Delete(ctx context.Context, key string) error
	// List returns the files whose keys start with prefix, sorted by key.
	List(ctx context.Context, prefix string) ([]Object, error)
}

// Presigner is implemented by buckets whose files can be fetched directly
// with a time-limited URL, such as S3; Serve redirects to it.
type Presigner interface {
	PresignGet(key string, ttl time.Duration) (string, error)
}

var keyRe = regexp.MustCompile(`^[A-Za-z0-9!_.*'()@ +,=-]+(/[A-Za-z0-9!_.*'()@ +,=-]+)*$`)

// ValidKey reports whether key is safe to use with every bucket: segments
// of letters, digits and !_.*'()@ +,=- separated by single slashes, no "."
// or ".." segments, and at most 1024 bytes.
func ValidKey(key string) error {
	if len(key) == 0 || len(key) > 1024 || !keyRe.MatchString(key) {
		return fmt.Errorf("storage: invalid key %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "." || seg == ".." || strings.HasPrefix(seg, ".tern") {
			return fmt.Errorf("storage: invalid key %q", key)
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// NewKey returns a new unique key under prefix for a file named filename,
// such as "avatars/2026/11/3f9a1c2e7b4d-portrait.png". The random part
// keeps keys unguessable; the name and extension are kept readable.
func NewKey(prefix, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if len(ext) > 10 || !regexp.MustCompile(`^\.[a-z0-9]+$`).MatchString(ext) {
		ext = ""
	}
	name := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSuffix(path.Base(filename), path.Ext(filename))), "-"), "-")
	if len(name) > 40 {
		name = name[:40]
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	if name != "" {
		id += "-" + name
	}
	now := time.Now().UTC()
	parts := []string{strings.Trim(prefix, "/"), now.Format("2006"), now.Format("01"), id + ext}
	if parts[0] == "" {
		parts = parts[1:]
	}
	return strings.Join(parts, "/")
}
