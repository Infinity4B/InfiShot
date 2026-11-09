//go:build darwin

package ocr

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/Infinity4B/infishot/internal/macos"
)

// Vision is called through the Objective-C runtime with purego, so the app
// still builds with CGO_ENABLED=0.

type cgRect struct{ X, Y, W, H float64 }

const visionPath = "/System/Library/Frameworks/Vision.framework/Vision"

func recognize(image []byte, languages []string) (res Result, err error) {
	if err := macos.Load(visionPath); err != nil {
		return Result{}, fmt.Errorf("ocr: %w", err)
	}
	if macos.Class("VNRecognizeTextRequest") == 0 {
		return Result{}, ErrUnsupported
	}
	macos.WithPool(func() { res, err = perform(image, languages) })
	return res, err
}

func perform(image []byte, languages []string) (Result, error) {
	data := macos.Send(macos.Class("NSData"), "dataWithBytes:length:", unsafe.Pointer(&image[0]), uint(len(image)))
	runtime.KeepAlive(image)
	options := macos.Send(macos.Class("NSDictionary"), "dictionary")

	handler := macos.Send(macos.Send(macos.Class("VNImageRequestHandler"), "alloc"), "initWithData:options:", data, options)
	if handler == 0 {
		return Result{}, errors.New("ocr: 无法读取图片")
	}
	defer macos.Release(handler)

	req := macos.Send(macos.Send(macos.Class("VNRecognizeTextRequest"), "alloc"), "init")
	defer macos.Release(req)
	macos.Send(req, "setRecognitionLevel:", 0) // VNRequestTextRecognitionLevelAccurate
	macos.Send(req, "setUsesLanguageCorrection:", true)
	if langs := supported(req, languages); len(langs) > 0 {
		macos.Send(req, "setRecognitionLanguages:", macos.Strings(langs))
	} else if responds(req, "setAutomaticallyDetectsLanguage:") {
		macos.Send(req, "setAutomaticallyDetectsLanguage:", true)
	}

	nsErr := new(objc.ID)
	requests := macos.Send(macos.Class("NSArray"), "arrayWithObject:", req)
	if !objc.Send[bool](handler, macos.Sel("performRequests:error:"), requests, unsafe.Pointer(nsErr)) {
		msg := macos.ErrorString(*nsErr)
		if msg == "" {
			msg = "识别失败"
		}
		return Result{}, fmt.Errorf("ocr: %s", msg)
	}

	var res Result
	results := macos.Send(req, "results")
	for i := range macos.Count(results) {
		obs := macos.At(results, i)
		cands := macos.Send(obs, "topCandidates:", uint(1))
		if macos.Count(cands) == 0 {
			continue
		}
		cand := macos.At(cands, 0)
		text := macos.GoString(macos.Send(cand, "string"))
		if text == "" {
			continue
		}
		box := flip(objc.Send[cgRect](obs, macos.Sel("boundingBox")))
		res.Lines = append(res.Lines, Line{
			Text:       text,
			Confidence: objc.Send[float32](cand, macos.Sel("confidence")),
			Box:        box,
		})
	}
	return res, nil
}

// supported keeps the languages the request's revision recognizes, in the
// order asked, so that an older system skips those it lacks rather than
// failing.
func supported(req objc.ID, want []string) []string {
	if !responds(req, "supportedRecognitionLanguagesAndReturnError:") {
		return want
	}
	list := macos.Send(req, "supportedRecognitionLanguagesAndReturnError:", unsafe.Pointer(new(objc.ID)))
	if list == 0 {
		return want
	}
	have := map[string]bool{}
	for i := range macos.Count(list) {
		have[macos.GoString(macos.At(list, i))] = true
	}
	var out []string
	for _, l := range want {
		if have[l] {
			out = append(out, l)
		}
	}
	return out
}

func responds(id objc.ID, sel string) bool {
	return objc.Send[bool](id, macos.Sel("respondsToSelector:"), macos.Sel(sel))
}

// flip turns a rectangle of Vision's, whose origin is the bottom-left
// corner, into one from the top-left corner.
func flip(r cgRect) Rect { return Rect{X: r.X, Y: 1 - r.Y - r.H, W: r.W, H: r.H} }
