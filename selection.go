package main

import (
	"image"
	"math"

	"github.com/egoist/mygo/ui"
)

// rect is an area of a window, in DIPs.
type rect struct{ X, Y, W, H float32 }

func (r rect) right() float32  { return r.X + r.W }
func (r rect) bottom() float32 { return r.Y + r.H }
func (r rect) empty() bool     { return r.W <= 0 || r.H <= 0 }
func (r rect) ui() ui.Rect     { return ui.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }

func (r rect) contains(x, y float32) bool {
	return x >= r.X && x < r.right() && y >= r.Y && y < r.bottom()
}

// span makes the rectangle between two corners, in any order.
func span(x0, y0, x1, y1 float32) rect {
	return rect{X: min(x0, x1), Y: min(y0, y1), W: abs(x1 - x0), H: abs(y1 - y0)}
}

func (r rect) intersect(o rect) rect {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.right(), o.right()), min(r.bottom(), o.bottom())
	if x1 <= x0 || y1 <= y0 {
		return rect{}
	}
	return rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// handle is the part of a selection the pointer grabs.
type handle int

const (
	handleNone handle = iota
	handleCreate
	handleMove
	handleN
	handleS
	handleE
	handleW
	handleNE
	handleNW
	handleSE
	handleSW
)

// hit tells which part of r is at (x, y): a corner or an edge within tol
// of it, the inside, or none.
func hit(r rect, x, y, tol float32) handle {
	if r.empty() {
		return handleNone
	}
	nearL, nearR := abs(x-r.X) <= tol, abs(x-r.right()) <= tol
	nearT, nearB := abs(y-r.Y) <= tol, abs(y-r.bottom()) <= tol
	inX := x >= r.X-tol && x <= r.right()+tol
	inY := y >= r.Y-tol && y <= r.bottom()+tol
	switch {
	case nearT && nearL:
		return handleNW
	case nearT && nearR:
		return handleNE
	case nearB && nearL:
		return handleSW
	case nearB && nearR:
		return handleSE
	case nearT && inX:
		return handleN
	case nearB && inX:
		return handleS
	case nearL && inY:
		return handleW
	case nearR && inY:
		return handleE
	case r.contains(x, y):
		return handleMove
	}
	return handleNone
}

// drag moves the part h of orig by (dx, dy), keeping it in bounds. Edges
// dragged past the opposite one flip the selection, as in every editor.
func drag(orig rect, h handle, dx, dy float32, bounds rect) rect {
	if h == handleMove {
		x := clamp(orig.X+dx, bounds.X, bounds.right()-orig.W)
		y := clamp(orig.Y+dy, bounds.Y, bounds.bottom()-orig.H)
		return rect{X: x, Y: y, W: orig.W, H: orig.H}
	}
	l, t, r, b := orig.X, orig.Y, orig.right(), orig.bottom()
	switch h {
	case handleN, handleNE, handleNW:
		t += dy
	case handleS, handleSE, handleSW:
		b += dy
	}
	switch h {
	case handleW, handleNW, handleSW:
		l += dx
	case handleE, handleNE, handleSE:
		r += dx
	}
	l, r = clamp(l, bounds.X, bounds.right()), clamp(r, bounds.X, bounds.right())
	t, b = clamp(t, bounds.Y, bounds.bottom()), clamp(b, bounds.Y, bounds.bottom())
	return span(l, t, r, b)
}

func (h handle) cursor() ui.Cursor {
	switch h {
	case handleN, handleS:
		return ui.CursorResizeNS
	case handleE, handleW:
		return ui.CursorResizeEW
	case handleNW, handleSE:
		return ui.CursorResizeNWSE
	case handleNE, handleSW:
		return ui.CursorResizeNESW
	case handleMove:
		return ui.CursorMove
	}
	return ui.CursorCrosshair
}

// pixels converts a selection in DIPs to the pixels of an image scale
// times larger, rounded outwards and kept inside the image.
func pixels(r rect, scale float32, size image.Point) image.Rectangle {
	x0 := int(math.Floor(float64(r.X * scale)))
	y0 := int(math.Floor(float64(r.Y * scale)))
	x1 := int(math.Ceil(float64(r.right() * scale)))
	y1 := int(math.Ceil(float64(r.bottom() * scale)))
	return image.Rect(x0, y0, x1, y1).Intersect(image.Rectangle{Max: size})
}

// toolbarAt places a toolbar of w × h by a selection in a window of
// winW × winH: under its bottom-right corner, else over its top, else
// inside it, and always in the window.
func toolbarAt(sel rect, w, h, winW, winH float32) (x, y float32) {
	const gap = 8
	x = sel.right() - w
	switch {
	case sel.bottom()+gap+h <= winH:
		y = sel.bottom() + gap
	case sel.Y-gap-h >= 0:
		y = sel.Y - gap - h
	default:
		y = sel.bottom() - gap - h
	}
	return clamp(x, 4, max(4, winW-w-4)), clamp(y, 4, max(4, winH-h-4))
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// snapped is r with its edges at the nearest whole DIPs.
func (r rect) snapped() rect {
	x0, y0 := float32(math.Round(float64(r.X))), float32(math.Round(float64(r.Y)))
	x1, y1 := float32(math.Round(float64(r.right()))), float32(math.Round(float64(r.bottom())))
	return rect{x0, y0, x1 - x0, y1 - y0}
}
