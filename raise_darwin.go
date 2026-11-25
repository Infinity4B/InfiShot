//go:build darwin

package main

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// AppKit window levels (CGWindowLevel.h).
const (
	nsFloatingWindowLevel    = 3
	nsModalPanelWindowLevel  = 8
	nsStatusWindowLevel      = 25
	nsPopUpMenuWindowLevel   = 101
	nsScreenSaverWindowLevel = 1000

	// pinWindowLevel matches NSPopUpMenuWindowLevel (101): above the Dock (20),
	// Notification Center (21), Siri (23), and menu bar (24/25), and below the
	// capture overlay (1000) so a new capture still covers existing pins.
	// Sharing level 101 with NSMenu and Control Center keeps last-ordered-front
	// ordering within the level: a pin created after Control Center stays above
	// it, while a right-click context menu or menu-bar dropdown opened later
	// still appears above the pin.
	pinWindowLevel = nsPopUpMenuWindowLevel
)

// coverScreen puts the capture overlay over the whole display b, above
// the menu bar and the Dock, at the screen saver's level, where an
// always-on-top window (floating level) would stay under them. Call it on
// the main thread.
func coverScreen(w *mygo.Window, b mygo.Rectangle) {
	if h := objc.ID(w.NativeHandle()); h != 0 {
		h.Send(objc.RegisterName("setLevel:"), nsScreenSaverWindowLevel)
		unconstrain(h)
	}
	w.SetBounds(b)
}

// placeFreely lets a window go anywhere on the screen, as a pin over the
// menu bar where it was captured.
func placeFreely(w *mygo.Window) {
	if h := objc.ID(w.NativeHandle()); h != 0 {
		unconstrain(h)
	}
}

// raisePin lifts a pin window to pinWindowLevel (101), above the Dock,
// Notification Center, Siri, and menu bar, while sharing the pop-up menu
// level so context menus still open above it and staying below the capture
// overlay (1000). MyGo maps AlwaysOnTop to NSFloatingWindowLevel (3), which
// stays behind those system surfaces; pins have no parent window and never
// call SetAlwaysOnTop, so MyGo's applyLevel does not reset this level later.
// Call it on the main thread.
func raisePin(w *mygo.Window) {
	if w == nil {
		return
	}
	if h := objc.ID(w.NativeHandle()); h != 0 {
		h.Send(objc.RegisterName("setLevel:"), pinWindowLevel)
	}
}

// lowerPin drops a pin window back to NSFloatingWindowLevel (3), below
// NSModalPanelWindowLevel (8), so modal dialogs such as the save panel
// appear in front of it. Call it on the main thread.
func lowerPin(w *mygo.Window) {
	if w == nil {
		return
	}
	if h := objc.ID(w.NativeHandle()); h != 0 {
		h.Send(objc.RegisterName("setLevel:"), nsFloatingWindowLevel)
	}
}

var unconstrained sync.Map // class → struct{}

// unconstrain has AppKit leave the windows of h's class where they are
// put. It keeps windows with a title bar (MyGo's frameless windows have a
// hidden one) under the menu bar, and on displays with a camera housing
// even at the screen saver's level, through -constrainFrameRect:toScreen:.
// Its replacement is CGRectStandardize, which on arm64 takes the rectangle
// in the registers the method gets it in (the window, the selector and the
// screen go in others) and returns it, with a positive size, as it is.
// Changing the window's style instead would keep it from the keyboard.
// Elsewhere windows stay constrained.
func unconstrain(h objc.ID) {
	if runtime.GOARCH != "arm64" || !loadObjC() {
		return
	}
	cls := objectGetClass(uintptr(h))
	if cls == 0 {
		return
	}
	if _, done := unconstrained.LoadOrStore(cls, struct{}{}); done {
		return
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	standardize, err := purego.Dlsym(cg, "CGRectStandardize")
	if err != nil {
		return
	}
	types := []byte("{CGRect={CGPoint=dd}{CGSize=dd}}@:{CGRect={CGPoint=dd}{CGSize=dd}}@\x00")
	classReplaceMethod(cls, uintptr(objc.RegisterName("constrainFrameRect:toScreen:")), standardize, unsafe.Pointer(&types[0]))
	runtime.KeepAlive(types)
}

var (
	objcOnce           sync.Once
	objectGetClass     func(obj uintptr) uintptr
	classReplaceMethod func(cls, sel, imp uintptr, types unsafe.Pointer) uintptr
)

func loadObjC() bool {
	objcOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&objectGetClass, lib, "object_getClass")
		purego.RegisterLibFunc(&classReplaceMethod, lib, "class_replaceMethod")
	})
	return objectGetClass != nil
}
