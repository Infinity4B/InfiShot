package main

import (
	"cmp"
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/capture"
)

// action is what to do with a selection.
type action int

const (
	actCopy action = iota
	actSave
	actPin
	actOCR
)

var (
	accent   = ui.RGB(0x1E, 0x90, 0xFF)
	dimColor = ui.RGBA(0, 0, 0, 0.45)
)

// overlay is the capture in progress: a window over a display showing the
// picture of it taken a moment before, where the user selects an area.
type overlay struct {
	a       *app
	win     *mygo.Window
	display mygo.Display
	img     *image.RGBA
	bmp     *ui.Bitmap
	scale   float32 // pixels of img per DIP
	ocrOnly bool    // recognize the selection as soon as it is made
	windows []rect  // the windows under the overlay, front to back
	snap    snaps   // where the edges of the selection snap; none if off

	// The elements found in the picture, in a while (detectElements);
	// the pointer highlights the smallest under it, and the wheel the
	// larger ones around it, level steps up.
	elements []rect
	level    int
	levelOf  rect    // the smallest under the pointer when level was set
	wheel    float32 // the scrolling not yet a step

	sel   rect // the selection, in the window's DIPs
	has   bool // whether sel is a made selection
	grab  handle
	press ui.Point // where the grab started
	orig  rect     // sel when the grab started
	moved bool     // whether the grab moved past the click slop
	ptr   ui.Point
	done  bool

	// Annotations.
	tool     tool
	color    int // in palette
	size     int // small, medium or large: in sizes
	shapes   []*shape
	drawing  *shape // the shape being drawn
	editing  *shape // the text being typed, not in shapes yet
	editText string
	mosaic   mosaics
}

func newOverlay(a *app, d mygo.Display, img *image.RGBA, wins []capture.Window, ocrOnly bool) *overlay {
	b := d.Bounds
	o := &overlay{
		a:       a,
		display: d,
		img:     img,
		bmp:     ui.NewBitmap(img).Pixelated(), // the magnifier shows pixels as squares
		scale:   float32(img.Bounds().Dx()) / float32(b.Width),
		ocrOnly: ocrOnly,
		size:    1,
	}
	o.mosaic = mosaics{img: img, scale: o.scale}
	screen := rect{W: float32(b.Width), H: float32(b.Height)}
	for _, w := range wins {
		// Application windows (layer 0) and the menu bar (24, 25); not the
		// Dock, whose window covers the screen.
		if w.Layer != 0 && w.Layer != 24 && w.Layer != 25 {
			continue
		}
		r := rect{float32(w.X - b.X), float32(w.Y - b.Y), float32(w.W), float32(w.H)}.intersect(screen)
		if !r.empty() {
			o.windows = append(o.windows, r)
		}
	}
	if a.cfg.Snap {
		o.setElements(nil)
	}
	cur := mygo.Screen.CursorScreenPoint()
	o.ptr = ui.Point{X: float32(cur.X - b.X), Y: float32(cur.Y - b.Y)}

	o.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "InfiShot",
		X:                 b.X,
		Y:                 b.Y,
		Width:             b.Width,
		Height:            b.Height,
		Frameless:         true,
		Panel:             true,
		AlwaysOnTop:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		DisableShadow:     true,
		BackgroundColor:   "#000000",
		Content:           ui.View(o.view),
	})
	// A window at (0, 0) would be centered: place it, then over the menu
	// bar, where the platform's placing does not go.
	o.win.SetBounds(b)
	coverScreen(o.win, b)
	// However it closes, the next capture must start a new overlay.
	o.win.OnClosed(func() {
		o.done = true
		if a.overlay == o {
			a.overlay = nil
		}
	})
	o.win.Focus()
	if a.cfg.Snap {
		go func() {
			els := detectElements(img, o.scale)
			o.win.Update(func() { o.setElements(els) })
		}()
	}
	return o
}

