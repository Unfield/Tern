package signals

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// helloInfo is what the plugin keeps of a TLS ClientHello.
type helloInfo struct {
	ja4    string
	grease bool     // the client sent GREASE values, as Chromium and Safari do
	alpn   []string // the protocols offered
	maxTLS uint16
}

// isGREASE reports whether v is one of the reserved GREASE values
// (RFC 8701): 0x0a0a, 0x1a1a, … 0xfafa.
func isGREASE(v uint16) bool { return v&0x0f0f == 0x0a0a && v>>8 == v&0xff }

// ja4 computes the JA4 TLS client fingerprint (FoxIO's JA4, BSD-3-Clause
// spec) from a ClientHello: protocol, version, SNI, counts and ALPN, then
// truncated hashes of the sorted cipher suites and of the sorted
// extensions with the signature algorithms.
func ja4(h *tls.ClientHelloInfo) helloInfo {
	info := helloInfo{alpn: h.SupportedProtos}
	var ciphers, exts []uint16
	for _, c := range h.CipherSuites {
		if isGREASE(c) {
			info.grease = true
			continue
		}
		ciphers = append(ciphers, c)
	}
	sawSNI, sawALPN := false, false
	for _, e := range h.Extensions {
		if isGREASE(e) {
			info.grease = true
			continue
		}
		switch e {
		case 0x0000:
			sawSNI = true
		case 0x0010:
			sawALPN = true
		}
		exts = append(exts, e)
	}
	for _, v := range h.SupportedVersions {
		if isGREASE(v) {
			info.grease = true
			continue
		}
		info.maxTLS = max(info.maxTLS, v)
	}

	version := "00"
	switch info.maxTLS {
	case tls.VersionTLS13:
		version = "13"
	case tls.VersionTLS12:
		version = "12"
	case tls.VersionTLS11:
		version = "11"
	case tls.VersionTLS10:
		version = "10"
	}
	sni := "i"
	if sawSNI && h.ServerName != "" {
		sni = "d"
	}
	alpn := "00"
	if sawALPN && len(h.SupportedProtos) > 0 {
		alpn = alpnChars(h.SupportedProtos[0])
	}
	a := fmt.Sprintf("t%s%s%02d%02d%s", version, sni, min(len(ciphers), 99), min(len(exts), 99), alpn)

	sorted := slices.Clone(ciphers)
	slices.Sort(sorted)
	b := hash12(hexList(sorted))

	var rest []uint16
	for _, e := range exts {
		if e != 0x0000 && e != 0x0010 {
			rest = append(rest, e)
		}
	}
	slices.Sort(rest)
	c := hexList(rest)
	var sigs []uint16
	for _, s := range h.SignatureSchemes {
		if !isGREASE(uint16(s)) {
			sigs = append(sigs, uint16(s))
		}
	}
	if len(sigs) > 0 {
		c += "_" + hexList(sigs)
	}
	if len(rest) == 0 && len(sigs) == 0 {
		c = ""
	}
	info.ja4 = a + "_" + b + "_" + hash12(c)
	return info
}

// alpnChars is the first and last character of the first ALPN value, or
// the first and last hex digits when either isn't alphanumeric.
func alpnChars(p string) string {
	if p == "" {
		return "00"
	}
	first, last := p[0], p[len(p)-1]
	if alnum(first) && alnum(last) {
		return string([]byte{first, last})
	}
	h := hex.EncodeToString([]byte(p))
	return h[:1] + h[len(h)-1:]
}

func alnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func hexList(vs []uint16) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprintf("%04x", v)
	}
	return strings.Join(parts, ",")
}

// hash12 is the first 12 hex digits of s's SHA-256, or twelve zeros for
// an empty s.
func hash12(s string) string {
	if s == "" {
		return "000000000000"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
