package main

import (
	"image"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/ocr"
)

// sampleText is two rows: "Hello" and "世界" side by side, then "第二行".
func sampleText() ocr.Result {
	return ocr.Result{Lines: []ocr.Line{
		{Text: "第二行", Box: ocr.Rect{X: 0.1, Y: 0.6, W: 0.3, H: 0.2}},
		{Text: "世界", Box: ocr.Rect{X: 0.6, Y: 0.1, W: 0.2, H: 0.2}},
		{Text: "Hello", Box: ocr.Rect{X: 0.1, Y: 0.1, W: 0.4, H: 0.2}},
	}}
}

func TestTextLayer(t *testing.T) {
	tl := newTextLayer(sampleText())
	if got := tl.text(tl.all()); got != "Hello 世界\n第二行" {
		t.Fatalf("all: %q", got)
	}
	// "Hello" runs from 0.1 to 0.5, its letters as wide as the font has
	// them: "H" wider than "l". A point just right of a boundary is after
	// it.
	box := tl.boxes[0]
	if box[0].W <= box[2].W || math.Abs(box[4].X+box[4].W-0.5) > 1e-9 {
		t.Fatalf("boxes %+v", box)
	}
	afterHe := box[2].X + 0.005
	if c := tl.caretAt(afterHe, 0.2); c != (caret{0, 2}) {
		t.Fatalf("caret %+v", c)
	}
	// Backwards from the middle of "第二行" to after "He".
	a, b := tl.caretAt(0.21, 0.7), tl.caretAt(afterHe, 0.15)
	if got := tl.text(a, b); got != "llo 世界\n第" {
		t.Fatalf("selection %q", got)
	}
	rects := tl.rects(a, b)
	if len(rects) != 3 || rects[0].X != box[2].X {
		t.Fatalf("rects %+v", rects)
	}
	// Beside a line counts as on it; far from any is not.
	if !tl.on(0.09, 0.2) || tl.on(0.95, 0.95) {
		t.Fatal("on")
	}
	// A point right of everything in the second row selects to its end.
	if c := tl.caretAt(0.9, 0.7); c != tl.lineEnd(2) {
		t.Fatalf("end caret %+v", c)
	}
}

func testPin() (*pin, *ui.Tester) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	p := &pin{a: &app{}, img: img, bmp: ui.NewBitmap(img), w: 200, h: 100, zoom: 1, opacity: 1}
	p.text = newTextLayer(sampleText())
	return p, ui.NewTester(p.view, 200, 100)
}

func TestPinSelectText(t *testing.T) {
	p, tt := testPin()
	tt.Frame()
	// Drag from after "He" into "世界", 0.75 of 200.
	tt.Press(float32(p.text.boxes[0][2].X+0.005)*200, 20)
	tt.Move(100, 20)
	tt.Release(150, 20)
	if got := p.text.text(p.anchor, p.focus); got != "llo 世" {
		t.Fatalf("selected %q", got)
	}
	if p.dragging {
		t.Fatal("selecting dragged the pin")
	}
	if tt.Cursor() != ui.CursorText {
		t.Fatalf("cursor over text %v", tt.Cursor())
	}
	// Esc lets go of the selection, not of the pin.
	tt.Key(0, ui.KeyEscape)
	if p.hasSelection() {
		t.Fatal("Esc kept the selection")
	}
	// ⌘A selects everything; a double click a line.
	tt.Key(ui.Cmd, ui.KeyA)
	if got := p.text.text(p.anchor, p.focus); got != "Hello 世界\n第二行" {
		t.Fatalf("⌘A selected %q", got)
	}
	tt.ClickAt(40, 70)
	tt.ClickAt(40, 70)
	if got := p.text.text(p.anchor, p.focus); got != "第二行" {
		t.Fatalf("double click selected %q", got)
	}
}