// setElements takes the elements found in the picture, and snaps the
// edges of the selection to them, to the windows and to lines in the
// picture.
func (o *overlay) setElements(els []rect) {
	o.elements = els
	xs, ys := windowEdges(o.windows, o.bounds())
	exs, eys := elementEdges(els)
	xs, ys = append(xs, exs...), append(ys, eys...)
	o.snap = snaps{x: snapTo(xs, o.img, o.scale, true), y: snapTo(ys, o.img, o.scale, false)}
}

// toolKeys are the tools in the toolbar's order, with their keys.
var toolKeys = []struct {
	tool tool
	key  ui.Key
	name string
	icon *ui.SVG
	tip  string
}{
	{toolRect, ui.KeyR, "rect", iconRect, "矩形 (R)"},
	{toolEllipse, ui.KeyE, "ellipse", iconEllipse, "椭圆 (E)"},
	{toolArrow, ui.KeyA, "arrow", iconArrow, "箭头 (A)"},
	{toolPen, ui.KeyP, "pen", iconPen, "画笔 (P)"},
	{toolMarker, ui.KeyM, "marker", iconMarker, "荧光笔 (M)"},
	{toolMosaic, ui.KeyX, "mosaic", iconMosaic, "马赛克 (X)"},
	{toolText, ui.KeyT, "text", iconText, "文字 (T)"},
}

// idle reports whether no drag or typing is in progress, when the
// toolbar and the shortcuts work.
func (o *overlay) idle() bool {
	return o.grab == handleNone && o.drawing == nil && o.editing == nil
}

func (o *overlay) view(c *ui.Context) {
	if o.editing != nil {
		// Esc ends the text, not the capture; Enter ends it in the input.
		c.OnShortcut(0, ui.KeyEscape, o.commitText)
	} else {
		c.OnShortcut(0, ui.KeyEscape, o.cancel)
	}
	// The keys check the state as they come rather than as the view last
	// built: a key pressed right after the mouse let go of a stroke comes
	// before the next frame.
	when := func(ok func() bool, fn func()) func() {
		return func() {
			if ok() {
				fn()
			}
		}
	}
	if o.editing == nil && !o.ocrOnly {
		// Without a selection they take what is highlighted: the window
		// under the pointer, or the display.
		for _, s := range []struct {
			mods ui.Modifiers
			key  ui.Key
			act  action
		}{
			{0, ui.KeyEnter, actCopy}, {ui.Cmd, ui.KeyC, actCopy}, {ui.Cmd, ui.KeyS, actSave},
			{ui.Cmd, ui.KeyT, actPin}, {ui.Cmd, ui.KeyO, actOCR},
		} {
			c.OnShortcut(s.mods, s.key, when(o.idle, func() { o.shortcut(s.act) }))
		}
		marking := func() bool { return o.has && o.idle() }
		for _, t := range toolKeys {
			c.OnShortcut(0, t.key, when(marking, func() { o.pick(t.tool) }))
		}
		c.OnShortcut(ui.Cmd, ui.KeyZ, when(o.idle, o.undo))

		// Arrows move the selection a point; with Shift they grow it.
		editing := func() bool { return marking() && o.tool == toolNone }
		for _, k := range []struct {
			key    ui.Key
			dx, dy float32
		}{{ui.KeyLeft, -1, 0}, {ui.KeyRight, 1, 0}, {ui.KeyUp, 0, -1}, {ui.KeyDown, 0, 1}} {
			c.OnShortcut(0, k.key, when(editing, func() { o.nudge(k.dx, k.dy, false) }))
			c.OnShortcut(ui.Shift, k.key, when(editing, func() { o.nudge(k.dx, k.dy, true) }))
		}
	}

	w, h := c.Size()
	ui.Box(c).Absolute().Top(0).Left(0).Size(w, h).
		Cursor(o.cursor()).
		Draw(o.paint).
		HandleInput(o.input)
	if o.editing != nil {
		o.textInput(c)
	}
	if o.has && o.grab == handleNone && o.drawing == nil && !o.ocrOnly {
		o.toolbar(c, w, h)
	}
}

