package js

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"
	"sync"
)

// Sources is the source text of compiled modules, for Function.prototype
// .toString: gzipped JSON, an array of each module's code, unpacked when
// first needed.
type Sources struct {
	// GZ is the gzipped sources: generated code points it at an embedded
	// string, so a Sources, and the FnInfos that point at it, are data the
	// linker lays out rather than code that runs at start.
	GZ   *string
	once sync.Once
	gz   string
	mods []string
}

// NewSources holds gzipped sources.
func NewSources(gz string) *Sources { return &Sources{gz: gz} }

// Module is the code of module i.
func (s *Sources) Module(i int) string {
	s.once.Do(func() {
		gz := s.gz
		if s.GZ != nil {
			gz = *s.GZ
		}
		r, err := gzip.NewReader(strings.NewReader(gz))
		if err != nil {
			return
		}
		b, err := io.ReadAll(r)
		if err != nil {
			return
		}
		_ = json.Unmarshal(b, &s.mods)
	})
	if i < 0 || i >= len(s.mods) {
		return ""
	}
	return s.mods[i]
}

// SetInfo describes a function made another way, such as a class.
func (o *Object) SetInfo(info *FnInfo) {
	if o.fnLazy != 0 {
		o.fn = info
		return
	}
	src := *info
	if o.fn != nil {
		src.Name, src.Length = o.fn.Name, o.fn.Length
	}
	o.fn = &src
}

// WithInfo sets o's info and returns o, for compiled code.
func WithInfo(o *Object, info *FnInfo) *Object {
	o.SetInfo(info)
	return o
}
