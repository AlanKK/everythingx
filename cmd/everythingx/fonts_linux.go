package main

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// fontLookupTimeout bounds the whole lookup, so a missing or slow session bus
// (WSL, a bare X session) can't hold up the window.
const fontLookupTimeout = time.Second

// systemFontPaths finds the files of the desktop's UI font: regular, bold,
// italic and bold italic, "" for any not found. The family is GNOME's
// interface font, as GTK apps use it, else fontconfig's sans-serif. Either is
// resolved through fontconfig, which substitutes a family that isn't installed
// just as GTK does, so the result matches what GTK apps draw with.
func systemFontPaths() [4]string {
	ctx, cancel := context.WithTimeout(context.Background(), fontLookupTimeout)
	defer cancel()

	family := "sans-serif"
	if out, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "font-name").Output(); err == nil {
		if f := gtkFontFamily(string(out)); f != "" {
			family = f
		}
	}

	var paths [4]string
	var wg sync.WaitGroup
	for i, style := range []string{"", ":weight=bold", ":slant=italic", ":weight=bold:slant=italic"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := exec.CommandContext(ctx, "fc-match", "-f", "%{file}", family+style).Output()
			if err == nil {
				paths[i] = strings.TrimSpace(string(out))
			}
		}()
	}
	wg.Wait()
	return paths
}

// gtkFontFamily takes the family from a GTK font description as gsettings
// prints it, such as "'Cantarell 11'" or "'Noto Sans Bold 10'". Fontconfig
// names a style with ":" rather than after a space, so a style word in the
// description is left on the family; fc-match then still finds the family.
func gtkFontFamily(setting string) string {
	desc := strings.Trim(strings.TrimSpace(setting), "'\"")
	fields := strings.Fields(desc)
	if n := len(fields); n > 1 && strings.Trim(fields[n-1], "0123456789.") == "" {
		fields = fields[:n-1] // the point size
	}
	return strings.Join(fields, " ")
}
