package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// TestDecodeUntagged checks that an image without a profile, taken to be
// in sRGB, keeps its pixels.
func TestDecodeUntagged(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 4), uint8(y * 8), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	got, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != src.Bounds() {
		t.Fatalf("bounds %v", got.Bounds())
	}
	for y := range 32 {
		for x := range 64 {
			a, b := src.NRGBAAt(x, y), got.RGBAAt(x, y)
			if d := max(diff(a.R, b.R), diff(a.G, b.G), diff(a.B, b.B)); d > 1 || b.A != 255 {
				t.Fatalf("(%d, %d): %v became %v", x, y, a, b)
			}
		}
	}
}

func TestDecodeGarbage(t *testing.T) {
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Fatal("no error")
	}
	if _, err := Decode(nil); err == nil {
		t.Fatal("no error for nothing")
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
