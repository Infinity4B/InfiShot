//go:build !darwin

package main

import "github.com/egoist/mygo"

func coverScreen(w *mygo.Window, b mygo.Rectangle) { w.SetBounds(b) }

func placeFreely(*mygo.Window) {}