// pick chooses a tool to draw with, or lets go of the one chosen.
func (o *overlay) pick(t tool) {
	o.commitText()
	if o.tool == t {
		o.tool = toolNone
	} else {
		o.tool = t
	}
}

func (o *overlay) undo() {
	if o.editing != nil {
		o.editing = nil
		return
	}
	if n := len(o.shapes); n > 0 {
		o.shapes = o.shapes[:n-1]
	}
}

// commitText ends the text being typed, keeping it unless it is empty.
func (o *overlay) commitText() {
	if o.editing == nil {
		return
	}
	o.editing.text = strings.TrimRight(o.editText, " ")
	if o.editing.visible() {
		o.shapes = append(o.shapes, o.editing)
	}
	o.editing, o.editText = nil, ""
}

// textInput is where the text being typed is, drawn as it will be.
func (o *overlay) textInput(c *ui.Context) {
	s := o.editing
	x, y := s.pts[0].X, s.pts[0].Y
	in := ui.TextInputBase(c.Key("text-input"), &o.editText).Absolute().Left(x).Top(y).
		Width(max(80, o.sel.right()-x-4)).
		FontSize(s.size).FontWeight(600).TextColor(s.color).CaretColor(s.color).
		Label("标注文字").AutoFocus()
	if in.Submitted() {
		o.commitText()
	}
}

func (o *overlay) bounds() rect {
	return rect{W: float32(o.display.Bounds.Width), H: float32(o.display.Bounds.Height)}
}

// current is what the overlay outlines: the selection being made or made,
// else the window under the pointer, else the whole display.
func (o *overlay) current() (rect, bool) {
	switch {
	case o.grab == handleCreate && o.moved:
		return o.sel, true
	case o.has:
		return o.sel, true
	case o.grab == handleNone || o.grab == handleCreate:
		return o.candidate(), true
	}
	return rect{}, false
}

func (o *overlay) candidate() rect {
	ch := o.chain()
	return ch[clampInt(o.level, 0, len(ch)-1)]
}

// elementPad is the room, in DIPs, left around an element highlighted,
// for it not to touch the edges of the picture taken of it.
const elementPad = 6

// chain is what the pointer can highlight, smallest first: the elements
// under it in the window under it with room around them, that window,
// and the display.
func (o *overlay) chain() []rect {
	screen := o.bounds()
	win := screen
	for _, w := range o.windows {
		if w.contains(o.ptr.X, o.ptr.Y) {
			win = w
			break
		}
	}
	var ch []rect
	for _, e := range o.elements {
		if !e.contains(o.ptr.X, o.ptr.Y) || !e.inside(win, 1) {
			continue
		}
		p := e.pad(elementPad).intersect(win)
		if !p.near(win, 3) {
			ch = append(ch, p)
		}
	}
	slices.SortStableFunc(ch, func(a, b rect) int { return cmp.Compare(a.W*a.H, b.W*b.H) })
	// Of boxes about the same, the smallest.
	kept := ch[:0]
	for _, e := range ch {
		if len(kept) == 0 || !e.near(kept[len(kept)-1], 2) {
			kept = append(kept, e)
		}
	}
	ch = append(kept, win)
	if win != screen {
		ch = append(ch, screen)
	}
	return ch
}

// aim follows the pointer with the highlight: from the smallest element
// under it again when that changes.
func (o *overlay) aim() {
	if o.has || o.grab != handleNone {
		return
	}
	if first := o.chain()[0]; first != o.levelOf {
		o.level, o.levelOf, o.wheel = 0, first, 0
	}
}

