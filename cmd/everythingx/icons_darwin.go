//go:build darwin

package main

/*
#include <stdlib.h>
#include "openwith_darwin.h"
*/
import "C"

import (
	"path/filepath"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2"

	"github.com/AlanKK/everythingx/internal/shared"
)

// nativeFileIcon returns the icon Finder shows for the result's kind of item.
// Icons are rendered once per kind and extension; an app bundle gets its own
// icon, cached by path. It returns nil when none can be rendered.
func nativeFileIcon(r *shared.SearchResult) fyne.Resource {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(r.Fullpath), "."))
	kind := C.EXKindFile
	switch r.ObjectType {
	case shared.ItemIsDir:
		if ext == "app" {
			return appIcon(r.Fullpath)
		}
		kind, ext = C.EXKindFolder, ""
	case shared.ItemIsSymlink:
		kind, ext = C.EXKindSymlink, ""
	case shared.ItemIsFile:
	default:
		// Pipes, sockets and devices: Finder shows them as generic documents.
		ext = ""
	}
	return cachedTypeIcon(typeIconKey(int(kind), ext), func() fyne.Resource {
		cext := C.CString(ext)
		defer C.free(unsafe.Pointer(cext))
		var n C.int
		buf := C.exTypeIconPNG(cext, C.int(kind), appIconPixels, &n)
		if buf == nil {
			return nil
		}
		defer C.free(buf)
		return fyne.NewStaticResource("type-"+typeIconKey(int(kind), ext)+".png", C.GoBytes(buf, n))
	})
}
