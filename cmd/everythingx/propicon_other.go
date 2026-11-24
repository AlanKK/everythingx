//go:build !darwin && !linux

package main

import (
	"os"

	"fyne.io/fyne/v2"
)

// propertiesIcon has no system icons to offer on this platform; the dialog
// uses the row's icon.
func propertiesIcon(string, os.FileMode) fyne.Resource { return nil }
