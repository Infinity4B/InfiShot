package main

import (
	"image"
	"testing"
)

func TestSpan(t *testing.T) {
	if got := span(10, 20, 4, 5); got != (rect{4, 5, 6, 15}) {
		t.Fatalf("span = %+v", got)
	}
}

func TestHit(t *testing.T) {
	r := rect{100, 100, 200, 100}
	for _, tc := range []struct {
		x, y float32
		want handle
	}{
		{100, 100, handleNW},
		{300, 200, handleSE},
		{302, 98, handleNE},
		{98, 202, handleSW},
		{200, 100, handleN},
		{200, 199, handleS},
		{100, 150, handleW},
		{301, 150, handleE},
		{200, 150, handleMove},
		{50, 50, handleNone},
	} {
		if got := hit(r, tc.x, tc.y, 6); got != tc.want {
			t.Errorf("hit(%v, %v) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	if hit(rect{}, 0, 0, 6) != handleNone {
		t.Error("an empty selection was hit")
	}
}

func TestDrag(t *testing.T) {
	bounds := rect{0, 0, 1000, 800}
	r := rect{100, 100, 200, 100}

	if got := drag(r, handleMove, 50, -20, bounds, snaps{}); got != (rect{150, 80, 200, 100}) {
		t.Errorf("move = %+v", got)
	}
	if got := drag(r, handleMove, -500, 0, bounds, snaps{}); got.X != 0 || got.W != 200 {
		t.Errorf("move past the edge = %+v", got)
	}
	if got := drag(r, handleSE, 10, 20, bounds, snaps{}); got != (rect{100, 100, 210, 120}) {
		t.Errorf("resize SE = %+v", got)
	}
	// Dragging the left edge past the right one flips the selection.
	if got := drag(r, handleW, 250, 0, bounds, snaps{}); got != (rect{300, 100, 50, 100}) {
		t.Errorf("flip = %+v", got)
	}
	if got := drag(r, handleN, 0, -500, bounds, snaps{}); got.Y != 0 || got.bottom() != 200 {
		t.Errorf("resize past the edge = %+v", got)
	}
}

func TestPixels(t *testing.T) {
	got := pixels(rect{10.4, 20, 100.3, 50}, 2, image.Pt(400, 200))
	if want := image.Rect(20, 40, 222, 140); got != want {
		t.Errorf("pixels = %v, want %v", got, want)
	}
	if got := pixels(rect{150, 50, 100, 100}, 2, image.Pt(400, 200)); got.Max != image.Pt(400, 200) {
		t.Errorf("pixels not clamped: %v", got)
	}
}

func TestToolbarAt(t *testing.T) {
	// Below the selection, right-aligned.
	if x, y := toolbarAt(rect{100, 100, 300, 200}, 200, 40, 1000, 800); x != 200 || y != 308 {
		t.Errorf("below = %v, %v", x, y)
	}
	// No room below: above.
	if _, y := toolbarAt(rect{100, 500, 300, 280}, 200, 40, 1000, 800); y != 452 {
		t.Errorf("above = %v", y)
	}
	// The whole screen: inside.
	if _, y := toolbarAt(rect{0, 0, 1000, 800}, 200, 40, 1000, 800); y != 752 {
		t.Errorf("inside = %v", y)
	}
	// Kept in the window on the left.
	if x, _ := toolbarAt(rect{0, 100, 50, 50}, 200, 40, 1000, 800); x != 4 {
		t.Errorf("left = %v", x)
	}
}
