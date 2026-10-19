package mnml

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
)

// Image is an image, sent in the same call that uses it, up to 15 MB: a public https:// link, a
// data:image/…;base64, URI or base64 text. ImageFromBytes and ImageFromFile make one from the
// image's bytes. The API checks the image again, so the type read off the first bytes is only a
// label.
type Image string

// ImageFromBytes is the image's bytes (JPEG, PNG or WebP), sent as a data URI.
func ImageFromBytes(b []byte) Image {
	return Image("data:" + sniff(b) + ";base64," + base64.StdEncoding.EncodeToString(b))
}

// ImageFromFile is the image in a file on disk.
func ImageFromFile(path string) (Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ImageFromBytes(b), nil
}

// ImageFromReader is the image read from r to its end.
func ImageFromReader(r io.Reader) (Image, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return ImageFromBytes(b), nil
}

func sniff(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	case len(b) >= 4 && bytes.Equal(b[:4], []byte{0x89, 'P', 'N', 'G'}):
		return "image/png"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	}
	return "application/octet-stream"
}
