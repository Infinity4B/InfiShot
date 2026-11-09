package main

import (
	"image"
	"math"
	"slices"
)

// Edge snapping: an edge of the selection dragged near the edge of a
// window, of the display, or of something in the picture of it (a panel,
// a button, an image: a straight line where the colors change) jumps to
// it.

// snapReach is how near, in DIPs, an edge has to come to snap.
const snapReach = 6

// segment is a horizontal or vertical line: at pos on one axis, from lo
// to hi on the other.
type segment struct{ pos, lo, hi float32 }

// snapFunc moves v, an edge across [lo, hi] on the other axis, to the
// nearest edge within reach; v when there is none.
type snapFunc func(v, lo, hi float32) float32

// snaps snaps vertical edges (x) and horizontal ones (y); nil for none.
type snaps struct{ x, y snapFunc }

func (s snaps) snapX(v, lo, hi float32) float32 {
	if s.x == nil {
		return v
	}
	return s.x(v, lo, hi)
}

func (s snaps) snapY(v, lo, hi float32) float32 {
	if s.y == nil {
		return v
	}
	return s.y(v, lo, hi)
}

// windowEdges are the edges of windows, front to back, and of the screen
// behind them, where they show: the parts of an edge under a window in
// front of it are left out.
func windowEdges(windows []rect, screen rect) (xs, ys []segment) {
	all := append(slices.Clip(windows), screen)
	for i, w := range all {
		front := all[:i]
		xs = append(xs, visible(segment{w.X, w.Y, w.bottom()}, front, true)...)
		xs = append(xs, visible(segment{w.right(), w.Y, w.bottom()}, front, true)...)
		ys = append(ys, visible(segment{w.Y, w.X, w.right()}, front, false)...)
		ys = append(ys, visible(segment{w.bottom(), w.X, w.right()}, front, false)...)
	}
	return xs, ys
}

// visible is what the windows in front leave of s, vertical if vert.
func visible(s segment, front []rect, vert bool) []segment {
	parts := []segment{s}
	for _, w := range front {
		pos0, pos1, lo, hi := w.Y, w.bottom(), w.X, w.right()
		if vert {
			pos0, pos1, lo, hi = w.X, w.right(), w.Y, w.bottom()
		}
		// A window covers the line only across it, not along its edge.
		if s.pos <= pos0 || s.pos >= pos1 {
			continue
		}
		var left []segment
		for _, p := range parts {
			if p.lo < lo {
				left = append(left, segment{p.pos, p.lo, min(p.hi, lo)})
			}
			if p.hi > hi {
				left = append(left, segment{p.pos, max(p.lo, hi), p.hi})
			}
		}
		parts = left
	}
	return parts
}

// nearestSegment is the position of the segment nearest v within reach
// that overlaps [lo, hi], and whether there is one.
func nearestSegment(segs []segment, v, lo, hi, reach float32) (float32, bool) {
	best, found := float32(0), false
	for _, s := range segs {
		if s.hi <= lo || s.lo >= hi {
			continue
		}
		if d := abs(s.pos - v); d <= reach && (!found || d < abs(best-v)) {
			best, found = s.pos, true
		}
	}
	return best, found
}

// contentEdge looks in img, scale pixels a DIP, for a line where the
// colors change near v: between two columns of pixels (two rows if
// !vert) across most of [lo, hi]. It returns where in DIPs, the one
// nearest v within reach, and whether there is one.
func contentEdge(img *image.RGBA, scale float32, vert bool, v, lo, hi, reach float32) (float32, bool) {
	const (
		minSpan  = 8    // DIPs: an edge across less tells nothing
		samples  = 200  // pixels of the span looked at, at most
		strength = 0.6  // of them that have to change
		contrast = 0x30 // in the channel changing the most
	)
	if hi-lo < minSpan {
		return 0, false
	}
	b := img.Bounds()
	along, across := b.Dy(), b.Dx() // pixels along the edge, and across it
	if !vert {
		along, across = across, along
	}
	at := func(c, i int) []uint8 { // the pixel at c across, i along
		x, y := c, i
		if !vert {
			x, y = i, c
		}
		o := img.PixOffset(b.Min.X+x, b.Min.Y+y)
		return img.Pix[o : o+3 : o+3]
	}
	i0 := clampInt(int(math.Ceil(float64(lo*scale))), 0, along)
	i1 := clampInt(int(math.Floor(float64(hi*scale))), 0, along)
	if i1 <= i0 {
		return 0, false
	}
	step := max(1, (i1-i0)/samples)
	n := (i1 - i0 + step - 1) / step

	// Between pixels c-1 and c; the nearest to v that changes enough.
	mid := v * scale
	c0 := max(1, int(math.Ceil(float64((v-reach)*scale))))
	c1 := min(across-1, int(math.Floor(float64((v+reach)*scale))))
	best, found := float32(0), false
	for c := c0; c <= c1; c++ {
		if found && abs(float32(c)-mid) >= abs(best-mid) {
			continue
		}
		changed := 0
		for i := i0; i < i1; i += step {
			p, q := at(c-1, i), at(c, i)
			d := max(diff(p[0], q[0]), diff(p[1], q[1]), diff(p[2], q[2]))
			if d >= contrast {
				changed++
			}
		}
		if float32(changed) >= strength*float32(n) {
			best, found = float32(c), true
		}
	}
	return best / scale, found
}

func diff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// snapTo is a snapFunc on one axis: to the nearest of the segments and
// the edges in img, preferring the segments when they are as near.
func snapTo(segs []segment, img *image.RGBA, scale float32, vert bool) snapFunc {
	return func(v, lo, hi float32) float32 {
		lo, hi = min(lo, hi), max(lo, hi)
		best, ok := nearestSegment(segs, v, lo, hi, snapReach)
		if c, found := contentEdge(img, scale, vert, v, lo, hi, snapReach); found && (!ok || abs(c-v) < abs(best-v)) {
			best, ok = c, true
		}
		if !ok {
			return v
		}
		return best
	}
}

// snapMove moves a span of size n starting at v so that its nearer end
// snaps, keeping its size.
func snapMove(f func(v, lo, hi float32) float32, v, n, lo, hi float32) float32 {
	a, b := f(v, lo, hi)-v, f(v+n, lo, hi)-(v+n)
	switch {
	case a != 0 && (b == 0 || abs(a) <= abs(b)):
		return v + a
	case b != 0:
		return v + b
	}
	return v
}
