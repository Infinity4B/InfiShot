package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/capture"
)

// testOverlay is an overlay without a window, over a display of 400 × 300
// DIPs at scale 2, with one window at (50, 40) of 200 × 100.
func testOverlay() (*overlay, *ui.Tester) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	o := &overlay{
		a:       &app{},
		display: mygo.Display{Bounds: mygo.Rectangle{Width: 400, Height: 300}, ScaleFactor: 2},
		img:     img,
		bmp:     ui.NewBitmap(img),
		scale:   2,
		windows: []rect{{50, 40, 200, 100}},
		ptr:     ui.Point{X: -1, Y: -1},
		size:    1,
	}
	o.mosaic = mosaics{img: img, scale: 2}
	return o, ui.NewTester(o.view, 400, 300)
}

func TestOverlayDragSelects(t *testing.T) {
	o, tt := testOverlay()
	if _, ok := tt.Find("复制到剪贴板 (↩ 或 ⌘C)"); ok {
		t.Fatal("toolbar shown before a selection")
	}
	tt.Press(20, 20)
	tt.Move(60, 50)
	tt.Move(100, 80)
	tt.Release(100, 80)
	if !o.has || o.sel != (rect{20, 20, 80, 60}) {
		t.Fatalf("selection = %+v (made %v)", o.sel, o.has)
	}
	if _, ok := tt.Find("复制到剪贴板 (↩ 或 ⌘C)"); !ok {
		t.Fatalf("no toolbar: %q", tt.Texts())
	}
	if got := pixels(o.sel, o.scale, o.img.Bounds().Size()); got != image.Rect(40, 40, 200, 160) {
		t.Fatalf("pixels = %v", got)
	}
}

func TestOverlayClickTakesWindow(t *testing.T) {
	o, tt := testOverlay()
	tt.ClickAt(100, 60)
	if !o.has || o.sel != (rect{50, 40, 200, 100}) {
		t.Fatalf("click in a window selected %+v", o.sel)
	}
	// Outside every window, a click takes the whole display.
	tt.PressButton(1, 5, 5) // right-click lets go of the selection
	tt.ReleaseButton(1, 5, 5)
	if o.has {
		t.Fatal("right-click kept the selection")
	}
	tt.ClickAt(350, 250)
	if o.sel != (rect{0, 0, 400, 300}) {
		t.Fatalf("click outside windows selected %+v", o.sel)
	}
}

func TestOverlayEditSelection(t *testing.T) {
	o, tt := testOverlay()
	o.sel, o.has = rect{100, 100, 100, 50}, true
	tt.Frame()

	// Drag the bottom-right corner.
	tt.Press(200, 150)
	tt.Move(220, 170)
	tt.Release(240, 180)
	if o.sel != (rect{100, 100, 140, 80}) {
		t.Fatalf("resized to %+v", o.sel)
	}
	// Drag the inside.
	tt.Press(150, 120)
	tt.Move(140, 120)
	tt.Release(130, 110)
	if o.sel != (rect{80, 90, 140, 80}) {
		t.Fatalf("moved to %+v", o.sel)
	}
	// Arrows nudge it; Shift+arrows grow it.
	tt.Key(0, ui.KeyRight)
	tt.Key(ui.Shift, ui.KeyDown)
	if o.sel != (rect{81, 90, 140, 81}) {
		t.Fatalf("nudged to %+v", o.sel)
	}
}

// selected is testOverlay with the area (100, 100, 200, 120) selected.
func selected(t *testing.T) (*overlay, *ui.Tester) {
	o, tt := testOverlay()
	o.sel, o.has = rect{100, 100, 200, 120}, true
	tt.Frame()
	return o, tt
}

