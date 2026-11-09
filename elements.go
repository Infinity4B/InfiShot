package main

import (
	"image"
	"math"
)

// Content detection: the things in a screenshot one may want to take,
// found in its pixels. They are of two kinds:
//
//   - areas of one color, bounded by where the colors change: panels,
//     fields, buttons, cards;
//   - clusters of detail, where the colors change a lot close together:
//     icons, words, lines of text, images.
//
// The screenshot is looked at in cells of one DIP: a cell is an edge
// when the colors change in it or at its border.

const (
	edgeContrast = 0x06 // in the channel changing the most
	minElement   = 6    // DIPs: the shorter side of an element, at least
	// The shorter side of an area of one color, at least: less are the
	// insides of letters.
	minArea = 10
)

// clusterGaps are how far apart, in DIPs, detail can be and still be one
// cluster, for each size of cluster: the letters of a word, the words of
// a line or a group.
var clusterGaps = []int{1, 4}

// detectElements finds the elements in img, scale pixels a DIP, in DIPs.
func detectElements(img *image.RGBA, scale float32) []rect {
	w, h, edge := edgeCells(img, scale)
	var els []rect
	add := func(x0, y0, x1, y1 int) {
		if x1-x0+1 >= minElement && y1-y0+1 >= minElement {
			els = append(els, rect{float32(x0), float32(y0), float32(x1 - x0 + 1), float32(y1 - y0 + 1)})
		}
	}
	addArea := func(x0, y0, x1, y1 int) {
		if x1-x0+1 >= minArea && y1-y0+1 >= minArea {
			els = append(els, rect{float32(x0), float32(y0), float32(x1 - x0 + 1), float32(y1 - y0 + 1)})
		}
	}
	// Areas of one color take in the edge cells around them, where the
	// colors change: the area's border is in them.
	components(w, h, edge, false, nil, false, func(x0, y0, x1, y1 int) {
		addArea(max(0, x0-1), max(0, y0-1), min(w-1, x1+1), min(h-1, y1+1))
	})
	for _, gap := range clusterGaps {
		joined := dilate(edge, w, h, gap)
		components(w, h, joined, true, edge, true, add)
	}
	return els
}

// edgeCells marks the cells, w × h DIPs, where the colors of img change:
// both cells at a change between pixels in two of them.
func edgeCells(img *image.RGBA, scale float32) (w, h int, edge []bool) {
	b := img.Bounds()
	pw, ph := b.Dx(), b.Dy()
	w = max(1, int(math.Ceil(float64(float32(pw)/scale-0.01))))
	h = max(1, int(math.Ceil(float64(float32(ph)/scale-0.01))))
	edge = make([]bool, w*h)
	cellX := make([]int, pw)
	for x := range cellX {
		cellX[x] = min(w-1, int(float32(x)/scale))
	}
	cellY := make([]int, ph)
	for y := range cellY {
		cellY[y] = min(h-1, int(float32(y)/scale))
	}
	changed := func(p, q []uint8) bool {
		return diff(p[0], q[0]) >= edgeContrast || diff(p[1], q[1]) >= edgeContrast || diff(p[2], q[2]) >= edgeContrast
	}
	for y := range ph {
		row := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):]
		var next []uint8
		if y+1 < ph {
			next = img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y+1):]
		}
		at := cellY[y] * w
		for x := range pw {
			o := 4 * x
			p := row[o : o+3 : o+3]
			if x+1 < pw && changed(p, row[o+4:o+7]) {
				edge[at+cellX[x]] = true
				edge[at+cellX[x+1]] = true
			}
			if next != nil && changed(p, next[o:o+3]) {
				edge[at+cellX[x]] = true
				edge[cellY[y+1]*w+cellX[x]] = true
			}
		}
	}
	return w, h, edge
}

// components calls fn with the bounding box, in cells, of each group of
// connected cells c with in[c] == want: next to each other, or diagonally
// too with diag. With of, the box is of the cells of the group for which
// of is true, and groups with none are left out.
func components(w, h int, in []bool, want bool, of []bool, diag bool, fn func(x0, y0, x1, y1 int)) {
	seen := make([]bool, len(in))
	var stack []int32
	for start := range in {
		if seen[start] || in[start] != want {
			continue
		}
		seen[start] = true
		stack = append(stack[:0], int32(start))
		x0, y0, x1, y1 := w, h, -1, -1
		for len(stack) > 0 {
			i := int(stack[len(stack)-1])
			stack = stack[:len(stack)-1]
			x, y := i%w, i/w
			if of == nil || of[i] {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 || !diag && dx != 0 && dy != 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if j := ny*w + nx; !seen[j] && in[j] == want {
						seen[j] = true
						stack = append(stack, int32(j))
					}
				}
			}
		}
		if x1 >= 0 {
			fn(x0, y0, x1, y1)
		}
	}
}

// dilate marks the cells within r of a marked cell, across and along:
// detail up to 2r apart joins.
func dilate(cells []bool, w, h, r int) []bool {
	across := make([]bool, len(cells))
	for y := range h {
		row := cells[y*w : (y+1)*w]
		n := 0 // marked cells in the window around x
		for x := 0; x < min(r, w); x++ {
			if row[x] {
				n++
			}
		}
		for x := range w {
			if x+r < w && row[x+r] {
				n++
			}
			if x-r-1 >= 0 && row[x-r-1] {
				n--
			}
			across[y*w+x] = n > 0
		}
	}
	out := make([]bool, len(cells))
	for x := range w {
		n := 0
		for y := 0; y < min(r, h); y++ {
			if across[y*w+x] {
				n++
			}
		}
		for y := range h {
			if y+r < h && across[(y+r)*w+x] {
				n++
			}
			if y-r-1 >= 0 && across[(y-r-1)*w+x] {
				n--
			}
			out[y*w+x] = n > 0
		}
	}
	return out
}

// near reports whether every side of r is within d of o's.
func (r rect) near(o rect, d float32) bool {
	return abs(r.X-o.X) <= d && abs(r.Y-o.Y) <= d && abs(r.right()-o.right()) <= d && abs(r.bottom()-o.bottom()) <= d
}

// pad is r grown by d on every side.
func (r rect) pad(d float32) rect { return rect{r.X - d, r.Y - d, r.W + 2*d, r.H + 2*d} }

func (r rect) inside(o rect, tol float32) bool {
	return r.X >= o.X-tol && r.Y >= o.Y-tol && r.right() <= o.right()+tol && r.bottom() <= o.bottom()+tol
}

// elementEdges are the sides of the elements, to snap to.
func elementEdges(els []rect) (xs, ys []segment) {
	for _, e := range els {
		xs = append(xs, segment{e.X, e.Y, e.bottom()}, segment{e.right(), e.Y, e.bottom()})
		ys = append(ys, segment{e.Y, e.X, e.right()}, segment{e.bottom(), e.X, e.right()})
	}
	return xs, ys
}
