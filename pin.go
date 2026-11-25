package main

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/ocr"
)

// pin is an image kept on top of every window, as a reference to look at
// while working elsewhere. Drag it to move it, scroll to zoom it, Option
// and scroll to fade it, double-click or Esc to close it.
type pin struct {
	a    *app
	win  *mygo.Window
	img  *image.RGBA
	bmp  *ui.Bitmap
	w, h float64 // its size at 100%, in DIPs

	zoom, opacity float64
	hint          string    // a passing label, as the zoom after it changes
	hintUntil     time.Time // when the label goes
	focused       bool

	dragging      bool
	dragFrom, at0 mygo.Point // the pointer and the window when a drag started

	// The text in the image, recognized in the background, to select
	// with the pointer as in a document.
	text          *textLayer
	recognizing   bool
	anchor, focus caret // the selection, empty when they are the same
	selecting     bool  // whether a drag is selecting text
	ptr           ui.Point
	size          ui.Point // the window's, in DIPs
}

func newPin(a *app, img *image.RGBA, at mygo.Rectangle) *pin {
	p := &pin{
		a:       a,
		img:     img,
		bmp:     ui.NewBitmap(img),
		w:       float64(at.Width),
		h:       float64(at.Height),
		zoom:    1,
		opacity: 1,
	}
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "InfiShot 贴图",
		X:                 at.X,
		Y:                 at.Y,
		Width:             at.Width,
		Height:            at.Height,
		Frameless:         true,
		Panel:             true, // over full-screen apps, on every Space, out of the Dock
		AlwaysOnTop:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Content:           ui.View(p.view),
	})
	placeFreely(p.win)
	// Keep newly created pins below any already-open save panel; the last
	// panel's completion handler raises all open pins once it closes.
	if a.saveOpen() {
		lowerPin(p.win)
	} else {
		raisePin(p.win)
	}
	// Where it was taken from, even at (0, 0), which would center it.
	p.win.SetBounds(at)
	p.win.OnFocus(func() { p.focused = true; p.win.Invalidate() })
	p.win.OnBlur(func() { p.focused = false; p.win.Invalidate() })
	p.win.OnClosed(func() { a.removePin(p) })
	p.win.Focus()
	if a.cfg.PinSelectText {
		p.recognizeText()
	}
	return p
}

