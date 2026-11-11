package jsvalue

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/jsonenc"
	"github.com/Unfield/Tern/native/js"
)

type Inner struct {
	A int    `json:"a"`
	B string `json:"b,omitempty"`
}

type embedded struct {
	Hidden int
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
	return []byte(`{"custom":` + string(rune('0'+p.N)) + `}`), nil
}

type Node struct {
	Name     string  `json:"name"`
	Children []*Node `json:"children"`
}

type Everything struct {
	embedded
	*Other
	Bool      bool             `json:"bool"`
	Int       int64            `json:"int"`
	Big       uint64           `json:"big"`
	F32       float32          `json:"f32"`
	F64       float64          `json:"f64"`
	NegZero   float64          `json:"negZero"`
	Str       string           `json:"str"`
	Bad       string           `json:"bad"`
	Tags      []string         `json:"tags"`
	NilTags   []string         `json:"nilTags"`
	Bytes     []byte           `json:"bytes"`
	NilBytes  []byte           `json:"nilBytes"`
	Arr       [3]int           `json:"arr"`
	Map       map[string]Inner `json:"map"`
	NumKeys   map[string]int   `json:"numKeys"`
	IntMap    map[int]string   `json:"intMap"`
	KindMap   map[kind]int     `json:"kindMap"`
	NilMap    map[string]int   `json:"nilMap"`
	Ptr       *Inner           `json:"ptr"`
	NilPtr    *Inner           `json:"nilPtr"`
	Any       any              `json:"any"`
	AnySlice  any              `json:"anySlice"`
	Time      time.Time        `json:"time"`
	Raw       json.RawMessage  `json:"raw"`
	Verbatim  jsonenc.Verbatim `json:"verbatim"`
	Kind      kind             `json:"kind"`
	Custom    ptrMarshal       `json:"custom"`
	Omit      string           `json:"omit,omitempty"`
	OmitZero  time.Time        `json:"omitZero,omitzero"`
	Skip      string           `json:"-"`
	Untagged  int
	Nested    []Inner        `json:"nested"`
	Tree      *Node          `json:"tree"`
	Interface map[string]any `json:"interface"`
	Quoted    struct {
		N int `json:"n,string"`
	} `json:"quoted"`
	unexported int //nolint:unused // left out, as encoding/json leaves it
}

func sample() Everything {
	return Everything{
		embedded: embedded{Hidden: 7, Shared: "mine"},
		Other:    &Other{Shared: "theirs", Deep: 3},
		Bool:     true, Int: -42, Big: 1<<63 + 12345, F32: 3.14, F64: 1e21, NegZero: math.Copysign(0, -1),
		Str: "héllo <b>&\u2028", Bad: "a\xffb\xed\xa0\x80c", Tags: []string{"a", "b"}, Bytes: []byte("hi"),
		Arr:     [3]int{1, 2, 3},
		Map:     map[string]Inner{"z": {A: 1}, "a": {A: 2, B: "x"}},
		NumKeys: map[string]int{"10": 1, "2": 2, "b": 3, "a": 4},
		IntMap:  map[int]string{10: "ten", 2: "two", -1: "minus"},
		KindMap: map[kind]int{1: 1, 0: 0},
		Ptr:     &Inner{A: 5}, Any: map[string]any{"k": []any{1, "two", nil, 2.5}}, AnySlice: []int(nil),
		Time: time.Date(2026, 11, 11, 1, 2, 3, 4, time.UTC), Raw: json.RawMessage(`{ "x" : [1, 2] }`), Verbatim: jsonenc.Verbatim(`{"v":true}`),
		Kind: 2, Custom: ptrMarshal{N: 4}, Untagged: 9,
		Nested:    []Inner{{A: 1}, {A: 2, B: "&"}},
		Tree:      &Node{Name: "root", Children: []*Node{{Name: "leaf"}}},
		Interface: map[string]any{"s": []string(nil), "n": 1.5e-7, "inner": Inner{A: 3}},
	}
}

// same checks Convert against JSON.parse of jsonenc's JSON: the same
// values, the same key order, the same signs of zero.
func same(t *testing.T, v any) {
	t.Helper()
	rt := js.NewRealm()
	b, err := jsonenc.Marshal(v)
	if err != nil {
		if _, cerr := Convert(rt, v); cerr == nil {
			t.Fatalf("jsonenc failed (%v) where Convert didn't", err)
		}
		return
	}
	want, err := rt.ParseJSON(string(b))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Convert(rt, v)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if path, ok := equal(rt, got, want, "$"); !ok {
		gs, _ := rt.Stringify(got, js.Undefined, js.Undefined)
		t.Fatalf("differs at %s:\n got %s\nwant %s", path, gs, b)
	}
}

func equal(rt *js.Realm, a, b js.Value, path string) (string, bool) {
	ao, aok := a.(*js.Object)
	bo, bok := b.(*js.Object)
	if aok != bok {
		return path, false
	}
	if !aok {
		return path, js.SameValue(a, b)
	}
	if js.IsArray(ao) != js.IsArray(bo) || ao.Proto() != bo.Proto() {
		return path, false
	}
	ak, bk := ao.OwnKeys(false, true), bo.OwnKeys(false, true)
	if len(ak) != len(bk) {
		return path + " (keys)", false
	}
	for i := range ak {
		if ak[i] != bk[i] {
			return path + " (key order)", false
		}
		if p, ok := equal(rt, rt.Get(ao, ak[i]), rt.Get(bo, bk[i]), path+"."+js.ToString(ak[i])); !ok {
			return p, false
		}
	}
	return "", true
}

func TestMatchesJSON(t *testing.T) {
	v := sample()
	same(t, v)
	same(t, &v)
	same(t, []Everything{v, {}})
	same(t, map[string]Everything{"a": v})
	for _, x := range []any{nil, true, 0, -1, 1 << 53, 1<<53 + 1, uint64(math.MaxUint64), 0.1, float32(0.1), float32(16777217), math.Copysign(0, -1), "s", []byte(nil), []any{}, map[string]any{}, struct{}{}} {
		same(t, x)
	}
	n := &Node{Name: "loop"}
	n.Children = []*Node{n}
	if _, err := Convert(js.NewRealm(), n); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := Convert(js.NewRealm(), math.NaN()); err == nil {
		t.Fatal("NaN converted")
	}
	if _, err := Convert(js.NewRealm(), map[string]any{"f": func() {}}); err == nil {
		t.Fatal("func converted")
	}
}

func FuzzStrings(f *testing.F) {
	for _, s := range []string{"", "a", "<>&", "\u2028", "\xff\xfe", "é\x00", "\xed\xa0\x80"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		same(t, s)
		same(t, map[string]string{s: s})
		same(t, struct {
			S string `json:"s"`
		}{s})
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
	rt := js.NewRealm()
	b.Run("convert", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = Convert(rt, v)
		}
	})
	data, _ := jsonenc.Marshal(v)
	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = rt.ParseJSON(string(data))
		}
	})
}