// scroll steps the highlight to a larger element around the pointer, or
// a smaller one, a step a notch of a wheel, or 24 DIPs of a touchpad.
func (o *overlay) scroll(ev ui.InputEvent) {
	if o.has || o.grab != handleNone || ev.DY == 0 {
		return
	}
	step := 0
	if ev.Precise {
		o.wheel += ev.DY
		for abs(o.wheel) >= 24 {
			step += int(math.Copysign(1, float64(o.wheel)))
			o.wheel -= float32(math.Copysign(24, float64(o.wheel)))
		}
	} else {
		step = int(math.Copysign(1, float64(ev.DY)))
	}
	// Scrolling up, away, takes in more.
	n := len(o.chain())
	o.level = clampInt(clampInt(o.level, 0, n-1)-step, 0, n-1)
}

func (o *overlay) cursor() ui.Cursor {
	if o.grab != handleNone {
		return o.grab.cursor()
	}
	if o.drawing != nil {
		return ui.CursorCrosshair
	}
	if o.tool != toolNone && o.has {
		switch {
		case !o.sel.contains(o.ptr.X, o.ptr.Y):
			return ui.CursorDefault
		case o.tool == toolText:
			return ui.CursorText
		}
		return ui.CursorCrosshair
	}
	if o.has {
		return hit(o.sel, o.ptr.X, o.ptr.Y, 6).cursor()
	}
	return ui.CursorCrosshair
}

func (o *overlay) input(ev ui.InputEvent) bool {
	pt := ui.Point{X: ev.X, Y: ev.Y}
	switch ev.Kind {
	case ui.InputScroll:
		o.ptr = pt
		o.aim()
		o.scroll(ev)
		return true

	case ui.InputPointerMove:
		o.ptr = pt
		o.aim()
		switch {
		case o.drawing != nil && ev.Button == 0:
			o.extend(pt, ev.Mods&ui.Shift != 0)
		case o.grab != handleNone && ev.Button == 0:
			o.dragTo(pt, ev.Mods)
		}
		return true

	case ui.InputPointerDown:
		o.ptr = pt
		switch ev.Button {
		case 0:
			typing := o.editing != nil
			o.commitText()
			if o.tool != toolNone && o.has {
				if o.sel.contains(pt.X, pt.Y) && !(typing && o.tool == toolText) {
					o.startShape(pt)
				}
				return true
			}
			if ev.Clicks >= 2 && o.has && o.sel.contains(pt.X, pt.Y) && !o.ocrOnly {
				o.finish(actCopy)
				return true
			}
			h := handleNone
			if o.has {
				h = hit(o.sel, pt.X, pt.Y, 6)
				if h == handleNone && len(o.shapes) > 0 {
					return true // the marks are made on this selection: keep it
				}
			}
			if h == handleNone {
				h = handleCreate
				o.has = false
			}
			o.grab, o.press, o.orig, o.moved = h, pt, o.sel, false
		case 1:
			// Right-click lets go of the text, of the tool, of the
			// selection unless marks were made on it, then of the capture.
			switch {
			case o.editing != nil:
				o.commitText()
			case o.tool != toolNone:
				o.tool = toolNone
			case o.has && len(o.shapes) > 0:
			case o.has:
				o.has = false
			default:
				o.cancel()
			}
		}
		return true

	case ui.InputPointerUp:
		if ev.Button != 0 {
			return true
		}
		if s := o.drawing; s != nil {
			if !ev.Canceled {
				o.extend(pt, ev.Mods&ui.Shift != 0)
				if s.visible() {
					o.shapes = append(o.shapes, s)
				}
			}
			o.drawing = nil
			return true
		}
		if o.grab == handleNone {
			return true
		}
		if ev.Canceled {
			// The system took the press: undo what it did.
			if o.grab == handleCreate {
				o.has = false
			} else {
				o.sel, o.has = o.orig, true
			}
			o.grab = handleNone
			return true
		}
		o.dragTo(pt, ev.Mods)
		if o.grab == handleCreate {
			if !o.moved {
				// A click takes the window under it, or the display.
				o.sel = o.candidate()
			}
			o.has = o.sel.W >= 2 && o.sel.H >= 2
		}
		o.grab = handleNone
		if o.has && o.ocrOnly {
			o.finish(actOCR)
		}
		return true
	}
	return false
}

