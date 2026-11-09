package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// config is what the user can change, in config.json in the app's data
// folder (on macOS ~/Library/Application Support/InfiShot). The tray
// menu opens it; changes apply at the next launch.
type config struct {
	// Global shortcuts, in MyGo's accelerator syntax.
	CaptureShortcut string `json:"captureShortcut"`
	OCRShortcut     string `json:"ocrShortcut"`
	PinShortcut     string `json:"pinClipboardShortcut"`
	// SaveDir is where the save dialog opens; empty for ~/Pictures/InfiShot.
	SaveDir string `json:"saveDir"`
	// OCRLanguages are BCP 47 codes, in order of preference.
	OCRLanguages []string `json:"ocrLanguages"`
	// OCRAutoCopy copies recognized text to the clipboard at once.
	OCRAutoCopy bool `json:"ocrAutoCopy"`
	// PinSelectText recognizes the text in pins, for the pointer to
	// select it there.
	PinSelectText bool `json:"pinSelectText"`
	// Snap makes the edges of the selection snap to the edges of windows
	// and of what is in them while it is dragged.
	Snap bool `json:"snap"`
}

func defaultConfig() config {
	return config{
		CaptureShortcut: "Cmd+Ctrl+A",
		OCRShortcut:     "Ctrl+Shift+O",
		PinShortcut:     "Ctrl+Shift+V",
		OCRLanguages:    []string{"zh-Hans", "zh-Hant", "en-US"},
		PinSelectText:   true,
		Snap:            true,
	}
}

func configPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// loadConfig reads the configuration, writing the defaults the first time
// so the user has a file to edit. Fields missing from the file keep their
// defaults.
func loadConfig() (config, string, error) {
	cfg := defaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, path, saveConfig(path, cfg)
	}
	if err != nil {
		return cfg, path, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), path, err
	}
	return cfg, path, nil
}

func saveConfig(path string, cfg config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (c config) saveDir() string {
	if c.SaveDir != "" {
		return c.SaveDir
	}
	if dir, err := mygo.App.Path(mygo.PathPictures); err == nil {
		return filepath.Join(dir, "InfiShot")
	}
	home, _ := os.UserHomeDir()
	return home
}
