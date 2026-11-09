package main

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/permissionflow"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/icon.png
var iconPNG []byte

var (
	okColor   = ui.RGB(0x34, 0xC7, 0x59)
	waitColor = ui.RGB(0xFF, 0x9F, 0x0A)
)

// screenGranted reports whether the app has the Screen Recording
// permission, which systems without one always give.
func screenGranted() bool {
	st, err := permissionflow.Check(permissionflow.ScreenRecording)
	return st == permissionflow.Granted || errors.Is(err, errors.ErrUnsupported)
}

// welcome is the window that sets InfiShot up: it gets the Screen Recording
// permission before the first capture, then shows the shortcuts. macOS
// gives a running app the permission only once it starts again, so after
// granting it, it offers to restart.
type welcome struct {
	a       *app
	win     *mygo.Window
	icon    *ui.Bitmap
	granted bool
	asked   bool // the user went to grant it
	stop    chan struct{}
}

func (a *app) openWelcome() {
	if w := a.welcome; w != nil && !w.win.IsDestroyed() {
		mygo.App.Focus()
		w.win.Focus()
		return
	}
	w := &welcome{a: a, granted: screenGranted(), stop: make(chan struct{})}
	w.icon, _ = ui.DecodeBitmap(iconPNG)
	w.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           "欢迎使用 InfiShot",
		Width:           480,
		Height:          400,
		DisableResize:   true,
		DisableMaximize: true,
		Content:         ui.View(w.view),
	})
	w.win.OnClosed(func() {
		close(w.stop)
		if a.welcome == w {
			a.welcome = nil
		}
		if w.granted {
			a.setWelcomed()
		}
	})
	a.welcome = w
	mygo.App.Focus() // an accessory app's windows come forward only when it is active
	w.win.Focus()

	// The permission changes in System Settings, out of the app's sight.
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
			}
			if g := screenGranted(); g != w.granted {
				w.win.Update(func() { w.granted = g })
			}
		}
	}()
}

// mustRestart reports whether the app got the permission after it
// started, which it can use only once it starts again.
func (a *app) mustRestart() bool { return !a.grantedAtStart }

func (w *welcome) view(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyW, w.win.Close)
	t := c.Theme()
	ui.Column(c).Fill().Padding(28).Gap(18).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			if w.icon != nil {
				ui.Image(c, w.icon).Size(56, 56)
			}
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, "欢迎使用 InfiShot").FontSize(20).Bold()
				ui.Text(c, "截图 · 标注 · 贴图 · 文字识别").FontSize(13).TextColor(t.TextMuted)
			})
		})

		// The permission.
		ui.Row(c).Gap(12).Padding(14).Radius(10).AlignItems(ui.Center).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			dot := waitColor
			if w.granted {
				dot = okColor
			}
			ui.Box(c).Size(10, 10).Radius(5).Background(dot)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "屏幕录制权限").FontSize(14).Bold()
				switch {
				case !w.granted:
					ui.Text(c, "截图需要这个权限，否则只能截到桌面壁纸。").FontSize(12).TextColor(t.TextMuted)
				case w.a.mustRestart():
					ui.Text(c, "已授权，重新打开 InfiShot 后生效。").FontSize(12).TextColor(t.TextMuted)
				default:
					ui.Text(c, "已授权").FontSize(12).TextColor(t.TextMuted)
				}
			})
			if !w.granted {
				if ui.PrimaryButton(c.Key("grant"), "前往授权").Clicked() {
					w.grant()
				}
			}
		})

		if !w.granted {
			msg := "InfiShot 只在你截图时读取一次屏幕画面，不会录制，也不会上传任何内容。"
			if w.asked {
				msg = "在「系统设置 › 隐私与安全性 › 屏幕与系统录音」中打开 InfiShot 的开关。" +
					"列表中没有 InfiShot 时，把系统设置下方面板里的图标拖进列表。授权后这里会自动更新。"
			}
			ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted).Wrap()
		} else {
			cfg := w.a.cfg
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "InfiShot 常驻在菜单栏，用快捷键开始：").FontSize(13).TextColor(t.TextMuted)
				for _, s := range []struct{ acc, what string }{
					{cfg.CaptureShortcut, "截图，可以标注、贴图、保存"},
					{cfg.OCRShortcut, "截图并识别文字"},
					{cfg.PinShortcut, "把剪贴板中的图片贴到屏幕上"},
				} {
					if s.acc == "" {
						continue
					}
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, shortcutLabel(s.acc)).FontSize(13).Bold().Width(64)
						ui.Text(c, s.what).FontSize(13)
					})
				}
			})
		}

		ui.Spacer(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c)
			switch {
			case !w.granted:
				if ui.Button(c.Key("later"), "稍后").Clicked() {
					w.win.Close()
				}
			case w.a.mustRestart():
				if ui.PrimaryButton(c.Key("restart"), "重新打开 InfiShot").Clicked() {
					w.a.setWelcomed()
					mygo.App.Relaunch()
				}
			default:
				if ui.PrimaryButton(c.Key("start"), "开始使用").Clicked() {
					w.win.Close()
				}
			}
		})
	})
}

// grant asks for the permission: the system's prompt the first time, which
// also adds the app to the list in System Settings, then System Settings
// with a panel of the app to drag into the list.
func (w *welcome) grant() {
	w.asked = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if st, _ := permissionflow.Request(ctx, permissionflow.ScreenRecording); st == permissionflow.Granted {
			return
		}
		_, _ = permissionflow.Guide(ctx, permissionflow.ScreenRecording, permissionflow.GuideOptions{Name: "InfiShot"})
	}()
}

// welcomed reports whether the welcome has been through once with the
// permission, after which it shows only while the permission is missing.
func (a *app) welcomed() bool {
	_, err := os.Stat(a.welcomedPath())
	return err == nil
}

func (a *app) setWelcomed() {
	if p := a.welcomedPath(); p != "" {
		_ = os.WriteFile(p, nil, 0o644)
	}
}

func (a *app) welcomedPath() string {
	if a.cfgPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.cfgPath), ".welcomed")
}

// shortcutLabel writes an accelerator as macOS shows them: "Cmd+Ctrl+A"
// becomes "⌃⌘A".
func shortcutLabel(acc string) string {
	var mods [4]bool // ⌃ ⌥ ⇧ ⌘, in the order macOS writes them
	key := ""
	for _, part := range strings.Split(acc, "+") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			mods[0] = true
		case "alt", "option":
			mods[1] = true
		case "shift":
			mods[2] = true
		case "cmd", "command", "super", "meta", "cmdorctrl", "commandorcontrol":
			mods[3] = true
		default:
			key = strings.ToUpper(strings.TrimSpace(part))
		}
	}
	var b strings.Builder
	for i, sym := range []string{"⌃", "⌥", "⇧", "⌘"} {
		if mods[i] {
			b.WriteString(sym)
		}
	}
	b.WriteString(key)
	return b.String()
}
