package storage

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		img.Set(x, x, color.RGBA{R: 200, A: 255})
	}
	return img
}

func segment(marker byte, data string) []byte {
	b := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(b[2:], uint16(len(data)+2))
	return append(b, data...)
}

func chunk(typ, data string) []byte {
	b := make([]byte, 8, 12+len(data))
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], typ)
	b = append(b, data...)
	return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE([]byte(typ+data)))
}

func TestStripJPEG(t *testing.T) {
	var enc bytes.Buffer
	if err := jpeg.Encode(&enc, testImage(), nil); err != nil {
		t.Fatal(err)
	}
	orig := enc.Bytes()
	// After SOI: an EXIF segment with a location, an ICC profile, and a comment.
	var withMeta []byte
	withMeta = append(withMeta, orig[:2]...)
	withMeta = append(withMeta, segment(0xE1, "Exif\x00\x00GPSLatitude 52.52")...)
	withMeta = append(withMeta, segment(0xE2, "ICC_PROFILE\x00profile")...)
	withMeta = append(withMeta, segment(0xFE, "taken at home")...)
	withMeta = append(withMeta, orig[2:]...)

	out, err := stripMetadata("image/jpeg", withMeta)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("GPSLatitude")) || bytes.Contains(out, []byte("taken at home")) {
		t.Error("metadata left in")
	}
	if !bytes.Contains(out, []byte("ICC_PROFILE")) {
		t.Error("the color profile was removed")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("the stripped image doesn't decode: %v", err)
	}
	if _, err := stripMetadata("image/jpeg", []byte("GIF89a")); err == nil {
		t.Error("a file that isn't a JPEG")
	}
}

func TestStripPNG(t *testing.T) {
	var enc bytes.Buffer
	if err := png.Encode(&enc, testImage()); err != nil {
		t.Fatal(err)
	}
	orig := enc.Bytes()
	// Before IEND, the last 12 bytes.
	end := len(orig) - 12
	withMeta := append(append(append(append([]byte{}, orig[:end]...), chunk("tEXt", "Author\x00Jane Doe")...), chunk("eXIf", "MM\x00*GPS")...), orig[end:]...)
	out, err := stripMetadata("image/png", withMeta)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("Jane Doe")) || bytes.Contains(out, []byte("eXIf")) {
		t.Error("metadata left in")
	}
	if !bytes.Equal(out, orig) {
		t.Error("the image itself changed")
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("the stripped image doesn't decode: %v", err)
	}
}
