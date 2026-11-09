//go:build darwin

package capture

import (
	"errors"
	"image"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/Infinity4B/infishot/internal/macos"
)

const (
	coreFoundation = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	imageIO        = "/System/Library/Frameworks/ImageIO.framework/ImageIO"

	alphaPremultipliedLast = 1  // kCGImageAlphaPremultipliedLast: RGBA, as image.RGBA
	blendModeCopy          = 17 // kCGBlendModeCopy
)

var (
	decodeOnce sync.Once
	decodeErr  error

	cfDataCreate      func(alloc uintptr, bytes unsafe.Pointer, length int) uintptr
	cfRelease         func(uintptr)
	sourceCreate      func(data, options uintptr) uintptr
	sourceImageAt     func(src uintptr, index uint, options uintptr) uintptr
	imageWidth        func(img uintptr) uint
	imageHeight       func(img uintptr) uint
	bitmapContext     func(data unsafe.Pointer, w, h, bpc, stride uint, space uintptr, info uint32) uintptr
	contextBlendMode  func(ctx uintptr, mode int32)
	contextDrawImage  func(ctx uintptr, r cgRect, img uintptr)
	colorSpaceForName func(name uintptr) uintptr
	srgb              uintptr // CGColorSpaceRef, kept for the process's life
)

func loadDecode() error {
	decodeOnce.Do(func() {
		if decodeErr = macos.Load(); decodeErr != nil {
			return
		}
		cf, err := macos.Dlopen(coreFoundation)
		if err != nil {
			decodeErr = err
			return
		}
		io, err := macos.Dlopen(imageIO)
		if err != nil {
			decodeErr = err
			return
		}
		cg, err := macos.Dlopen(coreGraphics)
		if err != nil {
			decodeErr = err
			return
		}
		purego.RegisterLibFunc(&cfDataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		purego.RegisterLibFunc(&sourceCreate, io, "CGImageSourceCreateWithData")
		purego.RegisterLibFunc(&sourceImageAt, io, "CGImageSourceCreateImageAtIndex")
		purego.RegisterLibFunc(&imageWidth, cg, "CGImageGetWidth")
		purego.RegisterLibFunc(&imageHeight, cg, "CGImageGetHeight")
		purego.RegisterLibFunc(&bitmapContext, cg, "CGBitmapContextCreate")
		purego.RegisterLibFunc(&contextBlendMode, cg, "CGContextSetBlendMode")
		purego.RegisterLibFunc(&contextDrawImage, cg, "CGContextDrawImage")
		purego.RegisterLibFunc(&colorSpaceForName, cg, "CGColorSpaceCreateWithName")
		name, err := purego.Dlsym(cg, "kCGColorSpaceSRGB")
		if err != nil {
			decodeErr = err
			return
		}
		// name is the address of the CFStringRef constant.
		srgb = colorSpaceForName(**(**uintptr)(unsafe.Pointer(&name)))
		if srgb == 0 {
			decodeErr = errors.New("capture: 无法创建 sRGB 色彩空间")
		}
	})
	return decodeErr
}

// Decode decodes an image file (PNG, JPEG, TIFF, HEIC…) with the system's
// ImageIO and converts its colors to sRGB, the space MyGo shows bitmaps in
// and that images without a profile are taken to be in. Screenshots carry
// the display's profile (Display P3 on recent Macs, others on external
// displays): their pixels read as sRGB would show off-color.
func Decode(data []byte) (*image.RGBA, error) {
	if len(data) == 0 {
		return nil, errors.New("capture: 图片数据为空")
	}
	if err := loadDecode(); err != nil {
		return nil, err
	}
	cfdata := cfDataCreate(0, unsafe.Pointer(&data[0]), len(data))
	runtime.KeepAlive(data)
	if cfdata == 0 {
		return nil, errors.New("capture: CFDataCreate 失败")
	}
	defer cfRelease(cfdata)
	src := sourceCreate(cfdata, 0)
	if src == 0 {
		return nil, errors.New("capture: 无法识别的图片格式")
	}
	defer cfRelease(src)
	cgimg := sourceImageAt(src, 0, 0)
	if cgimg == 0 {
		return nil, errors.New("capture: 无法解码图片")
	}
	defer cfRelease(cgimg)

	w, h := int(imageWidth(cgimg)), int(imageHeight(cgimg))
	if w <= 0 || h <= 0 {
		return nil, errors.New("capture: 图片尺寸为空")
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	var pin runtime.Pinner
	pin.Pin(&out.Pix[0])
	defer pin.Unpin()
	ctx := bitmapContext(unsafe.Pointer(&out.Pix[0]), uint(w), uint(h), 8, uint(out.Stride), srgb, alphaPremultipliedLast)
	if ctx == 0 {
		return nil, errors.New("capture: 无法创建位图上下文")
	}
	defer cfRelease(ctx)
	// Copy, not blend over the transparent start: the same pixels for
	// opaque images, and the image's own alpha for the others.
	contextBlendMode(ctx, blendModeCopy)
	contextDrawImage(ctx, cgRect{W: float64(w), H: float64(h)}, cgimg)
	return out, nil
}
