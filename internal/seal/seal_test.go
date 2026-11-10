package seal

import "testing"

func TestSeal(t *testing.T) {
	a, err := New("0123456789abcdef0123456789abcdef", "a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New("0123456789abcdef0123456789abcdef", "b")
	s := a.Seal([]byte("hello"))
	if got, err := a.Open(s); err != nil || string(got) != "hello" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := b.Open(s); err == nil {
		t.Error("another purpose opened the value")
	}
	if _, err := a.Open(s[:len(s)-2] + "AA"); err == nil {
		t.Error("a changed value opened")
	}
	if _, err := New("short", "a"); err == nil {
		t.Error("a short secret was accepted")
	}
}
