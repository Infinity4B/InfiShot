//go:build !darwin

package capture

import (
	"bytes"
	"image"
	_ "image/gif"  // Decode
	_ "image/jpeg" // Decode
	_ "image/png"  // Decode
)

// Decode decodes a PNG, JPEG or GIF image. Unlike on macOS, it leaves
// colors as they are, without color management.
func Decode(data []byte) (*image.RGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return toRGBA(img), nil
}
