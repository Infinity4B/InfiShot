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

type fakeSavePanel struct {
	showCalls  int
	focusCalls int
	lastOpts   saveOptions
	pending    func(path string, err error)
	syncErr    error
}

func (f *fakeSavePanel) show(opts saveOptions, done func(path string, err error)) {
	f.showCalls++
	f.lastOpts = opts
	if f.syncErr != nil {
		done("", f.syncErr)
		return
	}
	f.pending = done
}

func (f *fakeSavePanel) focus() {
	f.focusCalls++
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

func TestSaveSinglePanelGuard(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeSavePanel{}
	a := &app{
		cfg:       config{SaveDir: dir},
		savePanel: fake,
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	a.saveOnMain(img)
	if !a.saving {
		t.Fatal("a.saving = false, want true while panel is open")
	}
	if fake.showCalls != 1 || fake.focusCalls != 0 {
		t.Fatalf("after first save: show=%d focus=%d, want 1, 0", fake.showCalls, fake.focusCalls)
	}
	if fake.lastOpts.title != "保存截图" || fake.lastOpts.dir != dir {
		t.Fatalf("opts = %+v, want title=保存截图 dir=%q", fake.lastOpts, dir)
	}
	if !strings.HasPrefix(fake.lastOpts.name, "InfiShot_") || !strings.HasSuffix(fake.lastOpts.name, ".png") {
		t.Fatalf("opts.name = %q, want InfiShot_*.png", fake.lastOpts.name)
	}

	// Second request while open must focus the existing panel, not open another.
	a.saveOnMain(img)
	if fake.showCalls != 1 || fake.focusCalls != 1 {
		t.Fatalf("while open: show=%d focus=%d, want 1, 1", fake.showCalls, fake.focusCalls)
	}
	if !a.saving {
		t.Fatal("a.saving = false while panel is still open")
	}

	// Dismissing the panel clears a.saving so a subsequent save opens a new one.
	fake.pending("", nil)
	if a.saving {
		t.Fatal("a.saving = true after panel closed")
	}

	a.saveOnMain(img)
	if fake.showCalls != 2 || !a.saving {
		t.Fatalf("after reopen: show=%d saving=%v, want 2, true", fake.showCalls, a.saving)
	}
	fake.pending("", nil)
}

func TestSavePanelSyncErrorResetsState(t *testing.T) {
	fake := &fakeSavePanel{syncErr: errors.New("panel failed")}
	a := &app{
		cfg:       config{SaveDir: t.TempDir()},
		savePanel: fake,
	}
	a.saveOnMain(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if a.saving {
		t.Fatal("a.saving = true after synchronous error, want false")
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