func (p *pin) view(c *ui.Context) {
	c.OnShortcut(0, ui.KeyEscape, func() {
		if p.hasSelection() {
			p.focus = p.anchor
		} else {
			p.close()
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyA, p.selectAll)
	c.OnShortcut(ui.Cmd, ui.KeyW, p.close)
	c.OnShortcut(ui.Cmd, ui.KeyC, p.copy)
	c.OnShortcut(ui.Cmd, ui.KeyS, p.save)
	c.OnShortcut(ui.Cmd, ui.KeyO, p.recognize)
	c.OnShortcut(ui.Cmd, ui.Key0, func() { p.setZoom(1, -1, -1) })
	c.OnShortcut(ui.Cmd, ui.KeyEqual, func() { p.setZoom(p.zoom*1.25, -1, -1) })
	c.OnShortcut(ui.Cmd, ui.KeyMinus, func() { p.setZoom(p.zoom/1.25, -1, -1) })

	w, h := c.Size()
	p.size = ui.Point{X: w, Y: h}
	ui.Box(c).Absolute().Top(0).Left(0).Size(w, h).
		Cursor(p.cursor()).
		Draw(p.paint).
		HandleInput(p.input).
		ContextMenu(p.menu)
}

func (p *pin) paint(pt *ui.Painter, r ui.Rect) {
	pt.Image(p.bmp, r, ui.FillBox)
	if p.hasSelection() {
		for _, b := range p.text.rects(p.anchor, p.focus) {
			pt.Fill(ui.Rect{X: r.X + float32(b.X)*r.W, Y: r.Y + float32(b.Y)*r.H, W: float32(b.W) * r.W, H: float32(b.H) * r.H},
				ui.RGBA(0x1E, 0x90, 0xFF, 0.3), 2)
		}
	}
	border := ui.RGBA(0, 0, 0, 0.25)
	if p.focused {
		border = accent
	}
	pt.Stroke(r, border, 0, 1)
	if now := pt.Now(); now.Before(p.hintUntil) {
		lbl := ui.Span{Text: p.hint, Size: 12, Weight: 500, Color: ui.RGB(255, 255, 255)}
		w, h := pt.MeasureText(0, lbl)
		box := ui.Rect{X: r.X + (r.W-w-12)/2, Y: r.Y + (r.H-h-6)/2, W: w + 12, H: h + 6}
		pt.Fill(box, ui.RGBA(0, 0, 0, 0.7), 4)
		pt.RichText(box.X+6, box.Y+3, 0, lbl)
		pt.After(p.hintUntil.Sub(now))
	}
}

func (p *pin) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false // the context menu
		}
		p.ptr = ui.Point{X: ev.X, Y: ev.Y}
		x, y := p.frac(p.ptr)
		onText := p.text.on(x, y)
		switch {
		case onText && ev.Clicks >= 2:
			// A double click on text selects its line.
			c := p.text.caretAt(x, y)
			p.anchor, p.focus = caret{line: c.line}, p.text.lineEnd(c.line)
			p.selecting = false
			return true
		case ev.Clicks >= 2:
			p.close()
			return true
		case onText:
			p.anchor = p.text.caretAt(x, y)
			p.focus, p.selecting = p.anchor, true
			return true
		}
		p.focus = p.anchor // a press beside the text lets go of the selection
		// The window moves under the pointer, so follow the pointer on the
		// screen rather than in the window.
		p.dragging = true
		p.dragFrom = mygo.Screen.CursorScreenPoint()
		p.at0.X, p.at0.Y = p.win.Position()
		return true
	case ui.InputPointerMove:
		p.ptr = ui.Point{X: ev.X, Y: ev.Y}
		if p.selecting && ev.Button == 0 {
			p.focus = p.text.caretAt(p.frac(p.ptr))
			return true
		}
		if p.dragging && ev.Button == 0 {
			cur := mygo.Screen.CursorScreenPoint()
			p.win.SetPosition(p.at0.X+cur.X-p.dragFrom.X, p.at0.Y+cur.Y-p.dragFrom.Y)
		}
		return true
	case ui.InputPointerUp:
		if p.selecting {
			p.selecting = false
			return ev.Button == 0
		}
		if ev.Canceled && p.dragging {
			p.win.SetPosition(p.at0.X, p.at0.Y)
		}
		p.dragging = false
		return ev.Button == 0
	case ui.InputScroll:
		dy := float64(ev.DY)
		if !ev.Precise {
			dy *= 4 // a wheel's notch
		}
		if ev.Mods&ui.Alt != 0 {
			p.setOpacity(p.opacity - dy/400)
		} else {
			p.setZoom(p.zoom*math.Exp(-dy/300), float64(ev.X), float64(ev.Y))
		}
		return true
	case ui.InputZoom:
		p.setZoom(p.zoom*float64(ev.Scale), float64(ev.X), float64(ev.Y))
		return true
	}
	return false
}

func (p *pin) menu(m *ui.Menu) {
	if p.hasSelection() {
		if m.Item("复制文字").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
			p.copy()
		}
		if m.Item("复制图片").Chosen() {
			p.copyImage()
		}
	} else if m.Item("复制图片").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		p.copyImage()
	}
	switch {
	case !p.text.empty():
		if m.Item("复制全部文字").Chosen() {
			p.copyText(p.text.text(p.text.all()))
		}
		if m.Item("全选文字").Shortcut(ui.Cmd, ui.KeyA).Chosen() {
			p.selectAll()
		}
	case p.recognizing:
		m.Item("正在识别文字…").Disabled(true)
	}
	if m.Item("另存为…").Shortcut(ui.Cmd, ui.KeyS).Chosen() {
		p.save()
	}
	if m.Item("识别文字").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
		p.recognize()
	}
	m.Separator()
	if m.Item("原始大小").Shortcut(ui.Cmd, ui.Key0).Disabled(p.zoom == 1).Chosen() {
		p.setZoom(1, -1, -1)
	}
	m.Submenu("不透明度", func(m *ui.Menu) {
		for _, v := range []float64{1, 0.8, 0.6, 0.4} {
			if m.Item(fmt.Sprintf("%.0f%%", v*100)).Radio(math.Abs(p.opacity-v) < 0.01).Chosen() {
				p.setOpacity(v)
			}
		}
	})
	m.Separator()
	if m.Item("关闭").Shortcut(0, ui.KeyEscape).Chosen() {
		p.close()
	}
	if m.Item("关闭所有贴图").Chosen() {
		p.a.closePins()
	}
}

