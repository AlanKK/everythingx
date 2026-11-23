package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// openPath and revealPath do the platform work behind openFile and revealFile.
// Tests swap them for stubs.
var (
	openPath   = platformOpen
	revealPath = platformReveal
)

// openFile opens the item in its default application, or a folder in the file
// manager.
func openFile(path string) {
	launch(openPath, path)
}

// revealFile shows the item selected in its containing folder.
func revealFile(path string) {
	launch(revealPath, path)
}

// launch runs action on path off the UI thread, since "open" and "xdg-open" can
// take long enough to stall a click, and reports a failure in the status bar.
// The index can lag the disk, so a vanished item is reported as such rather
// than handed to the launcher.
func launch(action func(string) error, path string) {
	if path == "" {
		return
	}
	go func() {
		name := filepath.Base(path)
		var msg string
		if _, err := os.Lstat(path); err != nil {
			msg = "Not found: " + name
		} else if err := action(path); err != nil {
			fmt.Println("Error:", err)
			msg = "Couldn't open " + name
		}
		if msg != "" {
			fyne.Do(func() { flashStatus(msg) })
		}
	}()
}

// fileURI turns a path into a file:// URI, percent-encoding every byte that is
// not unreserved or a slash. Encoding bytes rather than runes keeps names that
// are not valid UTF-8 intact, and the result is always ASCII, which D-Bus
// strings require.
func fileURI(path string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	b.Grow(len("file://") + len(path)*3)
	b.WriteString("file://")
	for i := 0; i < len(path); i++ {
		c := path[i]
		if isUnreserved(c) || c == '/' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '-' || c == '.' || c == '_' || c == '~'
}
