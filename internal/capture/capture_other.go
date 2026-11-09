//go:build !darwin

package capture

import "image"

func region(Rect) (image.Image, error) { return nil, ErrUnsupported }

func windows() ([]Window, error) { return nil, nil }
