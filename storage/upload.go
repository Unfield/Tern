package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/Unfield/Tern/kit"
)

// Upload is a file sent with a form.
type Upload struct {
	// Filename is the name the browser sent, without directories. Use it
	// for display only; NewKey makes a safe key from it.
	Filename string
	Size     int64
	// ContentType is detected from the file's contents, not taken from
	// the browser.
	ContentType string
	header      *multipart.FileHeader
}

// Open reads the file.
func (u *Upload) Open() (multipart.File, error) { return u.header.Open() }

// Save stores the file in b under key.
func (u *Upload) Save(ctx context.Context, b Bucket, key string) error {
	f, err := u.Open()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return b.Put(ctx, key, f, u.Size, u.ContentType)
}

// ReceiveOptions limit what Receive accepts.
type ReceiveOptions struct {
	// MaxSize is the largest file accepted, in bytes; the default is 10 MB.
	MaxSize int64
	// MaxFiles is how many files the field may have; the default is 1.
	MaxFiles int
	// Types lists accepted content types, such as "image/*" or
	// "application/pdf"; by default any type is accepted.
	Types []string
	// Required makes an empty field an error.
	Required bool
}

// InvalidUpload is returned for files Receive refuses; show Message next
// to the field, with kit.Fail.
type InvalidUpload struct {
	Field   string
	Message string
}

func (e *InvalidUpload) Error() string { return "storage: " + e.Field + ": " + e.Message }

// Receive reads the files of a form field in an action or endpoint. It
// raises the request's body limit to fit MaxFiles files of MaxSize, checks
// each file's size and its type, detected from its first bytes, and
// returns an *InvalidUpload error for files it refuses:
//
//	uploads, err := storage.Receive(ctx, "avatar", storage.ReceiveOptions{MaxSize: 2 << 20, Types: []string{"image/png", "image/jpeg"}})
//	var invalid *storage.InvalidUpload
//	if errors.As(err, &invalid) {
//		return nil, kit.Fail(400, Invalid{Error: invalid.Message})
//	}
//
// The form must use enctype="multipart/form-data".
func Receive(ctx *kit.Ctx, field string, opts ReceiveOptions) ([]*Upload, error) {
	if opts.MaxSize <= 0 {
		opts.MaxSize = 10 << 20
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 1
	}
	ctx.SetBodyLimit(opts.MaxSize*int64(opts.MaxFiles) + 1<<20)
	if ct, _, _ := mime.ParseMediaType(ctx.Request.Header.Get("Content-Type")); ct != "multipart/form-data" {
		return nil, kit.Error(http.StatusBadRequest, `file uploads need a form with enctype="multipart/form-data"`)
	}
	if _, err := ctx.Form(); err != nil {
		var ke *kit.HTTPError
		if errors.As(err, &ke) && ke.Status == http.StatusRequestEntityTooLarge {
			return nil, &InvalidUpload{Field: field, Message: "The upload is too large; the limit is " + humanSize(opts.MaxSize) + "."}
		}
		return nil, err
	}
	var headers []*multipart.FileHeader
	if ctx.Request.MultipartForm != nil {
		for _, h := range ctx.Request.MultipartForm.File[field] {
			if h.Size > 0 || h.Filename != "" {
				headers = append(headers, h)
			}
		}
	}
	switch {
	case len(headers) == 0 && opts.Required:
		return nil, &InvalidUpload{Field: field, Message: "Choose a file."}
	case len(headers) > opts.MaxFiles:
		return nil, &InvalidUpload{Field: field, Message: fmt.Sprintf("Choose at most %d files.", opts.MaxFiles)}
	}
	out := make([]*Upload, 0, len(headers))
	for _, h := range headers {
		name := path.Base(strings.ReplaceAll(h.Filename, `\`, "/"))
		if h.Size > opts.MaxSize {
			return nil, &InvalidUpload{Field: field, Message: fmt.Sprintf("%s is larger than %s.", name, humanSize(opts.MaxSize))}
		}
		ct, err := sniff(h)
		if err != nil {
			return nil, err
		}
		if len(opts.Types) > 0 && !inline(ct, opts.Types) {
			return nil, &InvalidUpload{Field: field, Message: fmt.Sprintf("%s is not an accepted kind of file.", name)}
		}
		out = append(out, &Upload{Filename: name, Size: h.Size, ContentType: ct, header: h})
	}
	return out, nil
}

// sniff detects a file's type from its first 512 bytes.
func sniff(h *multipart.FileHeader) (string, error) {
	f, err := h.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 512)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	ct := http.DetectContentType(buf[:n])
	// DetectContentType can't tell these apart from plain text or zip.
	if strings.HasPrefix(ct, "text/plain") {
		switch strings.ToLower(path.Ext(h.Filename)) {
		case ".csv":
			return "text/csv", nil
		case ".md":
			return "text/markdown", nil
		case ".json":
			return "application/json", nil
		}
	}
	return ct, nil
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}
