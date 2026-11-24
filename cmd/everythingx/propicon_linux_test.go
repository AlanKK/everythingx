//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// App icons are found only by findApp, and file-type icons only by find.
func TestIconLookupFindApp(t *testing.T) {
	l, icons := testIconLookup(t)
	if got, want := l.findApp([]string{"text-plain"}, 32), filepath.Join(icons, "Test/16x16/apps/text-plain.png"); got != want {
		t.Errorf("findApp(text-plain) = %q, want %q", got, want)
	}
	if got := l.findApp([]string{"text-x-go"}, 32); got != "" {
		t.Errorf("findApp(text-x-go) = %q, want none: it is a file-type icon", got)
	}
}

// useIconLookup makes l the lookup the icon functions use for this test.
func useIconLookup(t *testing.T, l *iconLookup) {
	t.Helper()
	iconLookupOnce.Do(func() {})
	old := iconLookupVal
	iconLookupVal = l
	t.Cleanup(func() { iconLookupVal = old })
}

// A .desktop file shows its app's icon, by theme name or by path, and a
// symlink to one shows the same.
func TestPropertiesIconDesktopFile(t *testing.T) {
	l, icons := testIconLookup(t)
	useIconLookup(t, l)
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.png")
	writeFiles(t, dir, map[string]string{
		"custom.png":     "png",
		"named.desktop":  "[Desktop Entry]\nType=Application\nName=Named\nExec=named\nIcon=text-plain\n",
		"suffix.desktop": "[Desktop Entry]\nType=Application\nName=Suffix\nExec=suffix\nIcon=text-plain.png\n",
		"path.desktop":   "[Desktop Entry]\nType=Application\nName=Path\nExec=path\nIcon=" + custom + "\n",
	})
	link := filepath.Join(dir, "link.desktop")
	if err := os.Symlink(filepath.Join(dir, "named.desktop"), link); err != nil {
		t.Fatal(err)
	}
	appIcon := filepath.Join(icons, "Test/16x16/apps/text-plain.png")
	for name, want := range map[string]string{
		"named.desktop":  appIcon,
		"suffix.desktop": appIcon,
		"path.desktop":   custom,
		"link.desktop":   appIcon,
	} {
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		res := propertiesIcon(path, info.Mode())
		if res == nil || res.Name() != want {
			t.Errorf("propertiesIcon(%s) = %v, want %s", name, res, want)
		}
	}
}

// A dangling symlink gets the symlink icon, or nil when the theme has none.
func TestPropertiesIconDanglingLink(t *testing.T) {
	l, _ := testIconLookup(t)
	useIconLookup(t, l)
	link := filepath.Join(t.TempDir(), "gone")
	if err := os.Symlink("/nonexistent/target", link); err != nil {
		t.Fatal(err)
	}
	if res := propertiesIcon(link, os.ModeSymlink); res != nil {
		t.Errorf("propertiesIcon(dangling) = %s, want nil", res.Name())
	}
}

// Drawn large, a drawable SVG beats a PNG under half the size, but a PNG
// still beats an SVG Fyne cannot draw.
func TestIconLookupFindLarge(t *testing.T) {
	l, icons := testIconLookup(t)
	for names, want := range map[string]string{
		"image-jpeg": "Test/scalable/mimetypes/image-jpeg.svg",
		"image-png":  "Test/16x16/mimetypes/image-png.png",
		"text-x-go":  "Test/scalable/mimetypes/text-x-go.svg",
	} {
		if got := l.find([]string{names}, propertiesIconPixels); got != filepath.Join(icons, want) {
			t.Errorf("find(%s, %d) = %q, want %s", names, propertiesIconPixels, got, want)
		}
	}
}
