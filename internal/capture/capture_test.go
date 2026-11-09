package capture

import (
	"errors"
	"image"
	"image/color"
	"os"
	"testing"
)

func TestCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	src.Set(3, 4, color.RGBA{R: 255, A: 255})
	got := Crop(src, image.Rect(3, 4, 20, 20))
	if got.Bounds() != image.Rect(0, 0, 7, 6) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if c := got.RGBAAt(0, 0); c.R != 255 {
		t.Fatalf("pixel = %v", c)
	}
	got.Set(1, 1, color.White)
	if src.RGBAAt(4, 5) == (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("Crop shares pixels with its source")
	}
}

func TestRegionEmpty(t *testing.T) {
	if _, err := Region(Rect{W: 0, H: 10}); err == nil {
		t.Fatal("empty region succeeded")
	}
}

// TestRegion captures the screen; it needs a desktop session, so it runs
// only with INFISHOT_CAPTURE=1.
func TestRegion(t *testing.T) {
	if os.Getenv("INFISHOT_CAPTURE") == "" {
		t.Skip("set INFISHOT_CAPTURE=1 to capture the screen")
	}
	img, err := Region(Rect{X: 0, Y: 0, W: 200, H: 100})
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if w := img.Bounds().Dx(); w != 200 && w != 400 {
		t.Fatalf("width = %d, want 200 or 400 (Retina)", w)
	}
}

func TestWindows(t *testing.T) {
	if os.Getenv("INFISHOT_CAPTURE") == "" {
		t.Skip("set INFISHOT_CAPTURE=1 to list the windows on screen")
	}
	ws, err := Windows()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		t.Logf("%+v", w)
	}
}
