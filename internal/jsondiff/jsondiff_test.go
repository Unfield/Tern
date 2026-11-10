package jsondiff

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
)

// apply is the reference for what the browser does with a patch. Values
// come from Decode, so numbers are json.Number.
func apply(root any, ops []any) any {
	for _, o := range ops {
		op := o.([]any)
		path := op[1].([]any)
		if len(path) == 0 {
			if op[0] == "s" {
				root = op[2]
			} else {
				root = splice(root.([]any), num(op[2]), num(op[3]), op[4].([]any))
			}
			continue
		}
		parent := root
		for _, k := range path[:len(path)-1] {
			parent = child(parent, k)
		}
		last := path[len(path)-1]
		switch op[0] {
		case "s":
			set(parent, last, op[2])
		case "d":
			delete(parent.(map[string]any), last.(string))
		case "a":
			arr := child(parent, last).([]any)
			set(parent, last, splice(arr, num(op[2]), num(op[3]), op[4].([]any)))
		}
	}
	return root
}

func num(v any) int {
	n, _ := v.(json.Number).Int64()
	return int(n)
}

func splice(arr []any, start, del int, items []any) []any {
	out := append([]any{}, arr[:start]...)
	out = append(out, items...)
	return append(out, arr[start+del:]...)
}

func child(v, k any) any {
	if s, ok := k.(string); ok {
		return v.(map[string]any)[s]
	}
	return v.([]any)[num(k)]
}

func set(v, k, x any) {
	if s, ok := k.(string); ok {
		v.(map[string]any)[s] = x
		return
	}
	v.([]any)[num(k)] = x
}

func TestDiff(t *testing.T) {
	cases := []struct{ a, b string }{
		{`1`, `2`},
		{`{"a":1,"b":2}`, `{"a":1,"b":3,"c":4}`},
		{`{"a":1,"b":2}`, `{"a":1}`},
		{`[1,2,3]`, `[1,2,3,4]`},
		{`[1,2,3]`, `[0,1,2,3]`},
		{`[1,2,3,4,5]`, `[1,2,4,5]`},
		{`[{"id":1,"done":false},{"id":2,"done":false}]`, `[{"id":1,"done":false},{"id":2,"done":true}]`},
		{`[{"id":1},{"id":2},{"id":3}]`, `[{"id":1},{"id":3}]`},
		{`{"list":[1,2],"x":{"y":[true]}}`, `{"list":[1,2,3],"x":{"y":[false]}}`},
		{`[1,2]`, `[]`},
		{`[]`, `[1]`},
		{`"a"`, `["a"]`},
		{`{"n":12345678901234567890}`, `{"n":12345678901234567891}`},
	}
	for _, c := range cases {
		a, _ := Decode([]byte(c.a))
		b, _ := Decode([]byte(c.b))
		// The patch goes to the browser as JSON.
		raw, _ := json.Marshal(Diff(a, b))
		decoded, _ := Decode(raw)
		ops, _ := decoded.([]any)
		got := apply(a, ops)
		if !reflect.DeepEqual(got, b) {
			t.Errorf("%s → %s: patch %s gives %v", c.a, c.b, raw, got)
		}
	}
	a, _ := Decode([]byte(`[1,2]`))
	if ops := Diff(a, a); ops != nil {
		t.Errorf("equal values: %v", ops)
	}
	// Appending to a long list sends only the new item.
	var long []any
	for i := range 1000 {
		long = append(long, map[string]any{"id": json.Number(strconv.Itoa(i))})
	}
	longer := append(append([]any{}, long...), map[string]any{"id": json.Number("1000")})
	raw, _ := json.Marshal(Diff(long, longer))
	if len(raw) > 60 {
		t.Errorf("append patch is %d bytes: %s", len(raw), raw)
	}
}