// startShape starts drawing with the tool at pt, in the selection.
func (o *overlay) startShape(pt ui.Point) {
	s := &shape{tool: o.tool, color: palette[o.color], size: sizes[o.tool][o.size]}
	switch o.tool {
	case toolText:
		// The click is at the middle of the first line.
		s.pts = []ui.Point{{X: pt.X, Y: pt.Y - s.size*0.7}}
		o.editing, o.editText = s, ""
		return
	case toolPen, toolMarker:
		s.pts = []ui.Point{pt}
	default:
		s.pts = []ui.Point{pt, pt}
	}
	o.drawing = s
}

// extend takes the shape being drawn to pt, kept in the selection. With
// square, rectangles and ellipses are squares and circles, and arrows
// point at multiples of 45°.
func (o *overlay) extend(pt ui.Point, square bool) {
	s := o.drawing
	pt.X = clamp(pt.X, o.sel.X, o.sel.right())
	pt.Y = clamp(pt.Y, o.sel.Y, o.sel.bottom())
	switch s.tool {
	case toolPen, toolMarker:
		last := s.pts[len(s.pts)-1]
		if abs(pt.X-last.X)+abs(pt.Y-last.Y) >= 1 {
			s.pts = append(s.pts, pt)
		}
		return
	}
	a := s.pts[0]
	if square {
		dx, dy := pt.X-a.X, pt.Y-a.Y
		if s.tool == toolArrow {
			n := math.Hypot(float64(dx), float64(dy))
			th := math.Round(math.Atan2(float64(dy), float64(dx))/(math.Pi/4)) * (math.Pi / 4)
			pt = ui.Point{X: a.X + float32(n*math.Cos(th)), Y: a.Y + float32(n*math.Sin(th))}
		} else {
			d := max(abs(dx), abs(dy))
			pt = ui.Point{X: a.X + copySign(d, dx), Y: a.Y + copySign(d, dy)}
		}
	}
	s.pts[1] = pt
}

func copySign(v, sign float32) float32 { return float32(math.Copysign(float64(v), float64(sign))) }

// dragTo takes the grab to pt. The edges it moves snap, unless Option
// is held.
func (o *overlay) dragTo(pt ui.Point, mods ui.Modifiers) {
	dx, dy := pt.X-o.press.X, pt.Y-o.press.Y
	if !o.moved && abs(dx) < 3 && abs(dy) < 3 {
		return
	}
	o.moved = true
	s := o.snap
	if mods&ui.Alt != 0 {
		s = snaps{}
	}
	if o.grab == handleCreate {
		// As dragging the corner of a selection with no size at the press.
		b := o.bounds()
		o.sel = drag(rect{X: o.press.X, Y: o.press.Y}, handleSE, clamp(pt.X, 0, b.W)-o.press.X, clamp(pt.Y, 0, b.H)-o.press.Y, b, s)
		return
	}
	o.sel = drag(o.orig, o.grab, dx, dy, o.bounds(), s)
}

func (o *overlay) nudge(dx, dy float32, grow bool) {
	h := handleMove
	if grow {
		h = handleSE
	}
	if r := drag(o.sel, h, dx, dy, o.bounds(), snaps{}); r.W >= 2 && r.H >= 2 {
		o.sel = r
	}
}

