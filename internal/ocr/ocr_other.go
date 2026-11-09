//go:build !darwin

package ocr

func recognize([]byte, []string) (Result, error) { return Result{}, ErrUnsupported }
