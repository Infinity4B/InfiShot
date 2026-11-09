package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Infinity4B/infishot/internal/ocr"
)

// ocrWindow shows the text recognized in an image, editable, to copy.
type ocrWindow struct {
	a      *app
	win    *mygo.Window
	thumb  *ui.Bitmap
	text   string
	status string
	busy   bool
	failed bool
}

func newOCRWindow(a *app, img *image.RGBA, near mygo.Rectangle) *ocrWindow {
	o := &ocrWindow{a: a, thumb: ui.NewBitmap(img), busy: true, status: "正在识别…"}
	const w, h = 520, 400
	opts := mygo.WindowOptions{
		Title:     "文字识别 - InfiShot",
		Width:     w,
		Height:    h,
		MinWidth:  360,
		MinHeight: 260,
		Content:   ui.View(o.view),
	}
	// Beside what was recognized when there is room, else centered.
	if near.Width > 0 {
		area := mygo.Screen.DisplayMatching(near).WorkArea
		x, y := near.X+near.Width+12, near.Y
		if x+w > area.X+area.Width {
			x = near.X - w - 12
		}
		if x >= area.X {
			opts.X = x
			opts.Y = max(area.Y, min(y, area.Y+area.Height-h))
		}
	}
	o.win = mygo.NewWindow(opts)
	o.win.OnClosed(func() { a.removeOCR(o) })
	mygo.App.Focus() // an accessory app gives its windows the keyboard only when active
	o.win.Focus()

	langs := a.cfg.OCRLanguages
	go func() {
		start := time.Now()
		var buf bytes.Buffer
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		err := enc.Encode(&buf, img)
		var res ocr.Result
		if err == nil {
			res, err = ocr.Recognize(buf.Bytes(), langs...)
		}
		elapsed := time.Since(start)
		o.win.Update(func() { o.done(res, err, elapsed) })
	}()
	go func() {
		// The first recognition after installing or updating the app
		// loads and compiles the system's model, which takes a while;
		// later ones take a fraction of a second.
		time.Sleep(2 * time.Second)
		o.win.Update(func() {
			if o.busy {
				o.status = "首次识别需要加载系统模型，可能需要十几秒…"
			}
		})
	}()
	return o
}

func (o *ocrWindow) done(res ocr.Result, err error, elapsed time.Duration) {
	o.busy = false
	switch {
	case err != nil:
		o.failed = true
		o.status = err.Error()
	case len(res.Lines) == 0:
		o.status = "没有识别到文字"
	default:
		o.text = res.Text()
		o.status = fmt.Sprintf("识别到 %d 行，用时 %s", len(res.Lines), elapsed.Round(10*time.Millisecond))
		if o.a.cfg.OCRAutoCopy {
			mygo.Clipboard.WriteText(o.text)
			o.status += "，已复制"
		}
	}
}

func (o *ocrWindow) view(c *ui.Context) {
	c.OnShortcut(0, ui.KeyEscape, o.win.Close)
	c.OnShortcut(ui.Cmd, ui.KeyW, o.win.Close)
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(12).Height(56).Children(func() {
			ui.Image(c, o.thumb).Size(96, 56).Fit(ui.Contain).Radius(4)
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, "文字识别").FontSize(16).Bold()
				status := ui.Text(c, o.status).FontSize(12)
				if o.failed {
					status.TextColor(ui.RGB(0xE5, 0x48, 0x4D))
				} else {
					status.TextColor(t.TextMuted)
				}
			})
			if o.busy {
				ui.Spinner(c)
			}
		})
		ui.TextArea(c.Key("text"), &o.text).Grow(1).Label("识别结果").
			Placeholder("识别出的文字会显示在这里，可直接编辑").ReadOnly(o.busy)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "Esc 关闭").FontSize(11).TextColor(t.TextMuted)
			ui.Spacer(c)
			if ui.Button(c, "关闭").Clicked() {
				o.win.Close()
			}
			if ui.PrimaryButton(c, "复制全部").Disabled(o.text == "").Clicked() {
				mygo.Clipboard.WriteText(o.text)
				o.status = "已复制到剪贴板"
			}
		})
	})
}
