//go:build darwin

package capture

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ebitengine/purego"

	"github.com/Infinity4B/infishot/internal/macos"
)

type cgRect struct{ X, Y, W, H float64 }

var (
	getActiveDisplayList func(max uint32, displays *uint32, count *uint32) int32
	displayBounds        func(display uint32) cgRect
)

// display is a display as screencapture numbers them: 1 for the main
// display, then in the order of CGGetActiveDisplayList.
type display struct {
	index  int
	bounds cgRect
}

func displays() ([]display, error) {
	if err := macos.Load(); err != nil {
		return nil, err
	}
	if displayBounds == nil {
		lib, err := macos.Dlopen(coreGraphics)
		if err != nil {
			return nil, err
		}
		purego.RegisterLibFunc(&getActiveDisplayList, lib, "CGGetActiveDisplayList")
		purego.RegisterLibFunc(&displayBounds, lib, "CGDisplayBounds")
	}
	var ids [32]uint32
	var n uint32
	if rc := getActiveDisplayList(uint32(len(ids)), &ids[0], &n); rc != 0 {
		return nil, fmt.Errorf("capture: CGGetActiveDisplayList: %d", rc)
	}
	out := make([]display, n)
	for i := range out {
		out[i] = display{index: i + 1, bounds: displayBounds(ids[i])}
	}
	return out, nil
}

// region captures the display that holds r's center with the system's
// screencapture tool, then crops r out of it. screencapture is part of
// every macOS, handles every scale, and goes through the system's Screen
// Recording permission like any capture does; its -R option takes no
// negative coordinates, which displays above or left of the main one
// have, so it gets the display by number (-D) instead. The CoreGraphics
// capture functions are obsolete since macOS 15, and ScreenCaptureKit
// takes callbacks and blocks that a cgo-free build reaches only the long
// way around.
func region(r Rect) (image.Image, error) {
	ds, err := displays()
	if err != nil {
		return nil, err
	}
	cx, cy := float64(r.X)+float64(r.W)/2, float64(r.Y)+float64(r.H)/2
	var d *display
	for i := range ds {
		b := ds[i].bounds
		if cx >= b.X && cx < b.X+b.W && cy >= b.Y && cy < b.Y+b.H {
			d = &ds[i]
			break
		}
	}
	if d == nil {
		return nil, errors.New("capture: 区域不在任何显示器上")
	}
	img, err := screencapture(d.index)
	if err != nil {
		return nil, err
	}
	b := d.bounds
	if float64(r.X) == b.X && float64(r.Y) == b.Y && float64(r.W) == b.W && float64(r.H) == b.H {
		return img, nil
	}
	scale := float64(img.Bounds().Dx()) / b.W
	px := image.Rect(
		int(math.Floor((float64(r.X)-b.X)*scale)),
		int(math.Floor((float64(r.Y)-b.Y)*scale)),
		int(math.Ceil((float64(r.X+r.W)-b.X)*scale)),
		int(math.Ceil((float64(r.Y+r.H)-b.Y)*scale)),
	)
	return Crop(img, px), nil
}

func screencapture(index int) (image.Image, error) {
	dir, err := os.MkdirTemp("", "infishot-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "shot.png")

	var stderr bytes.Buffer
	cmd := exec.Command("/usr/sbin/screencapture",
		"-x",        // no sound
		"-t", "png", // lossless
		"-D", strconv.Itoa(index),
		path)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(strings.ToLower(msg), "permission") || strings.Contains(msg, "not authorized") {
			return nil, ErrPermission
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("capture: screencapture: %s", msg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// screencapture leaves no file when the system refused it.
		return nil, ErrPermission
	}
	// In sRGB: the file is in the display's colors.
	return Decode(data)
}
