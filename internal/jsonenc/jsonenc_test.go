package jsonenc

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/nilslices"
)

type Inner struct {
	A int    `json:"a"`
	B string `json:"b,omitempty"`
}

type embedded struct {
	Hidden int // promoted though the type isn't exported
	Shared string
}

type Other struct {
	Shared string
	Deep   int `json:"deep"`
}

type kind int

func (k kind) MarshalText() ([]byte, error) { return []byte("kind-" + string(rune('a'+int(k)))), nil }

type ptrMarshal struct{ N int }

func (p *ptrMarshal) MarshalJSON() ([]byte, error) {
	return []byte(`{"custom": ` + string(rune('0'+p.N)) + ` }`), nil
}

type Node struct {
	Name     string  `json:"name"`
	Children []*Node `json:"children"`
}

type Everything struct {
	embedded
	*Other
	Bool       bool             `json:"bool"`
	Int        int64            `json:"int"`
	Uint       uint8            `json:"uint"`
	F32        float32          `json:"f32"`
	F64        float64          `json:"f64"`
	Str        string           `json:"str"`
	Tags       []string         `json:"tags"`
	NilTags    []string         `json:"nilTags"`
	Bytes      []byte           `json:"bytes"`
	NilBytes   []byte           `json:"nilBytes"`
	Arr        [3]int           `json:"arr"`
	Map        map[string]Inner `json:"map"`
	IntMap     map[int]string   `json:"intMap"`
	KindMap    map[kind]int     `json:"kindMap"`
	NilMap     map[string]int   `json:"nilMap"`
	Ptr        *Inner           `json:"ptr"`
	NilPtr     *Inner           `json:"nilPtr"`
	Any        any              `json:"any"`
	AnySlice   any              `json:"anySlice"`
	Time       time.Time        `json:"time"`
	Raw        json.RawMessage  `json:"raw"`
	Kind       kind             `json:"kind"`
	Custom     ptrMarshal       `json:"custom"`
	Omit       string           `json:"omit,omitempty"`
	OmitZero   time.Time        `json:"omitZero,omitzero"`
	Skip       string           `json:"-"`
	Dash       string           `json:"-,"` //nolint:staticcheck // "-" as a name, on purpose
	Untagged   int
	Nested     []Inner        `json:"nested"`
	Tree       *Node          `json:"tree"`
	Interface  map[string]any `json:"interface"`
	unexported int
	Escapes    string `json:"escapes"`
}

func sample() Everything {
	v := Everything{
		embedded: embedded{Hidden: 7, Shared: "mine"},
		Other:    &Other{Shared: "theirs", Deep: 3},
		Bool:     true, Int: -42, Uint: 200, F32: 3.14, F64: 1e21,
		Str: "héllo", Tags: []string{"a", "b"}, Bytes: []byte("hi"),
		Arr:    [3]int{1, 2, 3},
		Map:    map[string]Inner{"z": {A: 1}, "a": {A: 2, B: "x"}},
		IntMap: map[int]string{10: "ten", 2: "two"}, KindMap: map[kind]int{1: 1, 0: 0},
		Ptr: &Inner{A: 5}, Any: map[string]any{"k": []any{1, "two", nil}}, AnySlice: []int(nil),
		Time: time.Date(2026, 11, 11, 1, 2, 3, 4, time.UTC), Raw: json.RawMessage(`{ "x" : [1, 2] , "h":"<b>" }`),
		Kind: 2, Custom: ptrMarshal{N: 4}, Dash: "dash", Untagged: 9,
		Nested:     []Inner{{A: 1}, {A: 2, B: "&"}},
		Tree:       &Node{Name: "root", Children: []*Node{{Name: "leaf"}}},
		Interface:  map[string]any{"s": []string(nil), "n": 1.5e-7, "inner": Inner{A: 3}},
		unexported: 1,
		Escapes:    "<script>&  \x00\x1f\"\\\b\f\n\r\t\xff end",
	}
	return v
}

func check(t *testing.T, v any) {
	t.Helper()
	want, werr := json.Marshal(nilslices.Remove(v))
	got, gerr := Marshal(v)
	if (werr != nil) != (gerr != nil) {
		t.Fatalf("errors differ: stdlib %v, jsonenc %v", werr, gerr)
	}
	if string(got) != string(want) {
		t.Fatalf("differs:\n got %s\nwant %s", got, want)
	}
}

func TestMatchesEncodingJSON(t *testing.T) {
	v := sample()
	check(t, v)
	check(t, &v)
	check(t, []Everything{v, {}})
	check(t, map[string]Everything{"a": v})
	check(t, Everything{})
	for _, f := range []float64{0, math.Copysign(0, -1), 1, -1, 0.1, 1e-6, 1e-7, 123456789, 1e20, 1e21, 1.5e300, 5e-324, math.MaxFloat64} {
		check(t, f)
		check(t, float32(f))
	}
	check(t, nil)
	check(t, "plain")
	check(t, []any{nil, true, []string(nil)})
	check(t, struct {
		A, B int
		C    string `json:"c,string"` // left to encoding/json
	}{1, 2, "x"})
}

func TestNilSlicesAreEmpty(t *testing.T) {
	got, _ := Marshal(struct {
		A []int            `json:"a"`
		B map[string][]int `json:"b"`
		C []byte           `json:"c"`
	}{B: map[string][]int{"x": nil}})
	if string(got) != `{"a":[],"b":{"x":[]},"c":null}` {
		t.Fatal(string(got))
	}
}

func TestErrors(t *testing.T) {
	if _, err := Marshal(math.NaN()); err == nil {
		t.Fatal("NaN encoded")
	}
	if _, err := Marshal(map[string]any{"f": func() {}}); err == nil {
		t.Fatal("func encoded")
	}
	n := &Node{Name: "loop"}
	n.Children = []*Node{n}
	if _, err := Marshal(n); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := Marshal(json.RawMessage(`{bad`)); err == nil {
		t.Fatal("bad raw message")
	}
}

func FuzzStrings(f *testing.F) {
	for _, s := range []string{"", "a", "<>&", " ", "\xff\xfe", "é\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		check(t, s)
		check(t, map[string]string{s: s})
	})
}

func BenchmarkItems(b *testing.B) {
	type item struct {
		ID      int      `json:"id"`
		Name    string   `json:"name"`
		Price   float64  `json:"price"`
		Tags    []string `json:"tags"`
		Updated string   `json:"updated"`
		Stock   int      `json:"stock"`
	}
	items := make([]item, 100)
	for i := range items {
		items[i] = item{ID: i, Name: "Item", Price: float64(i) + 0.99, Tags: []string{"a", "b", "c"}, Updated: "2026-01-01T00:00:00Z", Stock: i % 9}
	}
	v := struct {
		Items []item `json:"items"`
	}{items}
	b.Run("jsonenc", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = Marshal(v)
		}
	})
	b.Run("stdlib", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = json.Marshal(v)
		}
	})
	b.Run("stdlib+nilslices", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = json.Marshal(nilslices.Remove(v))
		}
	})
}
