// Package capture takes pictures of the screen.
package capture

import (
	"errors"
	"image"
	"image/draw"
)

// ErrUnsupported is returned on systems without a screen capture backend.
var ErrUnsupported = errors.New("capture: 当前系统暂不支持截屏")

// ErrPermission means the system kept the screen's content from the app:
// on macOS, the app lacks the Screen Recording permission.
var ErrPermission = errors.New("capture: 没有屏幕录制权限")

// Rect is an area of the screen in points (DIPs), in the coordinates of
// the desktop: the origin is the top-left corner of the primary display.
type Rect struct{ X, Y, W, H int }

// Region captures r at the display's full resolution: a Retina display
// gives an image twice r's size.
func Region(r Rect) (*image.RGBA, error) {
	if r.W <= 0 || r.H <= 0 {
		return nil, errors.New("capture: 区域为空")
	}
	img, err := region(r)
	if err != nil {
		return nil, err
	}
	return toRGBA(img), nil
}

// Window is a window on screen, in desktop points.
type Window struct {
	X, Y, W, H int
	// Layer is 0 for application windows; the menu bar, the Dock and
	// other system surfaces are above.
	Layer int
	Owner string
}

// Windows lists the windows on screen, front to back, leaving out this
// process's own. Windows without a listing backend return none.
func Windows() ([]Window, error) { return windows() }

// Crop copies the part r of img (in img's pixels, clamped to its bounds)
// into an image of its own, so the result keeps no reference to img.
func Crop(img image.Image, r image.Rectangle) *image.RGBA {
	r = r.Intersect(img.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) {
		return rgba
	}
	return Crop(img, img.Bounds())
}
