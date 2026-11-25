//go:build !darwin

package main

import (
	"path/filepath"

	"github.com/egoist/mygo"
)

type otherSavePanel struct{}

type otherSaveHandle struct{}

var platformSavePanel savePanel = otherSavePanel{}

func (otherSavePanel) show(opts saveOptions, done func(path string, err error)) saveHandle {
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
	return otherSaveHandle{}
}

func (otherSaveHandle) focus() {
	mygo.App.Focus()
}
