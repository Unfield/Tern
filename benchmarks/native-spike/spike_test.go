package nativespike

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// Set NATIVE_PAYLOAD and NATIVE_BODY to the payload and the body QuickJS
// rendered for /items (see README.md).
func fixtures(tb testing.TB) (ItemsData, []byte, []byte) {
	payload, err1 := os.ReadFile(os.Getenv("NATIVE_PAYLOAD"))
	body, err2 := os.ReadFile(os.Getenv("NATIVE_BODY"))
	if err1 != nil || err2 != nil {
		tb.Skip("set NATIVE_PAYLOAD and NATIVE_BODY")
	}
	var p struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		tb.Fatal(err)
	}
	var d ItemsData
	if err := json.Unmarshal(p.Nodes[1], &d); err != nil {
		tb.Fatal(err)
	}
	return d, payload, body
}

func TestSameHTMLAsSvelte(t *testing.T) {
	d, _, want := fixtures(t)
	got := RenderItemsPage(nil, d)
	if string(got) != string(want) {
		for i := range min(len(got), len(want)) {
			if got[i] != want[i] {
				t.Fatalf("differs at byte %d:\n got  %q\n want %q", i, got[max(0, i-40):min(len(got), i+60)], want[max(0, i-40):min(len(want), i+60)])
			}
		}
		t.Fatalf("lengths differ: %d, want %d", len(got), len(want))
	}
}

func TestJSNumbers(t *testing.T) {
	tenth := 0.1 // not a constant, so 0.1 + 0.2 rounds as it does in JavaScript
	sum := tenth + 0.2
	for _, c := range []struct {
		x    float64
		d    int
		want string
	}{
		{1.005, 2, "1.00"}, {0.125, 2, "0.13"}, {2.5, 0, "3"}, {-2.5, 0, "-3"}, {-0.004, 2, "-0.00"}, {math.Copysign(0, -1), 2, "0.00"},
		{18715, 2, "18715.00"}, {1e21, 2, "1e+21"}, {99.995, 2, "100.00"}, {sum, 2, "0.30"},
	} {
		if got := jsToFixed(c.x, c.d); got != c.want {
			t.Errorf("(%v).toFixed(%d) = %q, want %q", c.x, c.d, got, c.want)
		}
	}
	for x, want := range map[float64]string{
		1: "1", -1.5: "-1.5", sum: "0.30000000000000004", 1e21: "1e+21", 123456789012345680000: "123456789012345680000",
		1e-7: "1e-7", 0.000001: "0.000001", 1.5e300: "1.5e+300", math.Copysign(0, -1): "0",
	} {
		if got := jsNumber(x); got != want {
			t.Errorf("String(%v) = %q, want %q", x, got, want)
		}
	}
}

func BenchmarkNativeFromStructs(b *testing.B) {
	d, _, _ := fixtures(b)
	buf := make([]byte, 0, 64<<10)
	for b.Loop() {
		buf = RenderItemsPage(buf[:0], d)
	}
}

func BenchmarkNativeFromJSON(b *testing.B) {
	_, payload, _ := fixtures(b)
	buf := make([]byte, 0, 64<<10)
	for b.Loop() {
		var p struct {
			Nodes []json.RawMessage `json:"nodes"`
		}
		var d ItemsData
		if json.Unmarshal(payload, &p) != nil || json.Unmarshal(p.Nodes[1], &d) != nil {
			b.Fatal("decode")
		}
		buf = RenderItemsPage(buf[:0], d)
	}
}

func BenchmarkNativeParallel(b *testing.B) {
	d, _, _ := fixtures(b)
	b.RunParallel(func(pb *testing.PB) {
		buf := make([]byte, 0, 64<<10)
		for pb.Next() {
			buf = RenderItemsPage(buf[:0], d)
		}
	})
}
