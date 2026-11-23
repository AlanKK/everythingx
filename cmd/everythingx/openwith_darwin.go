//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "openwith_darwin.h"
*/
import "C"

import (
	"runtime/cgo"
	"sync"
	"unsafe"

	"fyne.io/fyne/v2"
)

// appIconPixels is the size app icons are rendered at: twice the menu's icon
// size, so they stay sharp on Retina displays.
const appIconPixels = 32

// appIcons caches rendered icons by app path for the life of the process.
var appIcons sync.Map // string -> fyne.Resource

// platformAppsFor asks LaunchServices for the apps that can open the item:
// the same list, and default, as Finder's Open With.
func platformAppsFor(pathname string) ([]appChoice, error) {
	cpath := C.CString(pathname)
	defer C.free(unsafe.Pointer(cpath))

	var n C.int
	raw := C.exAppsForPath(cpath, &n)
	if raw == nil {
		return nil, nil
	}
	defer C.exFreeApps(raw, n)

	apps := make([]appChoice, 0, int(n))
	for _, a := range unsafe.Slice(raw, int(n)) {
		path := C.GoString(a.path)
		if path == "" {
			continue
		}
		apps = append(apps, appChoice{
			Name:    C.GoString(a.name),
			ID:      path,
			Detail:  C.GoString(a.version),
			Icon:    appIcon(path),
			Default: a.isDefault != 0,
		})
	}
	return apps, nil
}

func appIcon(appPath string) fyne.Resource {
	if r, ok := appIcons.Load(appPath); ok {
		return r.(fyne.Resource)
	}
	cpath := C.CString(appPath)
	defer C.free(unsafe.Pointer(cpath))
	var n C.int
	buf := C.exAppIconPNG(cpath, appIconPixels, &n)
	if buf == nil {
		return nil
	}
	defer C.free(buf)
	r := fyne.NewStaticResource(appPath+".png", C.GoBytes(buf, n))
	appIcons.Store(appPath, r)
	return r
}

// platformOpenWith opens the item in the chosen app through LaunchServices.
func platformOpenWith(app appChoice, pathname string) error {
	return runOpen("-a", app.ID, pathname)
}

// platformChooseApp shows an Open panel for picking any app, then opens the
// item in it. Cancelling the panel is not an error.
func platformChooseApp(pathname string) error {
	done := make(chan string, 1)
	h := cgo.NewHandle(done)
	defer h.Delete()
	C.exChooseApp(C.uintptr_t(h))
	app := <-done
	if app == "" {
		return nil
	}
	return runOpen("-a", app, pathname)
}

//export goChooseAppDone
func goChooseAppDone(handle C.uintptr_t, appPath *C.char) {
	done := cgo.Handle(handle).Value().(chan string)
	if appPath == nil {
		done <- ""
		return
	}
	done <- C.GoString(appPath)
}
