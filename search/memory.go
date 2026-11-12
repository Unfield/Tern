package search

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// memory keeps an index in memory: an inverted index from words to the
// documents and fields they're in, with the words sorted for prefixes.
type memory struct {
	opts Options

	mu       sync.RWMutex
	docs     map[string]*memDoc
	postings map[string]map[string][]int // word -> doc ID -> count per field
	terms    []string                    // sorted words, rebuilt when stale
	stale    bool
	lengths  []int // total words per field, for the average
}

type memDoc struct {
	scope  string
	fields []string // the text of each field
	tokens [][]token
}

// token is a word of a field and where it is in the text.
type token struct {
	word       string
	start, end int
}

func newMemory(opts Options) *memory {
	return &memory{opts: opts, docs: map[string]*memDoc{}, postings: map[string]map[string][]int{}, lengths: make([]int, len(opts.Fields))}
}

func tokenize(text string) []token {
	var out []token
	start := -1
	var cur []rune
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) && start >= 0 {
			if start < 0 {
				start = i
			}
			if !unicode.Is(unicode.Mn, r) {
				cur = append(cur, fold(r))
			}
			continue
		}
		if start >= 0 {
			out = append(out, token{word: string(cur), start: start, end: i})
			start, cur = -1, cur[:0]
		}
	}
	if start >= 0 {
		out = append(out, token{word: string(cur), start: start, end: len(text)})
	}
	return out
}

func (m *memory) migrate(context.Context) error { return nil }

func (m *memory) put(_ context.Context, d Doc) error {
	doc := &memDoc{scope: d.Scope, fields: make([]string, len(m.opts.Fields)), tokens: make([][]token, len(m.opts.Fields))}
	for i, f := range m.opts.Fields {
		doc.fields[i] = d.Fields[f.Name]
		doc.tokens[i] = tokenize(doc.fields[i])
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drop(d.ID)
	m.docs[d.ID] = doc
	for i, toks := range doc.tokens {
		m.lengths[i] += len(toks)
		for _, t := range toks {
			p := m.postings[t.word]
			if p == nil {
				p = map[string][]int{}
				m.postings[t.word] = p
				m.stale = true
			}
			if p[d.ID] == nil {
				p[d.ID] = make([]int, len(m.opts.Fields))
			}
			p[d.ID][i]++
		}
	}
	return nil
}

// drop removes a document; the lock is held.
func (m *memory) drop(id string) {
	doc := m.docs[id]
	if doc == nil {
		return
	}
	delete(m.docs, id)
	for i, toks := range doc.tokens {
		m.lengths[i] -= len(toks)
		for _, t := range toks {
			if p := m.postings[t.word]; p != nil {
				delete(p, id)
				if len(p) == 0 {
					delete(m.postings, t.word)
					m.stale = true
				}
			}
		}
	}
}

func (m *memory) remove(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drop(id)
	return nil
}

func (m *memory) reset(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs, m.postings, m.terms, m.stale = map[string]*memDoc{}, map[string]map[string][]int{}, nil, false
	m.lengths = make([]int, len(m.opts.Fields))
	return nil
}

// sorted returns the sorted words, rebuilding them if words came or went.
func (m *memory) sorted() []string {
	m.mu.RLock()
	if !m.stale {
		t := m.terms
		m.mu.RUnlock()
		return t
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stale {
		m.terms = make([]string, 0, len(m.postings))
		for w := range m.postings {
			m.terms = append(m.terms, w)
		}
		sort.Strings(m.terms)
		m.stale = false
	}
	return m.terms
}

func (m *memory) search(_ context.Context, words []string, q Query) (Results, error) {
	terms := m.sorted()
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := float64(len(m.docs))
	avg := make([]float64, len(m.opts.Fields))
	for i, l := range m.lengths {
		avg[i] = math.Max(float64(l)/math.Max(n, 1), 1)
	}
	const k1, b = 1.2, 0.75
	var scores map[string]float64
	for _, w := range words {
		got := map[string]float64{}
		// Every indexed word that starts with w.
		for i := sort.SearchStrings(terms, w); i < len(terms) && strings.HasPrefix(terms[i], w); i++ {
			p := m.postings[terms[i]]
			if p == nil {
				continue // removed since the words were sorted
			}
			idf := math.Log(1 + (n-float64(len(p))+0.5)/(float64(len(p))+0.5))
			for id, counts := range p {
				doc := m.docs[id]
				if q.Scope != "" && doc.scope != q.Scope {
					continue
				}
				for f, tf := range counts {
					if tf == 0 {
						continue
					}
					l := float64(len(doc.tokens[f]))
					got[id] += m.opts.Fields[f].Boost * idf * float64(tf) * (k1 + 1) / (float64(tf) + k1*(1-b+b*l/avg[f]))
				}
			}
		}
		// Every word must match.
		if scores == nil {
			scores = got
		} else {
			for id := range scores {
				if s, ok := got[id]; ok {
					scores[id] += s
				} else {
					delete(scores, id)
				}
			}
		}
		if len(scores) == 0 {
			break
		}
	}
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if scores[ids[i]] != scores[ids[j]] {
			return scores[ids[i]] > scores[ids[j]]
		}
		return ids[i] < ids[j]
	})
	res := Results{Hits: []Hit{}, Total: len(ids)}
	for _, id := range ids[min(q.Offset, len(ids)):min(q.Offset+q.Limit, len(ids))] {
		doc := m.docs[id]
		h := Hit{ID: id, Scope: doc.scope, Score: scores[id], Snippets: map[string][]Segment{}}
		for f, field := range m.opts.Fields {
			h.Snippets[field.Name] = snippet(doc.fields[f], doc.tokens[f], words)
		}
		res.Hits = append(res.Hits, h)
	}
	return res, nil
}

// snippet marks the words of text that start with a query word, cut to a
// few words around the first when the text is long.
func snippet(text string, toks []token, words []string) []Segment {
	const window = 16
	matches := func(t token) bool {
		for _, w := range words {
			if strings.HasPrefix(t.word, w) {
				return true
			}
		}
		return false
	}
	from, to := 0, len(toks)
	if len(toks) > window {
		first := 0
		for i, t := range toks {
			if matches(t) {
				first = i
				break
			}
		}
		from = max(0, first-window/4)
		to = min(len(toks), from+window)
		from = max(0, to-window)
	}
	if len(toks) == 0 {
		if text == "" {
			return []Segment{}
		}
		return []Segment{{Text: text}}
	}
	start, end := 0, len(text)
	if from > 0 {
		start = toks[from].start
	}
	if to < len(toks) {
		end = toks[to-1].end
	}
	var b strings.Builder
	if from > 0 {
		b.WriteString("…")
	}
	pos := start
	for _, t := range toks[from:to] {
		b.WriteString(text[pos:t.start])
		if matches(t) {
			b.WriteString("\x01" + text[t.start:t.end] + "\x02")
		} else {
			b.WriteString(text[t.start:t.end])
		}
		pos = t.end
	}
	b.WriteString(text[pos:end])
	if to < len(toks) {
		b.WriteString("…")
	}
	s := b.String()
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	return segments(s)
}
