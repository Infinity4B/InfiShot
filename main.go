// InfiShot is a small screenshot tool: capture an area of the screen, pin
// it above every window, or recognize the text in it. It lives in the
// menu bar and is driven by global shortcuts.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
)

const usage = `InfiShot — 截图、贴图、文字识别

用法:
  infishot           启动（常驻菜单栏）
  infishot capture   截图
  infishot ocr       截图并识别文字
  infishot pin       把剪贴板中的图片贴到屏幕上

InfiShot 已在运行时，命令交给运行中的实例执行，可供 Raycast、Alfred、
快捷指令等调用。`

func main() {
	cmd := command(os.Args[1:])
	if cmd == "help" {
		fmt.Println(usage)
		return
	}
	mygo.App.SetName("InfiShot")
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance got the command
	}
	// A menu bar app: no Dock icon, no menu bar of its own.
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// Closing the last window (a pin, a result) must not quit the app.
	mygo.App.OnWindowAllClosed(func() {})

	a := &app{}
	// args are the second instance's, without the program's path.
	mygo.App.OnSecondInstance(func(args []string, _ string) { a.run(command(args)) })
	mygo.App.WhenReady(func() {
		a.ready()
		a.run(cmd)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// command finds the command among the arguments.
func command(args []string) string {
	for _, arg := range args {
		switch arg = strings.TrimLeft(arg, "-"); arg {
		case "capture", "ocr", "pin":
			return arg
		case "help", "h":
			return "help"
		}
	}
	return ""
}

func (a *app) run(cmd string) {
	switch cmd {
	case "capture":
		a.capture(false)
	case "ocr":
		a.capture(true)
	case "pin":
		a.pinClipboard()
	}
}

// trayIcon draws the crop symbol, black on transparent for macOS to tint
// as the menu bar is light or dark.
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	ink := image.NewUniform(color.NRGBA{A: 255})
	for _, r := range []image.Rectangle{
		image.Rect(9, 3, 12, 27), image.Rect(9, 24, 33, 27), // ⌞
		image.Rect(3, 9, 27, 12), image.Rect(24, 9, 27, 33), // ⌝
	} {
		draw.Draw(img, r, ink, image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
