//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// propertiesIcon is the icon the Properties dialog shows for the item at
// path, looked up for that one item rather than its kind, as a file manager's
// own Properties does: a .desktop file shows its app's icon, an executable
// with no recognised name the executable icon, and a symlink its target's
// icon. It returns nil when the theme has nothing, and the dialog falls back
// to the row's icon. It reads files, so call it off the UI thread.
func propertiesIcon(path string, mode os.FileMode) fyne.Resource {
	if mode&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(path)
		info, statErr := os.Stat(path)
		if err != nil || statErr != nil {
			return propIcon("inode-symlink")
		}
		path, mode = target, info.Mode()
	}
	switch {
	case mode.IsDir():
		return propIcon("folder", "inode-directory")
	case !mode.IsRegular():
		return propIcon(mimeIconNames(currentMimeDB(), mimeOctetStream)...)
	}
	if strings.EqualFold(filepath.Ext(path), ".desktop") {
		if e, err := loadDesktopEntry(path, filepath.Base(path)); err == nil {
			if res := appIconResource(e.Icon); res != nil {
				return res
			}
		}
	}
	db := currentMimeDB()
	t := mimeOctetStream
	if g := db.globType(filepath.Base(path)); g != "" {
		t = db.canonical(g)
	} else if mode&0o111 != 0 {
		t = "application/x-executable"
	}
	return propIcon(mimeIconNames(db, t)...)
}

// propIcon returns the first of names that the icon theme has, at the
// dialog's size, or nil.
func propIcon(names ...string) fyne.Resource {
	return iconResource(currentIconLookup().find(names, propertiesIconPixels))
}

// appIconResource resolves a .desktop file's Icon key: a file path, or an
// icon name to find among the theme's app icons. Some files give the name
// with an extension, which the spec does not allow but launchers accept.
func appIconResource(icon string) fyne.Resource {
	if icon == "" {
		return nil
	}
	if filepath.IsAbs(icon) {
		switch strings.ToLower(filepath.Ext(icon)) {
		case ".png", ".svg", ".jpg", ".jpeg":
			if fileExists(icon) {
				return iconResource(icon)
			}
		}
		return nil
	}
	for _, ext := range []string{".png", ".svg", ".xpm"} {
		icon = strings.TrimSuffix(icon, ext)
	}
	return iconResource(currentIconLookup().findApp([]string{icon}, propertiesIconPixels))
}
