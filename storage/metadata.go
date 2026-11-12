package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// errNotImage is a file that doesn't parse as the image its type says.
var errNotImage = errors.New("storage: the image is damaged")

// stripMetadata removes what a photo says about where and when it was
// taken, and by what: EXIF (with GPS coordinates), XMP, IPTC and comments
// in JPEGs, and the eXIf and text chunks of PNGs. The pixels and the color
// profile stay as they are, byte for byte. Other types are returned as
// they are.
func stripMetadata(contentType string, b []byte) ([]byte, error) {
	switch contentType {
	case "image/jpeg":
		return stripJPEG(b)
	case "image/png":
		return stripPNG(b)
	}
	return b, nil
}

func stripJPEG(b []byte) ([]byte, error) {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return nil, errNotImage
	}
	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	out.Write(b[:2])
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return nil, errNotImage
		}
		marker := b[i+1]
		if marker == 0xFF { // padding
			i++
			continue
		}
		if marker == 0xDA || marker == 0xD9 { // start of scan, or end: the rest is image data
			out.Write(b[i:])
			return out.Bytes(), nil
		}
		if marker >= 0xD0 && marker <= 0xD7 || marker == 0x01 { // no length
			out.Write(b[i : i+2])
			i += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return nil, errNotImage
		}
		switch marker {
		case 0xE1, 0xED, 0xFE: // APP1 (EXIF, XMP), APP13 (IPTC), comments
		default:
			out.Write(b[i : i+2+n])
		}
		i += 2 + n
	}
	return nil, errNotImage
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func stripPNG(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, pngSignature) {
		return nil, errNotImage
	}
	out := bytes.NewBuffer(make([]byte, 0, len(b)))
	out.Write(pngSignature)
	i := len(pngSignature)
	for i+12 <= len(b) {
		n := int(binary.BigEndian.Uint32(b[i:]))
		end := i + 12 + n
		if n < 0 || end > len(b) {
			return nil, errNotImage
		}
		switch string(b[i+4 : i+8]) {
		case "eXIf", "tEXt", "zTXt", "iTXt":
		default:
			out.Write(b[i:end])
		}
		if string(b[i+4:i+8]) == "IEND" {
			return out.Bytes(), nil
		}
		i = end
	}
	return nil, errNotImage
}
