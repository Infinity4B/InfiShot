package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"

	"github.com/Infinity4B/infishot/internal/capture"
)

// app holds InfiShot's state. Everything here runs on the main thread:
// the tray's and the shortcuts' callbacks, and the windows' views.
type app struct {
	cfg     config
	cfgPath string
	tray    *mygo.Tray
	overlay *overlay
	pins    []*pin
	ocrs    []*ocrWindow
	failed  []string // shortcuts that could not be registered
	welcome *welcome
	// saves tracks open save panels keyed by image, so repeated ⌘S on the same
	// image focuses its panel while a different image opens its own panel.
	// Open pins stay lowered whenever len(saves) > 0.
	saves     map[*image.RGBA]saveHandle
	savePanel savePanel // nil uses platformSavePanel
	// grantedAtStart is whether the app had the Screen Recording
	// permission when it started: macOS lets it capture only then.
	grantedAtStart bool
}

func (a *app) ready() {
	cfg, path, err := loadConfig()
	if err != nil {
		log.Printf("配置文件 %s: %v（使用默认配置）", path, err)
	}
	a.cfg, a.cfgPath = cfg, path
	a.grantedAtStart = screenGranted()

	for _, s := range []struct {
		acc string
		fn  func()
	}{
		{a.cfg.CaptureShortcut, func() { a.capture(false) }},
		{a.cfg.OCRShortcut, func() { a.capture(true) }},
		{a.cfg.PinShortcut, a.pinClipboard},
	} {
		if s.acc == "" {
			continue
		}
		if err := mygo.GlobalShortcut.Register(s.acc, s.fn); err != nil {
			log.Printf("快捷键 %s: %v", s.acc, err)
			a.failed = append(a.failed, s.acc)
		}
	}

	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		ID:             "infishot",
		Icon:           trayIcon(),
		IconIsTemplate: true,
		ToolTip:        "InfiShot 截图",
		Menu:           a.menu(),
	})
	if err != nil {
		log.Fatal("托盘图标: ", err)
	}
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })

	// Set up before the first capture: the permission, then the shortcuts.
	if !a.grantedAtStart || !a.welcomed() {
		a.openWelcome()
	}

	if len(a.failed) > 0 {
		a.alert("部分快捷键不可用",
			"以下快捷键已被其他应用占用："+strings.Join(a.failed, "、")+
				"\n\n可以在托盘菜单「打开配置文件」中修改，重启 InfiShot 后生效。")
	}
}

func (a *app) menu() *mygo.Menu {
	later := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		// Let the menu close before the screen is captured.
		return func(*mygo.MenuItem, *mygo.Window) {
			go func() {
				time.Sleep(250 * time.Millisecond)
				mygo.RunOnMain(fn)
			}()
		}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "截图", Accelerator: a.cfg.CaptureShortcut, Click: later(func() { a.capture(false) })},
		{Label: "截图并识别文字", Accelerator: a.cfg.OCRShortcut, Click: later(func() { a.capture(true) })},
		{Label: "从剪贴板贴图", Accelerator: a.cfg.PinShortcut, Click: func(*mygo.MenuItem, *mygo.Window) { a.pinClipboard() }},
		{Label: "关闭所有贴图", Click: func(*mygo.MenuItem, *mygo.Window) { a.closePins() }},
		mygo.Separator(),
		{Label: "打开配置文件…", Click: func(*mygo.MenuItem, *mygo.Window) {
			if err := mygo.Shell.OpenPath(a.cfgPath); err != nil {
				a.fail("无法打开配置文件", err)
			}
		}},
		{Label: "打开截图文件夹", Click: func(*mygo.MenuItem, *mygo.Window) {
			dir := a.cfg.saveDir()
			_ = os.MkdirAll(dir, 0o755)
			if err := mygo.Shell.OpenPath(dir); err != nil {
				a.fail("无法打开文件夹", err)
			}
		}},
		mygo.Separator(),
		{Label: "退出 InfiShot", Role: mygo.RoleQuit},
	})
}

