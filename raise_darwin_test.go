//go:build darwin

package main

import (
	"sync"
	"testing"

	"github.com/ebitengine/purego/objc"
)

var (
	testLevelOnce   sync.Once
	testLevelClass  objc.Class
	testLevelErr    error
	testLevelMu     sync.Mutex
	testWindowLevel = map[objc.ID]int{}
)

func initTestLevelClass() (objc.Class, error) {
	testLevelOnce.Do(func() {
		testLevelClass, testLevelErr = objc.RegisterClass(
			"InfiShotTestLevelWindow",
			objc.GetClass("NSObject"),
			nil,
			nil,
			[]objc.MethodDef{
				{
					Cmd: objc.RegisterName("setLevel:"),
					Fn: func(self objc.ID, _ objc.SEL, level int) {
						testLevelMu.Lock()
						testWindowLevel[self] = level
						testLevelMu.Unlock()
					},
				},
				{
					Cmd: objc.RegisterName("level"),
					Fn: func(self objc.ID, _ objc.SEL) int {
						testLevelMu.Lock()
						defer testLevelMu.Unlock()
						return testWindowLevel[self]
					},
				},
			},
		)
	})
	return testLevelClass, testLevelErr
}

func TestPinWindowLevel(t *testing.T) {
	// The existing unit-test harness runs without mygo.App.Run() / AppKit's
	// event loop, so it cannot instantiate a real mygo.Window. Register a
	// lightweight NSObject subclass to exercise the real Objective-C
	// setLevel: dispatch used by raisePin and lowerPin.
	cls, err := initTestLevelClass()
	if err != nil {
		t.Fatalf("RegisterClass: %v", err)
	}

	selSetLevel := objc.RegisterName("setLevel:")
	selLevel := objc.RegisterName("level")
	win := objc.ID(cls).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("init"))
	defer func() {
		testLevelMu.Lock()
		delete(testWindowLevel, win)
		testLevelMu.Unlock()
		win.Send(objc.RegisterName("release"))
	}()

	// Start at MyGo's AlwaysOnTop level (NSFloatingWindowLevel = 3).
	win.Send(selSetLevel, nsFloatingWindowLevel)

	raisePinHandle(win)
	got := objc.Send[int](win, selLevel)
	if got <= nsPopUpMenuWindowLevel || got >= nsScreenSaverWindowLevel {
		t.Fatalf("raised pin level = %d, want > %d and < %d", got, nsPopUpMenuWindowLevel, nsScreenSaverWindowLevel)
	}
	if got != pinWindowLevel {
		t.Fatalf("raised pin level = %d, want %d", got, pinWindowLevel)
	}

	// Lowering during a modal save dialog drops below NSModalPanelWindowLevel (8).
	lowerPinHandle(win)
	if got := objc.Send[int](win, selLevel); got != nsFloatingWindowLevel || got >= nsModalPanelWindowLevel {
		t.Fatalf("lowered pin level = %d, want %d (< %d)", got, nsFloatingWindowLevel, nsModalPanelWindowLevel)
	}

	// Restoring after the dialog returns to pinWindowLevel.
	raisePinHandle(win)
	if got := objc.Send[int](win, selLevel); got != pinWindowLevel {
		t.Fatalf("restored pin level = %d, want %d", got, pinWindowLevel)
	}

	// Nil windows are safe no-ops.
	raisePin(nil)
	lowerPin(nil)
}
