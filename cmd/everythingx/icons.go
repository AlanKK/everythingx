package main

import (
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
)

// typeIcons caches native file-type icons by typeIconKey for the life of the
// process. A lookup that found nothing is cached too, as a nil resource, so
// the fallback costs nothing on the next row of that type.
var typeIcons sync.Map // string -> fyne.Resource (possibly nil)

// typeIconKey names a kind of item: its object kind plus, for files, the
// lowercase extension.
func typeIconKey(kind int, ext string) string {
	return strconv.Itoa(kind) + ":" + ext
}

// cachedTypeIcon returns the icon cached under key, making it with load the
// first time.
func cachedTypeIcon(key string, load func() fyne.Resource) fyne.Resource {
	if r, ok := typeIcons.Load(key); ok {
		res, _ := r.(fyne.Resource)
		return res
	}
	res := load()
	typeIcons.Store(key, res)
	return res
}
