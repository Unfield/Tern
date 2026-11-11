// Package file is a logging sink that writes records to a file as JSON
// lines, rotating it by size:
//
//	file.New(file.Options{Path: "logs/app.log", MaxSize: 50 << 20, Keep: 10})
//
// A rotated file is renamed with the time it was rotated,
// app-20261111T150405.123.log, and gzipped if Compress is set; the oldest
// beyond Keep are removed.
package file

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/logging"
)

// Options configures the sink.
type Options struct {
	// Path is the file written to. Its directory is created if need be.
	Path string
	// MaxSize is the size in bytes at which the file is rotated. The
	// default is 100 MiB; -1 never rotates.
	MaxSize int64
	// Keep is how many rotated files are kept. The default is 7; -1 keeps
	// them all.
	Keep int
	// Compress gzips rotated files.
	Compress bool
	// Mode is the new files' permissions. The default is 0o640.
	Mode os.FileMode
}

// Sink writes to a file. Make one with New; Close it to close the file
// (logging.Close does).
type Sink struct {
	o   Options
	mu  sync.Mutex
	f   *os.File
	w   *bufio.Writer
	n   int64
	now func() time.Time
	// last is the time in the last rotated file's name; names are kept
	// apart by at least a millisecond.
	last time.Time
}

// New returns a file sink. The file is opened on the first write.
func New(o Options) *Sink {
	if o.MaxSize == 0 {
		o.MaxSize = 100 << 20
	}
	if o.Keep == 0 {
		o.Keep = 7
	}
	if o.Mode == 0 {
		o.Mode = 0o640
	}
	return &Sink{o: o, now: time.Now}
}

// Name implements the name logging reports errors with.
func (s *Sink) Name() string { return "file " + s.o.Path }

// Export implements logging.Sink.
func (s *Sink) Export(_ context.Context, entries []logging.Entry) error {
	if s.o.Path == "" {
		return logging.Permanent(errors.New("file: no Path"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range logging.EncodeJSON(entries) {
		line, err := json.Marshal(e)
		if err != nil {
			continue // can't happen: EncodeJSON makes encodable values
		}
		line = append(line, '\n')
		if s.f != nil && s.o.MaxSize > 0 && s.n > 0 && s.n+int64(len(line)) > s.o.MaxSize {
			if err := s.rotate(); err != nil {
				return err
			}
		}
		if s.f == nil {
			if err := s.open(); err != nil {
				return err
			}
		}
		if _, err := s.w.Write(line); err != nil {
			return err
		}
		s.n += int64(len(line))
	}
	if s.w != nil {
		return s.w.Flush()
	}
	return nil
}

func (s *Sink) open() error {
	if err := os.MkdirAll(filepath.Dir(s.o.Path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(s.o.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, s.o.Mode)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	s.f, s.w, s.n = f, bufio.NewWriterSize(f, 64<<10), st.Size()
	return nil
}

// rotate closes the file, renames it and opens a new one.
func (s *Sink) rotate() error {
	if err := s.closeFile(); err != nil {
		return err
	}
	ext := filepath.Ext(s.o.Path)
	base := strings.TrimSuffix(s.o.Path, ext)
	t := s.now().UTC().Truncate(time.Millisecond)
	if !t.After(s.last) {
		t = s.last.Add(time.Millisecond)
	}
	s.last = t
	name := fmt.Sprintf("%s-%s%s", base, t.Format("20060102T150405.000"), ext)
	if err := os.Rename(s.o.Path, name); err != nil {
		return err
	}
	if s.o.Compress {
		if err := gzipFile(name); err != nil {
			return err
		}
	}
	return s.prune(base, ext)
}

func (s *Sink) prune(base, ext string) error {
	if s.o.Keep < 0 {
		return nil
	}
	matches, err := filepath.Glob(globEscape(base) + "-*" + globEscape(ext) + "*")
	if err != nil {
		return err
	}
	var rotated []string
	for _, m := range matches {
		rest := strings.TrimPrefix(m, base+"-")
		rest = strings.TrimSuffix(strings.TrimSuffix(rest, ".gz"), ext)
		if _, err := time.Parse("20060102T150405.000", rest); err == nil {
			rotated = append(rotated, m)
		}
	}
	slices.Sort(rotated) // the names sort by time
	var errs []error
	for len(rotated) > s.o.Keep {
		errs = append(errs, os.Remove(rotated[0]))
		rotated = rotated[1:]
	}
	return errors.Join(errs...)
}

func globEscape(s string) string {
	r := strings.NewReplacer(`*`, `\*`, `?`, `\?`, `[`, `\[`)
	return r.Replace(s)
}

func gzipFile(name string) error {
	in, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(name+".gz", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode())
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	// Windows can't remove a file that is still open.
	if err := in.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

func (s *Sink) closeFile() error {
	if s.f == nil {
		return nil
	}
	err := errors.Join(s.w.Flush(), s.f.Close())
	s.f, s.w, s.n = nil, nil, 0
	return err
}

// Close flushes and closes the file.
func (s *Sink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeFile()
}
