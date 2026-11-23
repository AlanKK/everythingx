package main

import (
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// Font choices on the Appearance tab.
const (
	fontSystem = "system" // the desktop's UI font, as GTK apps draw with
	fontFyne   = "fyne"   // Fyne's bundled sans-serif
	fontMono   = "mono"   // Roboto Mono, the app's original look
)

var fontLabels = []string{"System", "Fyne default", "Monospace"}
var fontChoices = []string{fontSystem, fontFyne, fontMono}

// fontSet is one font family's regular, bold, italic and bold italic faces.
// A face the system has no file for is the regular one.
type fontSet struct {
	regular, bold, italic, boldItalic fyne.Resource
}

func (f *fontSet) face(s fyne.TextStyle) fyne.Resource {
	switch {
	case s.Bold && s.Italic:
		return f.boldItalic
	case s.Bold:
		return f.bold
	case s.Italic:
		return f.italic
	}
	return f.regular
}

// systemFonts is the desktop's UI font, or nil when it couldn't be found (or
// on a platform without a lookup), in which case "System" draws with Fyne's
// own font. Set once by loadSystemFonts before the theme is first applied,
// since the theme's Font runs on every paint and must not do the lookup.
var systemFonts *fontSet

// loadSystemFonts finds the desktop's UI font and loads its faces. A face the
// lookup doesn't find, or finds in a format Fyne can't draw, falls back to the
// regular face; without a regular face systemFonts stays nil.
func loadSystemFonts() {
	paths := systemFontPaths()
	regular := loadFontFile(paths[0])
	if regular == nil {
		return
	}
	set := &fontSet{regular: regular, bold: regular, italic: regular, boldItalic: regular}
	for i, dst := range []*fyne.Resource{&set.bold, &set.italic, &set.boldItalic} {
		if paths[i+1] == paths[0] {
			continue // a variable font, or no such face: Fyne can't pick its weight
		}
		if res := loadFontFile(paths[i+1]); res != nil {
			*dst = res
		}
	}
	systemFonts = set
}

// loadFontFile reads a TrueType or OpenType file. Collections (.ttc) and other
// formats return nil.
func loadFontFile(path string) fyne.Resource {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf":
	default:
		return nil
	}
	res, err := fyne.LoadResourceFromPath(path)
	if err != nil {
		return nil
	}
	return res
}