// capture photographs the display under the pointer and opens the overlay
// to select an area of it. With ocrOnly, the selection is recognized as
// soon as it is made.
func (a *app) capture(ocrOnly bool) {
	if a.overlay != nil && !a.overlay.win.IsDestroyed() {
		a.overlay.win.Focus()
		return
	}
	if !a.mayCapture() {
		return
	}
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	b := d.Bounds
	// Listed before the overlay covers them, though it leaves out this
	// process's windows anyway.
	wins, err := capture.Windows()
	if err != nil {
		log.Printf("窗口列表: %v", err)
	}
	img, err := capture.Region(capture.Rect{X: b.X, Y: b.Y, W: b.Width, H: b.Height})
	if err != nil {
		a.fail("截图失败", err)
		return
	}
	a.overlay = newOverlay(a, d, img, wins, ocrOnly)
}

// mayCapture reports whether the app may capture the screen, and otherwise
// opens the welcome window to set it up. Without the Screen Recording
// permission macOS captures the wallpaper and nothing else, and an app
// that got it after it started must start again.
func (a *app) mayCapture() bool {
	if a.grantedAtStart && screenGranted() {
		return true
	}
	a.openWelcome()
	return false
}

// use does what the user chose with a captured image. at is where the
// image is on screen, in desktop points, for those that place a window
// by it.
func (a *app) use(act action, img *image.RGBA, at mygo.Rectangle) {
	switch act {
	case actCopy:
		go func() {
			data, err := encodePNG(img, png.BestSpeed)
			if err == nil {
				err = mygo.Clipboard.Write(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
			}
			if err != nil {
				a.fail("复制失败", err)
			}
		}()
	case actSave:
		a.save(img)
	case actPin:
		a.pins = append(a.pins, newPin(a, img, at))
	case actOCR:
		a.ocrs = append(a.ocrs, newOCRWindow(a, img, at))
	}
}

type saveOptions struct {
	title string
	dir   string
	name  string
}

// savePanel presents the native save dialog without blocking the main thread.
// Platform files provide platformSavePanel.
type savePanel interface {
	// show opens a save panel and returns a handle that brings it forward
	// while open. It calls done on the main thread when the panel closes with
	// the chosen path, or "" if cancelled. If done is invoked synchronously
	// (e.g. on setup failure), show may return nil.
	show(opts saveOptions, done func(path string, err error)) saveHandle
}

// saveHandle refers to an open save panel on the main thread.
type saveHandle interface {
	focus()
}

// saveOpen reports whether any save panel is currently open.
func (a *app) saveOpen() bool { return len(a.saves) > 0 }

// save opens a save panel for img on the main thread, lowering open pins while
// any save panel is visible so they cannot cover it. A repeated request for an
// image that already has an open panel brings that panel forward; a request for
// a different image opens its own modeless panel.
func (a *app) save(img *image.RGBA) {
	mygo.RunOnMain(func() { a.saveOnMain(img) })
}

func (a *app) saveOnMain(img *image.RGBA) {
	if h, ok := a.saves[img]; ok {
		if h != nil {
			h.focus()
		}
		return
	}
	panel := a.savePanel
	if panel == nil {
		panel = platformSavePanel
	}
	if a.saves == nil {
		a.saves = map[*image.RGBA]saveHandle{}
	}
	first := len(a.saves) == 0
	// Reserve the slot before show so a synchronous done callback deletes it
	// cleanly without leaving a stale entry behind.
	var closed bool
	a.saves[img] = nil
	if first {
		a.lowerPins()
	}

	dir := a.cfg.saveDir()
	_ = os.MkdirAll(dir, 0o755)
	opts := saveOptions{
		title: "保存截图",
		dir:   dir,
		name:  "InfiShot_" + time.Now().Format("20060102_150405") + ".png",
	}
	h := panel.show(opts, func(path string, err error) {
		closed = true
		delete(a.saves, img)
		if len(a.saves) == 0 {
			a.raisePins()
		}
		if err != nil {
			log.Printf("保存截图: %v", err)
			return
		}
		if path == "" {
			return
		}
		path = ensurePNGExt(path)
		go func() {
			if err := writeSavedPNG(path, img); err != nil {
				a.fail("保存失败", err)
			}
		}()
	})
	if !closed {
		a.saves[img] = h
	}
}

func (a *app) lowerPins() {
	for _, p := range a.pins {
		if p.win != nil && !p.win.IsDestroyed() {
			lowerPin(p.win)
		}
	}
}

func (a *app) raisePins() {
	for _, p := range a.pins {
		if p.win != nil && !p.win.IsDestroyed() {
			raisePin(p.win)
		}
	}
}

// ensurePNGExt appends ".png" when path does not already end in it.
func ensurePNGExt(path string) string {
	if path == "" || strings.EqualFold(filepath.Ext(path), ".png") {
		return path
	}
	return path + ".png"
}

func writeSavedPNG(path string, img image.Image) error {
	data, err := encodePNG(img, png.DefaultCompression)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// pinClipboard pins the image on the clipboard under the pointer.
func (a *app) pinClipboard() {
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 {
		a.alert("剪贴板中没有图片", "先复制一张图片，再使用「从剪贴板贴图」。")
		return
	}
	// In sRGB, whatever profile the image carries.
	img, err := capture.Decode(data)
	if err != nil {
		a.fail("无法读取剪贴板中的图片", err)
		return
	}
	cur := mygo.Screen.CursorScreenPoint()
	d := mygo.Screen.DisplayNearestPoint(cur)
	scale := d.ScaleFactor
	if scale <= 0 {
		scale = 1
	}
	// At the display's scale, as a screenshot of it would be, and no
	// larger than most of its work area.
	w, h := float64(img.Rect.Dx())/scale, float64(img.Rect.Dy())/scale
	area := d.WorkArea
	if k := min(1, 0.8*float64(area.Width)/w, 0.8*float64(area.Height)/h); k < 1 {
		w, h = w*k, h*k
	}
	at := mygo.Rectangle{Width: max(1, int(w)), Height: max(1, int(h))}
	at.X = clampInt(cur.X-at.Width/2, area.X, area.X+area.Width-at.Width)
	at.Y = clampInt(cur.Y-at.Height/2, area.Y, area.Y+area.Height-at.Height)
	a.pins = append(a.pins, newPin(a, img, at))
}

func (a *app) closePins() {
	for _, p := range slices.Clone(a.pins) {
		p.close()
	}
}

func (a *app) removePin(p *pin) {
	a.pins = slices.DeleteFunc(a.pins, func(q *pin) bool { return q == p })
}

func (a *app) removeOCR(o *ocrWindow) {
	a.ocrs = slices.DeleteFunc(a.ocrs, func(q *ocrWindow) bool { return q == o })
}

// fail reports an error in a dialog, without blocking the caller.
func (a *app) fail(title string, err error) {
	log.Printf("%s: %v", title, err)
	a.alert(title, err.Error())
}

func (a *app) alert(title, detail string) {
	go func() {
		mygo.App.Focus()
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{
			Type:    mygo.MessageWarning,
			Title:   title,
			Message: title,
			Detail:  detail,
			Buttons: []string{"好"},
		})
	}()
}

// encodePNG encodes img, marked as sRGB, which its pixels are in (see
// capture.Decode).
func encodePNG(img image.Image, level png.CompressionLevel) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("PNG 编码: %w", err)
	}
	return withSRGB(buf.Bytes()), nil
}

// withSRGB adds an sRGB chunk to a PNG after its header (IHDR), for the
// apps that take an image without one to be in another space.
func withSRGB(data []byte) []byte {
	const ihdrEnd = 8 + 8 + 13 + 4 // signature, then IHDR: length and type, data, CRC
	if len(data) < ihdrEnd || string(data[12:16]) != "IHDR" {
		return data
	}
	chunk := []byte{0, 0, 0, 1, 's', 'R', 'G', 'B', 0} // length 1, type, perceptual intent
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, data[ihdrEnd:]...)
}
