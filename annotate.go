package main

import (
	"image"
	"math"

	"github.com/egoist/mygo/ui"
)

// tool is what dragging in the selection draws.
type tool int

const (
	toolNone tool = iota // dragging edits the selection
	toolRect
	toolEllipse
	toolArrow
	toolPen
	toolMarker
	toolMosaic
	toolText
)

// palette is the colors to draw in.
var palette = []ui.Color{
	ui.RGB(0xFF, 0x3B, 0x30), // red
	ui.RGB(0xFF, 0xCC, 0x00), // yellow
	ui.RGB(0x34, 0xC7, 0x59), // green
	ui.RGB(0x0A, 0x84, 0xFF), // blue
	ui.RGB(0x00, 0x00, 0x00),
	ui.RGB(0xFF, 0xFF, 0xFF),
}

// sizes are the small, medium and large sizes of each tool, in DIPs: the
// width of strokes, the height of text, the cells of the mosaic.
var sizes = map[tool][3]float32{
	toolRect:    {2, 4, 8},
	toolEllipse: {2, 4, 8},
	toolArrow:   {2, 4, 8},
	toolPen:     {2, 4, 8},
	toolMarker:  {12, 20, 32},
	toolMosaic:  {6, 10, 16},
	toolText:    {14, 20, 30},
}

// shape is a mark drawn over the capture, in the overlay's DIPs.
type shape struct {
	tool  tool
	color ui.Color
	size  float32
	// pts are the corners a drag went between for rectangles, ellipses,
	// arrows and mosaics, the path of the pen and the marker, and the
	// top left corner of text.
	pts  []ui.Point
	text string
}

// box is the rectangle between a shape's first and last points.
func (s *shape) box() rect {
	a, b := s.pts[0], s.pts[len(s.pts)-1]
	return span(a.X, a.Y, b.X, b.Y)
}

// visible reports whether the shape draws anything worth keeping: not a
// click that made a dot of a rectangle.
func (s *shape) visible() bool {
	switch s.tool {
	case toolText:
		return s.text != ""
	case toolPen, toolMarker:
		return len(s.pts) > 0
	case toolArrow:
		a, b := s.pts[0], s.pts[len(s.pts)-1]
		return math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)) >= 4
	}
	r := s.box()
	return r.W >= 3 && r.H >= 3
}

// textSpan is how text shapes draw.
func textSpan(text string, size float32, c ui.Color) ui.Span {
	return ui.Span{Text: text, Size: size, Weight: 600, Color: c}
}

// mosaics are the screenshot pixelated in cells of each size, made as the
// mosaic first uses one.
type mosaics struct {
	img   *image.RGBA
	scale float32 // pixels of img per DIP
	cells map[float32]*ui.Bitmap
}

// at returns the screenshot in cells of size DIPs, with the rectangle it
// covers in DIPs, a little larger than the screenshot where its last
// cells reach past it.
func (m *mosaics) at(size float32) (*ui.Bitmap, ui.Rect) {
	n := max(1, int(math.Round(float64(size*m.scale)))) // a cell's pixels
	b := m.img.Bounds()
	w, h := (b.Dx()+n-1)/n, (b.Dy()+n-1)/n
	r := ui.Rect{W: float32(w*n) / m.scale, H: float32(h*n) / m.scale}
	if bmp, ok := m.cells[size]; ok {
		return bmp, r
	}
	small := image.NewRGBA(image.Rect(0, 0, w, h))
	for cy := range h {
		for cx := range w {
			var sum [4]int
			k := 0
			for y := cy * n; y < min((cy+1)*n, b.Dy()); y++ {
				i := y*m.img.Stride + cx*n*4
				for x := cx * n; x < min((cx+1)*n, b.Dx()); x++ {
					px := m.img.Pix[i : i+4 : i+4]
					sum[0] += int(px[0])
					sum[1] += int(px[1])
					sum[2] += int(px[2])
					sum[3] += int(px[3])
					k++
					i += 4
				}
			}
			j := cy*small.Stride + cx*4
			for c := range 4 {
				small.Pix[j+c] = uint8(sum[c] / k)
			}
		}
	}
	bmp := ui.NewBitmap(small).Pixelated()
	if m.cells == nil {
		m.cells = map[float32]*ui.Bitmap{}
	}
	m.cells[size] = bmp
	return bmp, r
}