// setZoom resizes the window to zoom times the image's size, keeping the
// point (x, y) of the window where it is, or the center for x < 0.
func (p *pin) setZoom(zoom, x, y float64) {
	zoom = math.Max(0.1, math.Min(zoom, 8))
	if math.Abs(zoom-1) < 0.02 {
		zoom = 1 // snap to the original size
	}
	b := p.win.Bounds()
	w, h := math.Max(8, p.w*zoom), math.Max(8, p.h*zoom)
	if x < 0 {
		x, y = float64(b.Width)/2, float64(b.Height)/2
	}
	kx, ky := w/float64(b.Width), h/float64(b.Height)
	p.win.SetBounds(mygo.Rectangle{
		X:      b.X + int(math.Round(x-x*kx)),
		Y:      b.Y + int(math.Round(y-y*ky)),
		Width:  int(math.Round(w)),
		Height: int(math.Round(h)),
	})
	p.zoom = zoom
	p.flash(fmt.Sprintf("%.0f%%", zoom*100))
}

func (p *pin) setOpacity(v float64) {
	p.opacity = math.Max(0.2, math.Min(v, 1))
	p.win.SetOpacity(p.opacity)
	p.flash(fmt.Sprintf("不透明度 %.0f%%", p.opacity*100))
}

func (p *pin) flash(s string) {
	p.hint, p.hintUntil = s, time.Now().Add(800*time.Millisecond)
	p.win.Invalidate()
}

// copy copies the selected text, or the image when none is selected.
func (p *pin) copy() {
	if p.hasSelection() {
		p.copyText(p.text.text(p.anchor, p.focus))
		return
	}
	p.copyImage()
}

func (p *pin) copyImage() {
	p.a.use(actCopy, p.img, mygo.Rectangle{})
	p.flash("已复制图片")
}

func (p *pin) copyText(s string) {
	mygo.Clipboard.WriteText(s)
	p.flash("已复制文字")
}

func (p *pin) hasSelection() bool { return p.text != nil && p.anchor != p.focus }

func (p *pin) selectAll() {
	if !p.text.empty() {
		p.anchor, p.focus = p.text.all()
	}
}

// frac is pt in fractions of the window, which shows the whole image.
func (p *pin) frac(pt ui.Point) (float64, float64) {
	if p.size.X <= 0 || p.size.Y <= 0 {
		return -1, -1
	}
	return float64(pt.X / p.size.X), float64(pt.Y / p.size.Y)
}

func (p *pin) cursor() ui.Cursor {
	if p.selecting || !p.dragging && p.text.on(p.frac(p.ptr)) {
		return ui.CursorText
	}
	return ui.CursorGrab
}

// recognizeText finds the text in the image in the background, for the
// pointer to select.
func (p *pin) recognizeText() {
	p.recognizing = true
	img, langs := p.img, p.a.cfg.OCRLanguages
	go func() {
		data, err := encodePNG(img, png.BestSpeed)
		var res ocr.Result
		if err == nil {
			res, err = ocr.Recognize(data, langs...)
		}
		p.win.Update(func() {
			p.recognizing = false
			if err != nil {
				if !errors.Is(err, ocr.ErrUnsupported) {
					log.Printf("贴图文字识别: %v", err)
				}
				return
			}
			p.text = newTextLayer(res)
			if !p.text.empty() && !p.dragging {
				p.flash("可以直接选择文字")
			}
		})
	}()
}

func (p *pin) save() { p.a.use(actSave, p.img, mygo.Rectangle{}) }

func (p *pin) recognize() { p.a.use(actOCR, p.img, p.win.Bounds()) }

func (p *pin) close() { p.win.Close() }
