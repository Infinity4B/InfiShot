//go:build darwin

package capture

import (
	"os"
	"testing"
)

// TestDisplays captures every display whole, those left of or above the
// main one too, whose coordinates are negative.
func TestDisplays(t *testing.T) {
	if os.Getenv("INFISHOT_CAPTURE") == "" {
		t.Skip("set INFISHOT_CAPTURE=1 to capture the screen")
	}
	ds, err := displays()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		b := d.bounds
		r := Rect{X: int(b.X), Y: int(b.Y), W: int(b.W), H: int(b.H)}
		img, err := Region(r)
		if err != nil {
			t.Fatalf("display %d %+v: %v", d.index, r, err)
		}
		t.Logf("display %d %+v: %v", d.index, r, img.Bounds().Size())
		if w := img.Bounds().Dx(); w < r.W {
			t.Fatalf("display %d: width %d < %d", d.index, w, r.W)
		}
	}
}