func TestOverlayDrawShapes(t *testing.T) {
	o, tt := selected(t)
	tt.Key(0, ui.KeyR)
	if o.tool != toolRect {
		t.Fatalf("R chose %v", o.tool)
	}
	tt.Press(120, 120)
	tt.Move(150, 140)
	tt.Release(180, 160)
	if len(o.shapes) != 1 || o.shapes[0].box() != (rect{120, 120, 60, 40}) {
		t.Fatalf("shapes %+v", o.shapes)
	}
	if o.sel != (rect{100, 100, 200, 120}) {
		t.Fatalf("drawing moved the selection to %+v", o.sel)
	}
	// Strokes stay in the selection.
	tt.Key(0, ui.KeyP)
	tt.Press(150, 150)
	tt.Move(170, 150)
	tt.Move(400, 290)
	tt.Release(400, 290)
	pen := o.shapes[1]
	if pen.tool != toolPen || len(pen.pts) != 3 || pen.pts[2] != (ui.Point{X: 300, Y: 220}) {
		t.Fatalf("pen %+v", pen)
	}
	// A click with the rectangle makes nothing.
	tt.Key(0, ui.KeyR)
	tt.ClickAt(200, 200)
	if len(o.shapes) != 2 {
		t.Fatalf("a click drew %+v", o.shapes[2:])
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if len(o.shapes) != 1 {
		t.Fatalf("undo left %d shapes", len(o.shapes))
	}
	// Right-click lets go of the tool, then keeps the marked selection.
	tt.RightClickAt(50, 50)
	if o.tool != toolNone || !o.has {
		t.Fatalf("right-click: tool %v, selection %v", o.tool, o.has)
	}
	tt.RightClickAt(50, 50)
	if !o.has {
		t.Fatal("right-click dropped a marked selection")
	}
	// So does a press outside it.
	tt.Press(20, 20)
	tt.Release(60, 60)
	if o.sel != (rect{100, 100, 200, 120}) {
		t.Fatalf("selection became %+v", o.sel)
	}
}

func TestOverlayToolbarPicksTool(t *testing.T) {
	o, tt := selected(t)
	if err := tt.Click("箭头 (A)"); err != nil {
		t.Fatal(err)
	}
	if o.tool != toolArrow {
		t.Fatalf("tool %v", o.tool)
	}
	if err := tt.Click("颜色 4"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("粗"); err != nil {
		t.Fatal(err)
	}
	tt.Press(120, 200)
	tt.Release(250, 130)
	s := o.shapes[0]
	if s.color != palette[3] || s.size != sizes[toolArrow][2] {
		t.Fatalf("arrow %+v", s)
	}
}

func TestOverlayText(t *testing.T) {
	o, tt := selected(t)
	tt.Key(0, ui.KeyT)
	tt.ClickAt(150, 150)
	if o.editing == nil {
		t.Fatal("no text started")
	}
	tt.Type("你好 InfiShot")
	tt.Key(0, ui.KeyEnter)
	if len(o.shapes) != 1 || o.shapes[0].text != "你好 InfiShot" || o.editing != nil {
		t.Fatalf("shapes %+v, editing %v", o.shapes, o.editing)
	}
	if o.done {
		t.Fatal("Enter in the text finished the capture")
	}
	// Esc ends a text too, and an empty one is dropped.
	tt.ClickAt(150, 190)
	tt.Key(0, ui.KeyEscape)
	if o.done || o.editing != nil || len(o.shapes) != 1 {
		t.Fatalf("Esc: done %v, editing %v, %d shapes", o.done, o.editing, len(o.shapes))
	}
}

func TestAnnotated(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7 / 3)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	sel := rect{100, 50, 100, 80}
	shot := capture.Crop(img, pixels(sel, 2, img.Bounds().Size()))
	red := &shape{tool: toolRect, color: palette[0], size: 4, pts: []ui.Point{{X: 120, Y: 70}, {X: 170, Y: 110}}}
	m := &mosaics{img: img, scale: 2}
	out := annotated(shot, sel, 2, []*shape{red}, m)
	if out.Bounds() != shot.Bounds() {
		t.Fatalf("bounds %v, want %v", out.Bounds(), shot.Bounds())
	}
	// On the rectangle's left edge, at (20, 40) in the selection.
	if c := out.RGBAAt(40, 80); c.R < 240 || c.G > 80 || c.B > 80 {
		t.Fatalf("edge pixel %v is not red", c)
	}
	// Away from it, the screenshot as it was.
	for _, p := range []image.Point{{4, 4}, {100, 80}, {190, 150}} {
		if out.RGBAAt(p.X, p.Y) != shot.RGBAAt(p.X, p.Y) {
			t.Fatalf("%v changed: %v → %v", p, shot.RGBAAt(p.X, p.Y), out.RGBAAt(p.X, p.Y))
		}
	}
	if annotated(shot, sel, 2, nil, m) != shot {
		t.Fatal("no shapes made a new image")
	}
}

func TestMosaicCells(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 30, 10))
	for x := range 30 {
		for y := range 10 {
			img.Pix[y*img.Stride+x*4] = uint8(x * 8)
			img.Pix[y*img.Stride+x*4+3] = 255
		}
	}
	m := &mosaics{img: img, scale: 2}
	_, r := m.at(5) // cells of 10 pixels
	if r != (ui.Rect{W: 15, H: 5}) {
		t.Fatalf("covers %v", r)
	}
	_, r = m.at(4) // 8 pixels: 4 × 2 cells, past the image
	if r != (ui.Rect{W: 16, H: 8}) {
		t.Fatalf("covers %v", r)
	}
}

func TestAnnotatedMosaic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 13)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	sel := rect{100, 50, 100, 80}
	shot := capture.Crop(img, pixels(sel, 2, img.Bounds().Size()))
	// Cells of 10 DIPs: 20 pixels, from the screenshot's corner.
	m := &mosaics{img: img, scale: 2}
	s := &shape{tool: toolMosaic, size: 10, pts: []ui.Point{{X: 110, Y: 60}, {X: 150, Y: 100}}}
	out := annotated(shot, sel, 2, []*shape{s}, m)
	// The cell at (120, 60)–(130, 70) DIPs is (40, 20)–(60, 40) in the
	// selection's pixels: one color, away from its edges.
	c := out.RGBAAt(44, 24)
	for _, p := range []image.Point{{55, 24}, {44, 35}, {55, 35}} {
		if out.RGBAAt(p.X, p.Y) != c {
			t.Fatalf("cell not flat: %v at (44, 24), %v at %v", c, out.RGBAAt(p.X, p.Y), p)
		}
	}
	if c == shot.RGBAAt(44, 24) && c == shot.RGBAAt(55, 35) {
		t.Fatal("mosaic left the pixels as they were")
	}
	// Outside the box, untouched.
	if out.RGBAAt(150, 150) != shot.RGBAAt(150, 150) {
		t.Fatal("mosaic spilled out of its box")
	}
}