func (o *overlay) paint(p *ui.Painter, r ui.Rect) {
	p.Image(o.bmp, r, ui.FillBox)
	sel, ok := o.current()
	if !ok || sel.empty() {
		p.Fill(r, dimColor, 0)
	} else {
		p.Fill(ui.Rect{W: r.W, H: sel.Y}, dimColor, 0)
		p.Fill(ui.Rect{Y: sel.bottom(), W: r.W, H: r.H - sel.bottom()}, dimColor, 0)
		p.Fill(ui.Rect{Y: sel.Y, W: sel.X, H: sel.H}, dimColor, 0)
		p.Fill(ui.Rect{X: sel.right(), Y: sel.Y, W: r.W - sel.right(), H: sel.H}, dimColor, 0)
		if o.has || o.grab >= handleMove { // not over a new selection
			p.Clip(sel.ui(), 0, func() {
				for _, s := range o.shapes {
					drawShape(p, s, 0, 0, &o.mosaic)
				}
				if o.drawing != nil {
					drawShape(p, o.drawing, 0, 0, &o.mosaic)
				}
			})
			if s := o.editing; s != nil {
				box := ui.Rect{X: s.pts[0].X - 4, Y: s.pts[0].Y - 2, W: max(80, o.sel.right()-s.pts[0].X-4) + 8, H: s.size*1.4 + 4}
				p.StrokeDashed(box, ui.RGBA(255, 255, 255, 0.8), 2, 1)
			}
		}
		p.Stroke(sel.ui(), accent, 0, 2)
		if o.has || o.grab != handleNone && o.grab != handleCreate {
			o.paintHandles(p, sel)
		}
		o.paintSize(p, sel)
	}
	if o.magnifying() {
		o.paintMagnifier(p, r)
	}
}

func (o *overlay) paintHandles(p *ui.Painter, s rect) {
	const n = 7
	mx, my := s.X+s.W/2, s.Y+s.H/2
	for _, pt := range [][2]float32{
		{s.X, s.Y}, {mx, s.Y}, {s.right(), s.Y},
		{s.X, my}, {s.right(), my},
		{s.X, s.bottom()}, {mx, s.bottom()}, {s.right(), s.bottom()},
	} {
		h := ui.Rect{X: pt[0] - n/2, Y: pt[1] - n/2, W: n, H: n}
		p.Fill(h, ui.RGB(255, 255, 255), n/2)
		p.Stroke(h, accent, n/2, 1.5)
	}
}

// paintSize labels the selection with its size in pixels, above it, or
// inside it when it reaches the top of the display.
func (o *overlay) paintSize(p *ui.Painter, s rect) {
	px := pixels(s, o.scale, o.img.Bounds().Size())
	label := fmt.Sprintf("%d × %d", px.Dx(), px.Dy())
	lbl := ui.Span{Text: label, Size: 12, Weight: 500, Color: ui.RGB(255, 255, 255)}
	w, h := p.MeasureText(0, lbl)
	box := ui.Rect{X: s.X, Y: s.Y - h - 10, W: w + 12, H: h + 6}
	if box.Y < 2 {
		box.X, box.Y = s.X+4, s.Y+4
	}
	p.Fill(box, ui.RGBA(0, 0, 0, 0.7), 4)
	p.RichText(box.X+6, box.Y+3, 0, lbl)
}

// magnifying is whether to show the magnifier: while aiming, making the
// selection or moving an edge, not while moving the selection or using
// the toolbar.
func (o *overlay) magnifying() bool {
	switch o.grab {
	case handleMove:
		return false
	case handleNone:
		return !o.has
	}
	return true
}

