package nilslices

import (
	"encoding/json"
	"testing"
	"time"
)

type nsItem struct {
	Tags []string
	Raw  []byte
}

type nsData struct {
	Items   []nsItem
	Names   []string
	Ptr     *nsItem
	ByKey   map[string][]int
	Any     any
	When    time.Time
	Skip    []string `json:"-"`
	private []string
}

func TestWithoutNilSlices(t *testing.T) {
	shared := []nsItem{{}} // a nil slice inside, in data kept across requests
	in := nsData{private: nil, Items: shared, Ptr: &nsItem{}, ByKey: map[string][]int{"a": nil}, Any: map[string]any{"x": []string(nil)}}
	b, err := json.Marshal(Remove(in))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Items":[{"Tags":[],"Raw":null}],"Names":[],"Ptr":{"Tags":[],"Raw":null},"ByKey":{"a":[]},"Any":{"x":[]},"When":"0001-01-01T00:00:00Z"}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
	_ = in.private // unexported fields aren't encoded, so they're left alone
	if shared[0].Tags != nil || in.Ptr.Tags != nil || in.ByKey["a"] != nil {
		t.Fatal("the original data was changed")
	}
	// Data without slices isn't copied.
	type plain struct{ A, B int }
	p := plain{1, 2}
	if Remove(p) != any(p) {
		t.Fatal("copied data with no slices")
	}
}
