package main

import (
	"math"
	"strings"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/ocr"
)

// textLayer is the text recognized in a pin, to select as in a document:
// its lines in reading order, each rune with its box. Boxes are in
// fractions of the image, so they hold at any zoom.
type textLayer struct {
	lines []ocr.Line
	row   []int // the row of each line, for joining
	runes [][]rune
	boxes [][]ocr.Rect
}

// caret is a place between runes: before rune at of line.
type caret struct{ line, at int }

func (c caret) before(d caret) bool {
	return c.line < d.line || c.line == d.line && c.at < d.at
}

func newTextLayer(res ocr.Result) *textLayer {
	t := &textLayer{}
	for i, row := range ocr.Rows(res.Lines) {
		for _, l := range row {
			r := []rune(l.Text)
			if len(r) == 0 {
				continue
			}
			t.lines = append(t.lines, l)
			t.row = append(t.row, i)
			t.runes = append(t.runes, r)
			t.boxes = append(t.boxes, glyphBoxes(l, r))
		}
	}
	return t
}

func (t *textLayer) empty() bool { return t == nil || len(t.lines) == 0 }

// on reports whether (x, y), in fractions of the image, is on text: in a
// line's box, a little larger.
func (t *textLayer) on(x, y float64) bool {
	if t == nil {
		return false
	}
	for _, l := range t.lines {
		m := l.Box.H * 0.2
		if x >= l.Box.X-m && x <= l.Box.X+l.Box.W+m && y >= l.Box.Y-m && y <= l.Box.Y+l.Box.H+m {
			return true
		}
	}
	return false
}

// caretAt is the place nearest (x, y): in the line nearest it, vertically
// first, between the runes on either side of x.
func (t *textLayer) caretAt(x, y float64) caret {
	best, bestD := 0, math.Inf(1)
	for i, l := range t.lines {
		dy := max(l.Box.Y-y, y-(l.Box.Y+l.Box.H), 0)
		dx := max(l.Box.X-x, x-(l.Box.X+l.Box.W), 0)
		// A line beside the point wins over one above or below it.
		if d := dy*4 + dx; d < bestD {
			best, bestD = i, d
		}
	}
	at := 0
	for _, b := range t.boxes[best] {
		if x > b.X+b.W/2 {
			at++
		}
	}
	return caret{best, at}
}

// lineEnd is the place after the last rune of line i.
func (t *textLayer) lineEnd(i int) caret { return caret{i, len(t.runes[i])} }

// all returns the places before the first rune and after the last.
func (t *textLayer) all() (caret, caret) {
	return caret{}, t.lineEnd(len(t.lines) - 1)
}

// order returns a and b first to last.
func order(a, b caret) (caret, caret) {
	if b.before(a) {
		return b, a
	}
	return a, b
}

// span calls fn with the runes [from, to) of each line between a and b.
func (t *textLayer) span(a, b caret, fn func(line, from, to int)) {
	a, b = order(a, b)
	for i := a.line; i <= b.line && i < len(t.lines); i++ {
		from, to := 0, len(t.runes[i])
		if i == a.line {
			from = a.at
		}
		if i == b.line {
			to = b.at
		}
		if from < to {
			fn(i, from, to)
		}
	}
}

// text is the text between a and b: pieces of one row joined as Result
// joins them, rows on lines of their own, without the spaces a drag
// takes at its ends.
func (t *textLayer) text(a, b caret) string {
	var s strings.Builder
	prev, prevRow := "", -1
	t.span(a, b, func(i, from, to int) {
		piece := string(t.runes[i][from:to])
		switch {
		case prevRow < 0:
		case t.row[i] == prevRow:
			s.WriteString(ocr.Separator(prev, piece))
		default:
			s.WriteByte('\n')
		}
		s.WriteString(piece)
		prev, prevRow = piece, t.row[i]
	})
	return strings.Trim(s.String(), " ")
}

// rects are the boxes to highlight between a and b, one a line.
func (t *textLayer) rects(a, b caret) []ocr.Rect {
	var out []ocr.Rect
	t.span(a, b, func(i, from, to int) {
		first, last := t.boxes[i][from], t.boxes[i][to-1]
		box := t.lines[i].Box
		out = append(out, ocr.Rect{X: first.X, Y: box.Y, W: last.X + last.W - first.X, H: box.H})
	})
	return out
}

// glyphBoxes divides a line's box among its runes as the system's font
// spaces them. The recognizer finds the line well but places the runes in
// it poorly, Latin letters too wide and CJK characters too narrow; the text
// in screenshots is mostly in the system's font, or one of like widths.
func glyphBoxes(l ocr.Line, runes []rune) []ocr.Rect {
	lay := ui.ShapeText(l.Text, ui.Font{Size: 32}, 0)
	w, _ := lay.Size()
	xs := make([]float64, len(runes)+1)
	u := 0 // the layout's indexes count UTF-16 units
	for i, r := range runes {
		xs[i] = float64(lay.Caret(u).X)
		u += utf16.RuneLen(r)
	}
	xs[len(runes)] = float64(w)
	for i := 1; i < len(xs); i++ {
		if xs[i] < xs[i-1] || w <= 0 {
			return l.CharBoxes() // right to left, or not shaped
		}
	}
	k := l.Box.W / (xs[len(runes)] - xs[0])
	out := make([]ocr.Rect, len(runes))
	for i := range runes {
		out[i] = ocr.Rect{X: l.Box.X + (xs[i]-xs[0])*k, Y: l.Box.Y, W: (xs[i+1] - xs[i]) * k, H: l.Box.H}
	}
	return out
}
