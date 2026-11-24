//go:build darwin

package main

/*
#include <stdlib.h>
#include "openwith_darwin.h"
*/
import "C"

import (
	"os"
	"unsafe"

	"fyne.io/fyne/v2"
)

// propertiesIcon is the icon Finder shows for this one item, as in its Get
// Info window: an app's own icon, a document's icon from the app that owns
// it, or a custom icon. It returns nil when none can be rendered, and the
// dialog falls back to the row's icon.
func propertiesIcon(path string, _ os.FileMode) fyne.Resource {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var n C.int
	buf := C.exAppIconPNG(cpath, propertiesIconPixels, &n)
	if buf == nil {
		return nil
	}
	defer C.free(buf)
	// Named by path: Fyne caches drawn images by resource name.
	return fyne.NewStaticResource("properties:"+path+".png", C.GoBytes(buf, n))
}
