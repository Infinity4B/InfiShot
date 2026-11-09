// Package ocr recognizes text in images with the operating system's own
// engine: Vision on macOS. Nothing is uploaded and no model is bundled.
package ocr

import (
	"errors"
	"sort"
	"strings"
)

// ErrUnsupported is returned on systems without a text recognizer.
var ErrUnsupported = errors.New("ocr: 当前系统不支持文字识别")

// DefaultLanguages are tried, in order of preference, when none are given.
var DefaultLanguages = []string{"zh-Hans", "zh-Hant", "en-US"}

// Line is one line of recognized text.
type Line struct {
	Text       string
	Confidence float32
	// Box is where the line is in the image, in fractions of its size,
	// with the origin at the top-left corner.
	Box Rect
}

// CharBoxes estimates where each rune of the line is: its box divided
// among them, a CJK character taking twice the room of another.
func (l Line) CharBoxes() []Rect { return divide(l.Box, []rune(l.Text)) }

// divide splits box across runes from left to right by their widths.
func divide(box Rect, runes []rune) []Rect {
	out := make([]Rect, len(runes))
	total := 0.0
	for _, r := range runes {
		total += width(r)
	}
	x := box.X
	for i, r := range runes {
		w := box.W * width(r) / total
		out[i] = Rect{X: x, Y: box.Y, W: w, H: box.H}
		x += w
	}
	return out
}

func width(r rune) float64 {
	if isCJK(r) {
		return 2
	}
	return 1
}

// Separator is what goes between two pieces of text in one row: nothing
// between CJK text, which uses no spaces, and a space otherwise.
func Separator(prev, next string) string { return joiner(prev, next) }

// Rect is a rectangle in fractions of the image's size.
type Rect struct{ X, Y, W, H float64 }

// Result is the text of one image.
type Result struct {
	Lines []Line
}

// Text joins the lines in reading order: rows from top to bottom, and the
// pieces of a row from left to right.
func (r Result) Text() string {
	rows := Rows(r.Lines)
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, l := range row {
			if j > 0 {
				b.WriteString(joiner(row[j-1].Text, l.Text))
			}
			b.WriteString(l.Text)
		}
	}
	return b.String()
}

// Recognize finds the text in an encoded image (PNG, JPEG, ...).
func Recognize(image []byte, languages ...string) (Result, error) {
	if len(image) == 0 {
		return Result{}, errors.New("ocr: 图片为空")
	}
	if len(languages) == 0 {
		languages = DefaultLanguages
	}
	return recognize(image, languages)
}

// Rows groups lines that share a baseline into rows, sorted top to bottom,
// each sorted left to right. Two lines share a row when their vertical
// extents overlap by more than half the shorter one.
func Rows(lines []Line) [][]Line {
	sorted := append([]Line(nil), lines...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Box.Y+sorted[i].Box.H/2 < sorted[j].Box.Y+sorted[j].Box.H/2
	})
	var rows [][]Line
	for _, l := range sorted {
		if n := len(rows); n > 0 && sameRow(rows[n-1], l) {
			rows[n-1] = append(rows[n-1], l)
			continue
		}
		rows = append(rows, []Line{l})
	}
	for _, row := range rows {
		sort.SliceStable(row, func(i, j int) bool { return row[i].Box.X < row[j].Box.X })
	}
	return rows
}

func sameRow(row []Line, l Line) bool {
	for _, o := range row {
		top := max(o.Box.Y, l.Box.Y)
		bottom := min(o.Box.Y+o.Box.H, l.Box.Y+l.Box.H)
		if bottom-top > min(o.Box.H, l.Box.H)/2 {
			return true
		}
	}
	return false
}

// joiner separates two pieces of a row: nothing between CJK text, which
// uses no spaces, and a space otherwise.
func joiner(prev, next string) string {
	if prev == "" || next == "" {
		return ""
	}
	last := []rune(prev)
	if isCJK(last[len(last)-1]) && isCJK([]rune(next)[0]) {
		return ""
	}
	return " "
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x3000 && r <= 0x303F, // CJK punctuation
		r >= 0x3040 && r <= 0x30FF, // kana
		r >= 0x3400 && r <= 0x4DBF, // extension A
		r >= 0x4E00 && r <= 0x9FFF, // unified ideographs
		r >= 0xAC00 && r <= 0xD7AF, // hangul
		r >= 0xF900 && r <= 0xFAFF, // compatibility ideographs
		r >= 0xFF00 && r <= 0xFFEF: // full-width forms
		return true
	}
	return false
}
