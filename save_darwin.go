//go:build darwin

package main

import (
	"errors"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"

	"github.com/Infinity4B/infishot/internal/macos"
)

const (
	appKitPath = "/System/Library/Frameworks/AppKit.framework/AppKit"
	utTypePath = "/System/Library/Frameworks/UniformTypeIdentifiers.framework/UniformTypeIdentifiers"

	nsModalResponseOK = 1
)

// darwinSavePanel presents modeless NSSavePanel instances via
// beginWithCompletionHandler: instead of MyGo's Dialog.Save (which runs
// [NSSavePanel runModal]). An app-modal session blocks mouse and keyboard
// delivery to every window whose worksWhenModal is NO, including the capture
// overlay (MyGoPanel), so triggering a new capture while a save panel is
// open leaves the overlay frozen.
//
// Each show call retains its own NSSavePanel and returns a handle for it, so
// different images can have save panels open simultaneously while repeated ⌘S
// on the same image focuses its existing panel. All methods must be called on
// the main thread.
type darwinSavePanel struct{}

type darwinSaveHandle struct {
	panel objc.ID // retained NSSavePanel while visible; 0 once closed
}

var platformSavePanel savePanel = darwinSavePanel{}

func (darwinSavePanel) show(opts saveOptions, done func(path string, err error)) saveHandle {
	if err := macos.Load(appKitPath); err != nil {
		done("", err)
		return nil
	}
	// UniformTypeIdentifiers exists on macOS 11+; older systems fall back to
	// setAllowedFileTypes: below.
	_, _ = macos.Dlopen(utTypePath)

	var (
		handle   *darwinSaveHandle
		setupErr error
	)
	macos.WithPool(func() {
		panel := macos.Send(macos.Class("NSSavePanel"), "savePanel")
		if panel == 0 {
			setupErr = errors.New("无法创建保存面板")
			return
		}
		if opts.title != "" {
			macos.Send(panel, "setTitle:", macos.String(opts.title))
		}
		if opts.dir != "" {
			if u := macos.Send(macos.Class("NSURL"), "fileURLWithPath:isDirectory:", macos.String(opts.dir), true); u != 0 {
				macos.Send(panel, "setDirectoryURL:", u)
			}
		}
		if opts.name != "" {
			macos.Send(panel, "setNameFieldStringValue:", macos.String(opts.name))
		}
		macos.Send(panel, "setCanCreateDirectories:", true)
		if utClass := macos.Class("UTType"); utClass != 0 {
			if pngType := macos.Send(utClass, "typeWithFilenameExtension:", macos.String("png")); pngType != 0 {
				arr := macos.Send(macos.Class("NSArray"), "arrayWithObject:", pngType)
				macos.Send(panel, "setAllowedContentTypes:", arr)
			}
		} else {
			macos.Send(panel, "setAllowedFileTypes:", macos.Strings([]string{"png"}))
		}

		// Retain the autoreleased panel until its completion handler runs so
		// it stays alive across run-loop turns and can be brought forward on
		// repeated ⌘S for the same image.
		macos.Send(panel, "retain")
		handle = &darwinSaveHandle{panel: panel}

		blk := objc.NewBlock(func(_ objc.Block, resp int) {
			var path string
			macos.WithPool(func() {
				defer macos.Release(panel)
				handle.panel = 0
				if resp == nsModalResponseOK {
					if u := macos.Send(panel, "URL"); u != 0 {
						path = macos.GoString(macos.Send(u, "path"))
					}
				}
			})
			done(path, nil)
		})

		// An accessory app must activate so the modeless panel comes forward
		// and takes keyboard focus. Keep the panel at NSModalPanelWindowLevel
		// (8): above lowered pins (3) and below the capture overlay (1000).
		mygo.App.Focus()
		macos.Send(panel, "setLevel:", nsModalPanelWindowLevel)
		macos.Send(panel, "beginWithCompletionHandler:", blk)
		if handle.panel == panel {
			macos.Send(panel, "setLevel:", nsModalPanelWindowLevel)
		}
		blk.Release()
	})
	if setupErr != nil {
		done("", setupErr)
		return nil
	}
	if handle == nil || handle.panel == 0 {
		return nil
	}
	return handle
}

func (h *darwinSaveHandle) focus() {
	if h == nil || h.panel == 0 {
		return
	}
	macos.WithPool(func() {
		mygo.App.Focus()
		macos.Send(h.panel, "makeKeyAndOrderFront:", objc.ID(0))
	})
}
