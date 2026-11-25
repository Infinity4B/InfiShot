package main

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeSaveHandle struct {
	opts       saveOptions
	done       func(path string, err error)
	focusCalls int
}

func (h *fakeSaveHandle) focus() { h.focusCalls++ }

type fakeSavePanel struct {
	handles []*fakeSaveHandle
	syncErr error
}

func (f *fakeSavePanel) show(opts saveOptions, done func(path string, err error)) saveHandle {
	if f.syncErr != nil {
		done("", f.syncErr)
		return nil
	}
	h := &fakeSaveHandle{opts: opts, done: done}
	f.handles = append(f.handles, h)
	return h
}

func TestEnsurePNGExt(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"shot":          "shot.png",
		"shot.png":      "shot.png",
		"shot.PNG":      "shot.PNG",
		"/tmp/a.b/shot": "/tmp/a.b/shot.png",
		"/tmp/shot.jpg": "/tmp/shot.jpg.png",
	}
	for in, want := range cases {
		if got := ensurePNGExt(in); got != want {
			t.Errorf("ensurePNGExt(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSavePerImagePanels(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeSavePanel{}
	a := &app{
		cfg:       config{SaveDir: dir},
		savePanel: fake,
	}
	imgA := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imgB := image.NewRGBA(image.Rect(0, 0, 3, 3))

	// First save opens a panel for imgA.
	a.saveOnMain(imgA)
	if !a.saveOpen() {
		t.Fatal("a.saveOpen() = false, want true while panel A is open")
	}
	if len(fake.handles) != 1 {
		t.Fatalf("show count = %d, want 1", len(fake.handles))
	}
	hA := fake.handles[0]
	if hA.opts.title != "保存截图" || hA.opts.dir != dir {
		t.Fatalf("opts = %+v, want title=保存截图 dir=%q", hA.opts, dir)
	}
	if !strings.HasPrefix(hA.opts.name, "InfiShot_") || !strings.HasSuffix(hA.opts.name, ".png") {
		t.Fatalf("opts.name = %q, want InfiShot_*.png", hA.opts.name)
	}

	// Repeating save for the same image focuses its panel instead of opening another.
	a.saveOnMain(imgA)
	if len(fake.handles) != 1 || hA.focusCalls != 1 {
		t.Fatalf("same image: shows=%d focusA=%d, want 1, 1", len(fake.handles), hA.focusCalls)
	}

	// Saving a different image while panel A is open opens a second panel.
	a.saveOnMain(imgB)
	if len(fake.handles) != 2 {
		t.Fatalf("different image: shows=%d, want 2", len(fake.handles))
	}
	hB := fake.handles[1]
	if hA.focusCalls != 1 || hB.focusCalls != 0 {
		t.Fatalf("focuses after B opens: A=%d B=%d, want 1, 0", hA.focusCalls, hB.focusCalls)
	}

	// Repeating save for imgB focuses panel B, not panel A.
	a.saveOnMain(imgB)
	if len(fake.handles) != 2 || hA.focusCalls != 1 || hB.focusCalls != 1 {
		t.Fatalf("repeat B: shows=%d focusA=%d focusB=%d, want 2, 1, 1", len(fake.handles), hA.focusCalls, hB.focusCalls)
	}

	// Closing panel A keeps saveOpen true (pins stay lowered) while panel B is open.
	hA.done("", nil)
	if !a.saveOpen() {
		t.Fatal("a.saveOpen() = false with panel B still open, want true")
	}

	// Closing panel B clears saveOpen (pins can be raised again).
	hB.done("", nil)
	if a.saveOpen() {
		t.Fatal("a.saveOpen() = true after all panels closed, want false")
	}

	// Once closed, imgA can open a fresh panel.
	a.saveOnMain(imgA)
	if len(fake.handles) != 3 || !a.saveOpen() {
		t.Fatalf("reopen A: shows=%d saveOpen=%v, want 3, true", len(fake.handles), a.saveOpen())
	}
	fake.handles[2].done("", nil)
	if a.saveOpen() {
		t.Fatal("a.saveOpen() = true after reopened panel closed")
	}
}

func TestSavePanelSyncErrorResetsState(t *testing.T) {
	fake := &fakeSavePanel{syncErr: errors.New("panel failed")}
	a := &app{
		cfg:       config{SaveDir: t.TempDir()},
		savePanel: fake,
	}
	imgA := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imgB := image.NewRGBA(image.Rect(0, 0, 1, 1))

	a.saveOnMain(imgA)
	if a.saveOpen() {
		t.Fatal("a.saveOpen() = true after synchronous error, want false")
	}

	// A sync error on a second image must not clear an already-open first panel.
	fake.syncErr = nil
	a.saveOnMain(imgA)
	if !a.saveOpen() {
		t.Fatal("a.saveOpen() = false after opening panel A")
	}
	fake.syncErr = errors.New("panel B failed")
	a.saveOnMain(imgB)
	if !a.saveOpen() {
		t.Fatal("a.saveOpen() = false while panel A is still open")
	}
	fake.handles[0].done("", nil)
	if a.saveOpen() {
		t.Fatal("a.saveOpen() = true after panel A closed")
	}
}

func TestWriteSavedPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Pix[0], img.Pix[1], img.Pix[2], img.Pix[3] = 12, 34, 56, 255

	path := ensurePNGExt(filepath.Join(t.TempDir(), "capture"))
	if err := writeSavedPNG(path, img); err != nil {
		t.Fatalf("writeSavedPNG: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if !bytes.Contains(data, []byte("sRGB")) {
		t.Fatal("saved PNG missing sRGB chunk")
	}
	back, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	r, g, b, _ := back.At(0, 0).RGBA()
	if r>>8 != 12 || g>>8 != 34 || b>>8 != 56 {
		t.Fatalf("decoded pixel = (%d,%d,%d), want (12,34,56)", r>>8, g>>8, b>>8)
	}
}
