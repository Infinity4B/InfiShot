//go:build darwin

package capture

import (
	"os"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/Infinity4B/infishot/internal/macos"
)

const coreGraphics = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"

var copyWindowInfo func(option, relativeTo uint32) uintptr

// windows lists the windows on screen, front to back, with
// CGWindowListCopyWindowInfo. Their bounds need no permission; their
// titles would, and are not used.
func windows() ([]Window, error) {
	if err := macos.Load(); err != nil {
		return nil, err
	}
	if copyWindowInfo == nil {
		lib, err := macos.Dlopen(coreGraphics)
		if err != nil {
			return nil, err
		}
		purego.RegisterLibFunc(&copyWindowInfo, lib, "CGWindowListCopyWindowInfo")
	}
	const (
		onScreenOnly           = 1 << 0
		excludeDesktopElements = 1 << 4
	)
	self := os.Getpid()
	var out []Window
	macos.WithPool(func() {
		list := objc.ID(copyWindowInfo(onScreenOnly|excludeDesktopElements, 0))
		if list == 0 {
			return
		}
		defer macos.Release(list) // a Copy function's result is ours
		for i := range macos.Count(list) {
			d := macos.At(list, i)
			if int(macos.Float(macos.Get(d, "kCGWindowOwnerPID"))) == self {
				continue
			}
			if macos.Float(macos.Get(d, "kCGWindowAlpha")) == 0 {
				continue
			}
			b := macos.Get(d, "kCGWindowBounds")
			w := Window{
				X:     int(macos.Float(macos.Get(b, "X"))),
				Y:     int(macos.Float(macos.Get(b, "Y"))),
				W:     int(macos.Float(macos.Get(b, "Width"))),
				H:     int(macos.Float(macos.Get(b, "Height"))),
				Layer: int(macos.Float(macos.Get(d, "kCGWindowLayer"))),
				Owner: macos.GoString(macos.Get(d, "kCGWindowOwnerName")),
			}
			if w.W > 1 && w.H > 1 {
				out = append(out, w)
			}
		}
	})
	return out, nil
}
