package main

import "github.com/egoist/mygo/ui"

// The toolbar's icons: strokes on a 24 × 24 grid, which ui.Icon tints with
// the color of the text.
var (
	iconRect    = icon(`<rect x="4" y="5" width="16" height="14" rx="1.5"/>`)
	iconEllipse = icon(`<ellipse cx="12" cy="12" rx="8.5" ry="7"/>`)
	iconArrow   = icon(`<path d="M5 19L18.5 5.5M10 5h9v9"/>`)
	iconPen     = icon(`<path d="M15.5 4.5l4 4L8 20H4v-4z"/><path d="M13 7l4 4"/>`)
	iconMarker  = icon(`<path d="M9 14l-4.5 4.5V21h5l2-2"/><path d="M9 14l3 3 8.5-8.5a2.1 2.1 0 0 0 0-3L19.5 4.5a2.1 2.1 0 0 0-3 0z"/>`)
	iconMosaic  = icon(`<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M4 9.3h16M4 14.7h16M9.3 4v16M14.7 4v16"/>`)
	iconText    = icon(`<path d="M5 7V4.5h14V7M12 4.5v15M9 19.5h6"/>`)
	iconUndo    = icon(`<path d="M9 14L4 9l5-5"/><path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>`)
	iconOCR     = icon(`<path d="M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16"/><path d="M8 9h8M8 12h8M8 15h5"/>`)
	iconPin     = icon(`<path d="M12 16.5V22"/><path d="M9 3h6l-1 6.5 3.5 3.5v2.5h-11V13L10 9.5z"/>`)
	iconSave    = icon(`<path d="M12 4v11M7 10.5l5 5 5-5M5 20h14"/>`)
	iconClose   = icon(`<path d="M6 6l12 12M18 6L6 18"/>`)
	iconDone    = icon(`<path d="M5 12.5l4.5 4.5L19 7.5"/>`)
)

func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" ` +
		`fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}
