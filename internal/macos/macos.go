//go:build darwin

// Package macos holds the few Objective-C helpers InfiShot needs to call
// system frameworks through purego, without cgo.
package macos

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	loadOnce sync.Once
	loadErr  error
	poolPush func() uintptr
	poolPop  func(uintptr)

	selCache sync.Map // string → objc.SEL
)

// Load opens frameworks by path, once each, with the Objective-C runtime.
func Load(frameworks ...string) error {
	loadOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		purego.RegisterLibFunc(&poolPush, lib, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&poolPop, lib, "objc_autoreleasePoolPop")
		_, loadErr = purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	})
	if loadErr != nil {
		return loadErr
	}
	for _, fw := range frameworks {
		if _, err := Dlopen(fw); err != nil {
			return err
		}
	}
	return nil
}

var (
	libsMu sync.Mutex
	libs   = map[string]uintptr{}
)

// Dlopen opens a library once and returns its handle.
func Dlopen(path string) (uintptr, error) {
	libsMu.Lock()
	defer libsMu.Unlock()
	if h, ok := libs[path]; ok {
		return h, nil
	}
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("无法加载 %s: %w", path, err)
	}
	libs[path] = h
	return h, nil
}

// WithPool runs fn on a locked OS thread inside an autorelease pool, which
// takes the objects the frameworks autorelease.
func WithPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := poolPush()
	defer poolPop(p)
	fn()
}

// Sel returns a cached selector.
func Sel(name string) objc.SEL {
	if s, ok := selCache.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	selCache.Store(name, s)
	return s
}

// Class returns a class by name as an object to send messages to.
func Class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

// Send sends a message that returns an object.
func Send(id objc.ID, sel string, args ...any) objc.ID { return id.Send(Sel(sel), args...) }

// String makes an autoreleased NSString.
func String(s string) objc.ID {
	b := append([]byte(s), 0)
	id := Send(Class("NSString"), "stringWithUTF8String:", unsafe.Pointer(&b[0]))
	runtime.KeepAlive(b)
	return id
}

// Strings makes an autoreleased NSArray of NSStrings.
func Strings(ss []string) objc.ID {
	if len(ss) == 0 {
		return Send(Class("NSArray"), "array")
	}
	ids := make([]objc.ID, len(ss))
	for i, s := range ss {
		ids[i] = String(s)
	}
	arr := Send(Class("NSArray"), "arrayWithObjects:count:", unsafe.Pointer(&ids[0]), uint(len(ids)))
	runtime.KeepAlive(ids)
	return arr
}

// GoString copies an NSString.
func GoString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	return objc.Send[string](s, Sel("UTF8String"))
}

// Count is an NSArray's count.
func Count(arr objc.ID) int {
	if arr == 0 {
		return 0
	}
	return int(objc.Send[uint](arr, Sel("count")))
}

// At is an NSArray's object at i.
func At(arr objc.ID, i int) objc.ID { return Send(arr, "objectAtIndex:", uint(i)) }

// Get is an NSDictionary's object for a string key.
func Get(dict objc.ID, key string) objc.ID {
	if dict == 0 {
		return 0
	}
	return Send(dict, "objectForKey:", String(key))
}

// Float is an NSNumber's double value, 0 for nil.
func Float(num objc.ID) float64 {
	if num == 0 {
		return 0
	}
	return objc.Send[float64](num, Sel("doubleValue"))
}

// Release releases an object the caller owns.
func Release(id objc.ID) {
	if id != 0 {
		Send(id, "release")
	}
}

// ErrorString describes an NSError.
func ErrorString(err objc.ID) string {
	if err == 0 {
		return ""
	}
	return GoString(Send(err, "localizedDescription"))
}
