package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWindowEdgesLeaveOutCovered(t *testing.T) {
	// A window in front covering the middle of the left edge of the one
	// behind it.
	front := rect{0, 40, 100, 20}
	back := rect{50, 0, 100, 100}
	xs, _ := windowEdges([]rect{front, back}, rect{W: 400, H: 300})
	var left []segment
	for _, s := range xs {
		if s.pos == 50 {
			left = append(left, s)
		}
	}
	want := []segment{{50, 0, 40}, {50, 60, 100}}
	if len(left) != 2 || left[0] != want[0] || left[1] != want[1] {
		t.Fatalf("left edge of the window behind = %+v, want %+v", left, want)
	}
	// Edges meeting the front window's edge are not covered by it.
	if got := visible(segment{40, 0, 100}, []rect{front}, false); len(got) != 1 {
		t.Fatalf("edge along a window's edge = %+v", got)
	}
}

// panelImage is 400 × 300 DIPs at scale 2, white with a gray panel at
// (100, 50)–(300, 200) DIPs.
func panelImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := range 600 {
		for x := range 800 {
			c := color.RGBA{255, 255, 255, 255}
			if x >= 200 && x < 600 && y >= 100 && y < 400 {
				c = color.RGBA{200, 200, 200, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestContentEdge(t *testing.T) {
	img := panelImage()
	if v, ok := contentEdge(img, 2, true, 104, 60, 190, snapReach); !ok || v != 100 {
		t.Fatalf("left edge of the panel near 104: %v %v", v, ok)
	}
	if v, ok := contentEdge(img, 2, false, 196.5, 120, 280, snapReach); !ok || v != 200 {
		t.Fatalf("bottom edge of the panel near 196.5: %v %v", v, ok)
	}
	// Across mostly white, the panel's edge is not a line.
	if _, ok := contentEdge(img, 2, true, 104, 0, 290, snapReach); ok {
		t.Fatal("snapped to an edge across a fifth of the span")
	}
	if _, ok := contentEdge(img, 2, true, 120, 60, 190, snapReach); ok {
		t.Fatal("snapped out of reach")
	}
}

func TestDragSnaps(t *testing.T) {
	img := panelImage()
	xs, ys := windowEdges([]rect{{20, 20, 50, 50}}, rect{W: 400, H: 300})
	s := snaps{x: snapTo(xs, img, 2, true), y: snapTo(ys, img, 2, false)}
	bounds := rect{W: 400, H: 300}

	// The right edge snaps to the panel, the bottom to nothing near.
	r := drag(rect{150, 100, 100, 50}, handleSE, 46, 20, bounds, s)
	if r != (rect{150, 100, 150, 70}) {
		t.Fatalf("resized to %+v", r)
	}
	// Moving, the nearer end snaps: the left edge to the window's right.
	r = drag(rect{150, 30, 30, 20}, handleMove, -77, 0, bounds, s)
	if r != (rect{70, 30, 30, 20}) {
		t.Fatalf("moved to %+v", r)
	}
	// An edge does not snap onto the opposite one.
	r = drag(rect{96, 100, 1, 50}, handleE, 2, 0, bounds, s)
	if r.W < 1 {
		t.Fatalf("collapsed to %+v", r)
	}
}

func TestOverlaySnaps(t *testing.T) {
	o, tt := testOverlay()
	xs, ys := windowEdges(o.windows, o.bounds())
	o.snap = snaps{x: snapTo(xs, o.img, o.scale, true), y: snapTo(ys, o.img, o.scale, false)}
	// Dragged out to near the window's bottom-right corner (250, 140).
	tt.Press(100, 80)
	tt.Move(200, 120)
	tt.Release(246, 143)
	if o.sel != (rect{100, 80, 150, 60}) {
		t.Fatalf("selection = %+v", o.sel)
	}
	// With Option held it does not snap.
	o.has = false
	tt.Frame()
	tt.Press(300, 200)
	tt.HoldModifiers(ui.Alt)
	tt.Move(260, 150)
	tt.Release(253, 143)
	tt.HoldModifiers(0)
	if o.sel != (rect{253, 143, 47, 57}) {
		t.Fatalf("selection with Option = %+v", o.sel)
	}
}
