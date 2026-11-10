package js

import "testing"

// QuickJS has Uint8Array.fromBase64 and toBase64, which Node 24 lacks, so
// these are checked against known answers.
func TestUint8Base64(t *testing.T) {
	u8 := g("Uint8Array")
	from := func(s string, opts Value) string {
		return ToString(m(m(u8, "fromBase64", s, opts), "join"))
	}
	if got := from("aGk=", Undefined); got != "104,105" {
		t.Errorf("fromBase64 padded: %s", got)
	}
	if got := from(" aG k ", Undefined); got != "104,105" {
		t.Errorf("fromBase64 loose with spaces: %s", got)
	}
	if got := from("aGl", Undefined); got != "104,105" {
		t.Errorf("fromBase64 loose, no zero bits: %s", got)
	}
	opts := NewPlainObject()
	opts.Set("alphabet", "base64url")
	if got := from("-_8", opts); got != "251,255" {
		t.Errorf("fromBase64 url: %s", got)
	}
	strict := NewPlainObject()
	strict.Set("lastChunkHandling", "strict")
	if _, err := catch(func() { m(u8, "fromBase64", "aGl", strict) }); err == nil {
		t.Error("strict decoding should reject a chunk with non-zero bits")
	}
	a := Construct(u8, arr(104.0, 105.0, 255.0))
	if got := m(a, "toBase64"); got != "aGn/" {
		t.Errorf("toBase64: %v", got)
	}
	o := NewPlainObject()
	o.Set("alphabet", "base64url")
	o.Set("omitPadding", true)
	if got := m(m(a, "subarray", 0.0, 2.0), "toBase64", o); got != "aGk" {
		t.Errorf("toBase64 url unpadded: %v", got)
	}
	if got := m(a, "toHex"); got != "6869ff" {
		t.Errorf("toHex: %v", got)
	}
}

func catch(f func()) (v Value, err *Throw) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(*Throw)
		}
	}()
	f()
	return nil, nil
}
