package signals

import (
	"crypto/tls"
	"testing"
)

// The example in FoxIO's JA4 documentation: Chrome, t13d1516h2_8daaf6152771_e5627efa2ab1.
func TestJA4(t *testing.T) {
	h := &tls.ClientHelloInfo{
		ServerName: "example.com",
		CipherSuites: []uint16{0x1a1a, // GREASE
			0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc013, 0xc014, 0x009c, 0x009d, 0x002f, 0x0035},
		Extensions: []uint16{0x2a2a, // GREASE
			0x0000, 0x0017, 0xff01, 0x000a, 0x000b, 0x0023, 0x0010, 0x0005, 0x000d, 0x0012, 0x0033, 0x002d, 0x002b, 0x001b, 0x0015, 0x4469},
		SignatureSchemes:  []tls.SignatureScheme{0x0403, 0x0804, 0x0401, 0x0503, 0x0805, 0x0501, 0x0806, 0x0601},
		SupportedProtos:   []string{"h2", "http/1.1"},
		SupportedVersions: []uint16{0x3a3a, tls.VersionTLS13, tls.VersionTLS12},
	}
	got := ja4(h)
	if got.ja4 != "t13d1516h2_8daaf6152771_e5627efa2ab1" {
		t.Fatalf("ja4 = %s", got.ja4)
	}
	if !got.grease || got.maxTLS != tls.VersionTLS13 {
		t.Fatalf("%+v", got)
	}
}

func TestJA4Edges(t *testing.T) {
	got := ja4(&tls.ClientHelloInfo{SupportedVersions: []uint16{tls.VersionTLS12}})
	if got.ja4 != "t12i000000_000000000000_000000000000" {
		t.Fatal(got.ja4)
	}
	if alpnChars("http/1.1") != "h1" || alpnChars("h2") != "h2" || alpnChars("\xab") != "ab" {
		t.Fatal("alpn")
	}
	if !isGREASE(0xfafa) || isGREASE(0x0a1a) || isGREASE(0x1301) {
		t.Fatal("grease")
	}
}