// drawShape draws s, moved by (dx, dy): in the overlay at (0, 0), and in
// the image of the selection by minus its corner.
func drawShape(p *ui.Painter, s *shape, dx, dy float32, m *mosaics) {
	pt := func(q ui.Point) (float32, float32) { return q.X + dx, q.Y + dy }
	switch s.tool {
	case toolRect:
		r := s.box()
		r.X += dx
		r.Y += dy
		var path ui.Path
		path.MoveTo(r.X, r.Y).LineTo(r.right(), r.Y).LineTo(r.right(), r.bottom()).LineTo(r.X, r.bottom()).Close()
		p.StrokePath(path.Join(ui.MiterJoin), s.size, s.color)

	case toolEllipse:
		r := s.box()
		p.StrokePath(ellipse(r.X+dx, r.Y+dy, r.W, r.H), s.size, s.color)

	case toolArrow:
		x0, y0 := pt(s.pts[0])
		x1, y1 := pt(s.pts[len(s.pts)-1])
		drawArrow(p, x0, y0, x1, y1, s.size, s.color)

	case toolPen, toolMarker:
		var path ui.Path
		x, y := pt(s.pts[0])
		path.MoveTo(x, y)
		if len(s.pts) == 1 {
			path.LineTo(x+0.01, y) // a dot
		}
		for _, q := range s.pts[1:] {
			path.LineTo(pt(q))
		}
		c := s.color
		if s.tool == toolMarker {
			c = c.Alpha(0.4)
			path.Cap(ui.ButtCap)
		}
		p.StrokePath(&path, s.size, c)

	case toolMosaic:
		r := s.box()
		bmp, full := m.at(s.size)
		cols, rows := bmp.Size()
		cw, ch := full.W/float32(cols), full.H/float32(rows) // a cell, in DIPs
		// The cells the box reaches, which ImageRegion draws, as no part of
		// the bitmap outside them.
		x0 := max(0, int(math.Floor(float64(r.X/cw))))
		y0 := max(0, int(math.Floor(float64(r.Y/ch))))
		x1 := min(cols, int(math.Ceil(float64(r.right()/cw))))
		y1 := min(rows, int(math.Ceil(float64(r.bottom()/ch))))
		if x1 <= x0 || y1 <= y0 {
			return
		}
		src := ui.Rect{X: float32(x0), Y: float32(y0), W: float32(x1 - x0), H: float32(y1 - y0)}
		dst := ui.Rect{X: float32(x0)*cw + dx, Y: float32(y0)*ch + dy, W: src.W * cw, H: src.H * ch}
		p.Clip(ui.Rect{X: r.X + dx, Y: r.Y + dy, W: r.W, H: r.H}, 0, func() {
			p.ImageRegion(bmp, src, dst, ui.ImageOptions{})
		})

	case toolText:
		x, y := pt(s.pts[0])
		p.RichText(x, y, 0, textSpan(s.text, s.size, s.color))
	}
}

// ellipse is the path of the ellipse inside the rectangle (x, y, w, h).
func ellipse(x, y, w, h float32) *ui.Path {
	const k = 0.5522848 // control points' distance for a quarter circle
	rx, ry := w/2, h/2
	cx, cy := x+rx, y+ry
	ox, oy := rx*k, ry*k
	var p ui.Path
	p.MoveTo(cx+rx, cy).
		CubeTo(cx+rx, cy+oy, cx+ox, cy+ry, cx, cy+ry).
		CubeTo(cx-ox, cy+ry, cx-rx, cy+oy, cx-rx, cy).
		CubeTo(cx-rx, cy-oy, cx-ox, cy-ry, cx, cy-ry).
		CubeTo(cx+ox, cy-ry, cx+rx, cy-oy, cx+rx, cy).
		Close()
	return &p
}

// drawArrow draws an arrow from (x0, y0) to its head at (x1, y1), with a
// line width wide.
func drawArrow(p *ui.Painter, x0, y0, x1, y1, width float32, c ui.Color) {
	dx, dy := x1-x0, y1-y0
	n := float32(math.Hypot(float64(dx), float64(dy)))
	if n < 1 {
		return
	}
	ux, uy := dx/n, dy/n
	head := min(max(10, width*4), n) // the head's length
	half := head * 0.55              // the head's half width
	bx, by := x1-ux*head, y1-uy*head // the middle of the head's base

	var shaft ui.Path
	// Into the head, which covers the line's round end.
	shaft.MoveTo(x0, y0).LineTo(bx+ux*head*0.3, by+uy*head*0.3)
	p.StrokePath(&shaft, width, c)
	var tip ui.Path
	tip.MoveTo(x1, y1).
		LineTo(bx-uy*half, by+ux*half).
		LineTo(bx+ux*head*0.15, by+uy*head*0.15).
		LineTo(bx+uy*half, by-ux*half).
		Close()
	p.FillPath(&tip, c)
}

// annotated returns the selection's image with the shapes drawn over it,
// at the screenshot's resolution, drawn by MyGo's CPU renderer as the
// overlay draws them. sel is in whole DIPs and shot is its pixels.
func annotated(shot *image.RGBA, sel rect, scale float32, shapes []*shape, m *mosaics) *image.RGBA {
	if len(shapes) == 0 {
		return shot
	}
	bmp := ui.NewBitmap(shot)
	w, h := int(math.Round(float64(sel.W))), int(math.Round(float64(sel.H)))
	out := ui.Render(func(c *ui.Context) {
		ui.Box(c).Absolute().Top(0).Left(0).Size(sel.W, sel.H).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Image(bmp, r, ui.FillBox)
			for _, s := range shapes {
				drawShape(p, s, -sel.X, -sel.Y, m)
			}
		})
	}, w, h, scale)
	return out
}
