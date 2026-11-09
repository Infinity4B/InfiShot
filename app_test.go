package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestShortcutLabel(t *testing.T) {
	for acc, want := range map[string]string{
		"Cmd+Ctrl+A":      "⌃⌘A",
		"Ctrl+Shift+O":    "⌃⇧O",
		"Alt+Cmd+shift+v": "⌥⇧⌘V",
		"F1":              "F1",
	} {
		if got := shortcutLabel(acc); got != want {
			t.Errorf("%q: %q, want %q", acc, got, want)
		}
	}
}

func TestEncodePNGMarksSRGB(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Pix[0], img.Pix[3] = 200, 255
	data, err := encodePNG(img, png.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	if i := bytes.Index(data, []byte("sRGB")); i < 0 || i > bytes.Index(data, []byte("IDAT")) {
		t.Fatalf("no sRGB chunk before the data")
	}
	back, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := back.At(0, 0).RGBA(); r>>8 != 200 {
		t.Fatalf("pixel %v", back.At(0, 0))
	}
}

func TestSnapped(t *testing.T) {
	if got := (rect{10.4, 20.6, 30.2, 9.8}).snapped(); got != (rect{10, 21, 31, 9}) {
		t.Fatalf("snapped %+v", got)
	}
}
