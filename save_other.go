//go:build !darwin

package main

import (
	"path/filepath"

	"github.com/egoist/mygo"
)

type otherSavePanel struct{}

var platformSavePanel savePanel = &otherSavePanel{}

func (otherSavePanel) show(opts saveOptions, done func(path string, err error)) {
	go func() {
		mygo.App.Focus()
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:             opts.title,
			DefaultPath:       filepath.Join(opts.dir, opts.name),
			Filters:           []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}},
			CreateDirectories: true,
		})
		mygo.RunOnMain(func() { done(path, err) })
	}()
}

func (otherSavePanel) focus() {
	mygo.App.Focus()
}