func (o *overlay) paintMagnifier(p *ui.Painter, r ui.Rect) {
	const (
		cells = 15 // pixels across, odd so one is in the middle
		zoom  = 8
		size  = cells * zoom
		info  = 40
		gap   = 20
	)
	sz := o.img.Bounds().Size()
	px := clampInt(int(o.ptr.X*o.scale), 0, sz.X-1)
	py := clampInt(int(o.ptr.Y*o.scale), 0, sz.Y-1)

	x, y := o.ptr.X+gap, o.ptr.Y+gap
	if x+size > r.W {
		x = o.ptr.X - gap - size
	}
	if y+size+info > r.H {
		y = o.ptr.Y - gap - size - info
	}
	frame := ui.Rect{X: x - 2, Y: y - 2, W: size + 4, H: size + info + 4}
	p.Fill(frame, ui.RGBA(20, 20, 20, 0.92), 6)
	view := ui.Rect{X: x, Y: y, W: size, H: size}
	p.Clip(view, 4, func() {
		// The pixels around the pointer, cut to the image at its edges,
		// where the rest of the view stays dark: MyGo's CPU renderer
		// does not take a source reaching outside the bitmap.
		sx, sy := px-cells/2, py-cells/2
		x0, y0 := max(sx, 0), max(sy, 0)
		x1, y1 := min(sx+cells, sz.X), min(sy+cells, sz.Y)
		if x1 > x0 && y1 > y0 {
			src := ui.Rect{X: float32(x0), Y: float32(y0), W: float32(x1 - x0), H: float32(y1 - y0)}
			dst := ui.Rect{
				X: x + float32(x0-sx)*zoom, Y: y + float32(y0-sy)*zoom,
				W: src.W * zoom, H: src.H * zoom,
			}
			p.ImageRegion(o.bmp, src, dst, ui.ImageOptions{})
		}
		mid := float32(cells/2) * zoom
		guide := ui.RGBA(0x1E, 0x90, 0xFF, 0.35)
		p.Fill(ui.Rect{X: x, Y: y + mid, W: size, H: zoom}, guide, 0)
		p.Fill(ui.Rect{X: x + mid, Y: y, W: zoom, H: size}, guide, 0)
		p.Stroke(ui.Rect{X: x + mid, Y: y + mid, W: zoom, H: zoom}, ui.RGB(255, 255, 255), 0, 1)
	})
	c := o.img.RGBAAt(px, py)
	white, muted := ui.RGB(255, 255, 255), ui.RGB(180, 180, 180)
	p.Text(x+4, y+size+5, fmt.Sprintf("(%d, %d)", px, py), 11, muted)
	p.Fill(ui.Rect{X: x + 4, Y: y + size + 23, W: 10, H: 10}, ui.RGB(c.R, c.G, c.B), 2)
	p.Text(x+20, y+size+21, fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B), 11, white)
}

// The toolbar's metrics.
const (
	tbButton = 32 // a button of the main row
	tbSwatch = 28 // a button of the row of colors and sizes
	tbPad    = 6
	tbGap    = 4
	tbRowGap = 6 // between the rows
)

// rowWidth is the width of a toolbar row of n buttons of size b and seps
// separators.
func rowWidth(n, seps int, b float32) float32 {
	return 2*tbPad + float32(n)*b + float32(seps) + float32(n+seps-1)*tbGap
}

