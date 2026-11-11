package kittest

import (
	"net/http/httptest"
	"sync"

	"github.com/Unfield/Tern/kit"
)

// syncMap maps a Ctx to its recorder. Entries are few and short-lived:
// one per Ctx a test makes.
type syncMap struct{ m sync.Map }

func (s *syncMap) Store(c *kit.Ctx, w *httptest.ResponseRecorder) { s.m.Store(c, w) }

func (s *syncMap) Load(c *kit.Ctx) (*httptest.ResponseRecorder, bool) {
	v, ok := s.m.Load(c)
	if !ok {
		return nil, false
	}
	return v.(*httptest.ResponseRecorder), true
}
