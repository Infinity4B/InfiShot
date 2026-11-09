package main

import (
	"image"
	"image/color"
	"testing"
)

// fill paints r, in DIPs, at scale 2.
func fill(img *image.RGBA, r rect, c color.RGBA) {
	for y := int(r.Y * 2); y < int(r.bottom()*2); y++ {
		for x := int(r.X * 2); x < int(r.right()*2); x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

// uiImage is 400 × 300 DIPs at scale 2: white, with a gray panel at
// (100, 50, 200, 150) holding a blue button with two letters on it, and
// a dark icon.
func uiImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	fill(img, rect{0, 0, 400, 300}, color.RGBA{255, 255, 255, 255})
	fill(img, rect{100, 50, 200, 150}, color.RGBA{230, 230, 230, 255})
	fill(img, rect{120, 70, 60, 20}, color.RGBA{30, 120, 250, 255})
	for _, x := range []float32{140, 147} { // letters of strokes a DIP wide
		fill(img, rect{x, 75, 6, 10}, color.RGBA{255, 255, 255, 255})
		fill(img, rect{x + 1, 76, 4, 8}, color.RGBA{30, 120, 250, 255})
	}
	fill(img, rect{220, 80, 16, 16}, color.RGBA{40, 40, 40, 255})
	return img
}

func hasRect(els []rect, want rect, d float32) bool {
	for _, e := range els {
		if e.near(want, d) {
			return true
		}
	}
	return false
}

func TestDetectElements(t *testing.T) {
	els := detectElements(uiImage(), 2)
	for _, want := range []struct {
		name string
		r    rect
		d    float32
	}{
		{"panel", rect{100, 50, 200, 150}, 0},
		{"button", rect{120, 70, 60, 20}, 0},
		{"icon", rect{220, 80, 16, 16}, 1},
		{"word", rect{140, 75, 13, 10}, 1},
	} {
		if !hasRect(els, want.r, want.d) {
			t.Errorf("no %s at %+v in %+v", want.name, want.r, els)
		}
	}
	// The letters are a word, not one each.
	if hasRect(els, rect{140, 75, 6, 10}, 1) {
		t.Error("a letter is an element of its own")
	}
}

func TestDilate(t *testing.T) {
	cells := []bool{false, false, true, false, false, false}
	got := dilate(cells, 6, 1, 1)
	want := []bool{false, true, true, true, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dilate = %v, want %v", got, want)
		}
	}
}

func TestOverlayHighlightsElements(t *testing.T) {
	o, tt := testOverlay()
	o.windows = []rect{{100, 50, 200, 150}}
	o.img = uiImage()
	o.setElements(detectElements(o.img, 2))

	// On the button's letters: the word, with room around it; the wheel
	// takes in the button, the panel, then the display.
	tt.Move(143, 80)
	if got := o.candidate(); !got.near(rect{140, 75, 13, 10}.pad(elementPad), 1) {
		t.Fatalf("highlighted %+v", got)
	}
	var seen []rect
	for range 6 {
		tt.Wheel(143, 80, 0, -1)
		seen = append(seen, o.candidate())
	}
	if !hasRect(seen, rect{120, 70, 60, 20}.pad(elementPad), 0) || seen[len(seen)-1] != o.bounds() {
		t.Fatalf("scrolling up highlighted %+v", seen)
	}
	tt.Wheel(143, 80, 0, 10) // and down, back to the smallest
	for range 6 {
		tt.Wheel(143, 80, 0, 1)
	}
	if got := o.candidate(); !got.near(rect{140, 75, 13, 10}.pad(elementPad), 1) {
		t.Fatalf("scrolled down to %+v", got)
	}

	// A click takes the icon under the pointer.
	tt.ClickAt(228, 88)
	if !o.has || !o.sel.near(rect{220, 80, 16, 16}.pad(elementPad), 1) {
		t.Fatalf("click selected %+v", o.sel)
	}
	// The room stops at the window's edge.
	o.has = false
	o.windows = []rect{{217, 77, 60, 60}}
	tt.Move(228, 88)
	if got := o.candidate(); got.X != 217 || got.Y != 77 || !(got.right() > 240 && got.right() < 245) {
		t.Fatalf("highlighted %+v across the window's edge", got)
	}
}
