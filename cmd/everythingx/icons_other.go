//go:build !darwin && !linux

package main

import (
	"fyne.io/fyne/v2"

	"github.com/AlanKK/everythingx/internal/shared"
)

// nativeFileIcon has no system icons to offer on this platform; the results
// use the theme's icons.
func nativeFileIcon(*shared.SearchResult) fyne.Resource { return nil }