func (o *overlay) toolbar(c *ui.Context, winW, winH float32) {
	t := c.Theme()
	actions := []struct {
		key  string
		icon *ui.SVG
		tip  string
		act  action
	}{
		{"ocr", iconOCR, "识别文字 (⌘O)", actOCR},
		{"pin", iconPin, "贴到屏幕上 (⌘T)", actPin},
		{"save", iconSave, "保存 (⌘S)", actSave},
	}
	mainW := rowWidth(len(toolKeys)+1+len(actions)+2, 3, tbButton)
	mainH := float32(tbButton + 2*tbPad)
	subW := rowWidth(len(sizes[toolRect])+len(palette), 1, tbSwatch)
	subH := float32(tbSwatch + 2*tbPad)
	w, h := mainW, mainH
	if o.tool != toolNone {
		w, h = max(mainW, subW), mainH+tbRowGap+subH
	}
	x, y := toolbarAt(o.sel, w, h, winW, winH)

	bar := func(key string, x, y, w, h float32, children func()) {
		ui.Row(c.Key(key)).Absolute().Left(x).Top(y).Size(w, h).Padding(tbPad).Gap(tbGap).
			AlignItems(ui.Center).Radius(10).Background(t.Background).
			Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.3)).
			Cursor(ui.CursorDefault).
			Children(children)
	}
	sep := func() { ui.Box(c).Size(1, 20).Background(t.Border) }

	bar("toolbar", x, y, mainW, mainH, func() {
		for _, tk := range toolKeys {
			if o.iconButton(c, tk.name, tk.icon, tk.tip, o.tool == tk.tool, false) {
				o.pick(tk.tool)
			}
		}
		sep()
		if o.iconButton(c, "undo", iconUndo, "撤销 (⌘Z)", false, len(o.shapes) == 0) {
			o.undo()
		}
		sep()
		for _, it := range actions {
			if o.iconButton(c, it.key, it.icon, it.tip, false, false) {
				o.finish(it.act)
			}
		}
		sep()
		if o.iconButton(c, "cancel", iconClose, "取消 (Esc)", false, false) {
			o.cancel()
		}
		if o.iconButton(c, "copy", iconDone, "复制到剪贴板 (↩ 或 ⌘C)", true, false) {
			o.finish(actCopy)
		}
	})
	if o.tool == toolNone {
		return
	}
	bar("styles", x, y+mainH+tbRowGap, subW, subH, func() {
		for i, d := range []float32{4, 7, 11} {
			b := ui.ButtonBase(c.Key(fmt.Sprint("size", i))).Size(tbSwatch, tbSwatch).Radius(6).
				Tooltip([]string{"细", "中", "粗"}[i]).Label([]string{"细", "中", "粗"}[i])
			if o.size == i {
				b.Background(t.SurfacePressed)
			} else if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() { ui.Box(c).Size(d, d).Radius(d / 2).Background(t.Text) })
			if b.Clicked() {
				o.size = i
			}
		}
		sep()
		for i, col := range palette {
			b := ui.ButtonBase(c.Key(fmt.Sprint("color", i))).Size(tbSwatch, tbSwatch).Radius(tbSwatch / 2).
				Label(fmt.Sprintf("颜色 %d", i+1))
			if o.color == i {
				b.Border(2, accent)
			}
			b.Children(func() {
				ui.Box(c).Size(18, 18).Radius(9).Background(col).Border(1, ui.RGBA(128, 128, 128, 0.5))
			})
			if b.Clicked() {
				o.color = i
			}
		}
	})
}

// iconButton is a button of the toolbar showing an icon; on is the chosen
// tool, or the main action.
func (o *overlay) iconButton(c *ui.Context, key string, icon *ui.SVG, tip string, on, disabled bool) bool {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Size(tbButton, tbButton).Radius(7).Tooltip(tip).Label(tip).Disabled(disabled)
	fg := t.Text
	switch {
	case on:
		b.Background(accent)
		fg = ui.RGB(255, 255, 255)
	case disabled:
		fg = t.TextMuted.Alpha(0.5)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(18, 18).TextColor(fg) })
	return b.Clicked() && !disabled
}

// shortcut finishes with the selection, or without one with the highlight.
func (o *overlay) shortcut(act action) {
	if !o.has {
		o.sel, o.has = o.candidate(), true
	}
	o.finish(act)
}

// finish ends the capture with the selection and the marks on it.
func (o *overlay) finish(act action) {
	o.commitText()
	// In whole DIPs, so that the image is whole pixels of the screenshot
	// a DIP and shows in a window of the selection's size unscaled.
	sel := o.sel.snapped().intersect(o.bounds())
	if o.done || sel.empty() {
		return
	}
	o.done = true
	shot := capture.Crop(o.img, pixels(sel, o.scale, o.img.Bounds().Size()))
	shot = annotated(shot, sel, o.scale, o.shapes, &o.mosaic)
	b := o.display.Bounds
	at := mygo.Rectangle{
		X:      b.X + int(sel.X),
		Y:      b.Y + int(sel.Y),
		Width:  max(1, int(sel.W)),
		Height: max(1, int(sel.H)),
	}
	o.close()
	o.a.use(act, shot, at)
}

func (o *overlay) cancel() {
	if o.done {
		return
	}
	o.done = true
	o.close()
}

func (o *overlay) close() {
	o.win.Close()
	if o.a.overlay == o {
		o.a.overlay = nil
	}
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }
