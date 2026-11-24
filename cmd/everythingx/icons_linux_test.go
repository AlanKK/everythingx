//go:build linux

package main

import (
	"path/filepath"
	"slices"
	"testing"
)

const testIconTheme = `[Icon Theme]
Name=Test
Inherits=Parent
Directories=16x16/mimetypes,32x32/mimetypes,128x128/mimetypes,scalable/mimetypes,symbolic/mimetypes,16x16/apps

[16x16/mimetypes]
Context=MimeTypes
Size=16
Type=Fixed

[32x32/mimetypes]
Context=MimeTypes
Size=32
Type=Fixed

[128x128/mimetypes]
Context=MimeTypes
Size=128
Type=Fixed

[scalable/mimetypes]
Context=MimeTypes
Size=128
Type=Scalable

[symbolic/mimetypes]
Context=MimeTypes
Size=16
Type=Scalable

[16x16/apps]
Context=Applications
Size=16
Type=Fixed
`

const testParentTheme = `[Icon Theme]
Name=Parent
Directories=places
[places]
Context=Places
Size=48
Type=Fixed
`

const testHicolor = `[Icon Theme]
Name=hicolor
Directories=48x48/mimetypes
[48x48/mimetypes]
Context=MimeTypes
Size=48
Type=Fixed
`

func testIconLookup(t *testing.T) (*iconLookup, string) {
	t.Helper()
	icons := t.TempDir()
	writeFiles(t, icons, map[string]string{
		"Test/index.theme":                              testIconTheme,
		"Test/16x16/mimetypes/text-x-go.png":            "",
		"Test/32x32/mimetypes/text-x-go.png":            "",
		"Test/scalable/mimetypes/text-x-go.svg":         "",
		"Test/16x16/mimetypes/image-png.png":            "",
		"Test/scalable/mimetypes/image-gif.svg":         "",
		"Test/scalable/mimetypes/image-png.svg":         `<svg><mask id="m"/></svg>`,
		"Test/16x16/mimetypes/image-jpeg.png":           "",
		"Test/scalable/mimetypes/image-jpeg.svg":        `<svg><path d="M0 0h1"/></svg>`,
		"Test/scalable/mimetypes/image-tiff.svg":        `<svg><filter id="f"/></svg>`,
		"Test/128x128/mimetypes/image-tiff.svg":         `<svg><path d="M0 0h1"/></svg>`,
		"Test/scalable/mimetypes/image-bmp.svg":         `<svg><use href="#a"/></svg>`,
		"Test/symbolic/mimetypes/audio-x-generic.svg":   "",
		"Test/16x16/apps/text-plain.png":                "",
		"Parent/index.theme":                            testParentTheme,
		"Parent/places/folder.png":                      "",
		"Adwaita/index.theme":                           testParentTheme,
		"Adwaita/places/folder.png":                     "",
		"hicolor/index.theme":                           testHicolor,
		"hicolor/48x48/mimetypes/application-x-foo.png": "",
	})
	return newIconLookup([]string{filepath.Join(t.TempDir(), "missing"), icons}, "Test"), icons
}

func TestIconLookupFind(t *testing.T) {
	l, icons := testIconLookup(t)
	var names []string
	for _, th := range l.themes {
		names = append(names, th.name)
	}
	// The user theme's own parents come before Adwaita.
	if !slices.Equal(names, []string{"Test", "Parent", "Adwaita", "hicolor"}) {
		t.Fatalf("theme chain = %v, want Test, Parent, Adwaita, hicolor", names)
	}
	for _, tc := range []struct {
		desc  string
		names []string
		want  string
	}{
		{"32px icon preferred over 16px and scalable", []string{"text-x-go"}, "Test/32x32/mimetypes/text-x-go.png"},
		{"a smaller PNG preferred over an SVG", []string{"image-jpeg"}, "Test/16x16/mimetypes/image-jpeg.png"},
		{"a smaller PNG preferred over an SVG Fyne cannot draw", []string{"image-png"}, "Test/16x16/mimetypes/image-png.png"},
		{"a drawable SVG preferred over one Fyne cannot draw", []string{"image-tiff"}, "Test/128x128/mimetypes/image-tiff.svg"},
		{"an undrawable SVG when there is nothing else", []string{"image-bmp"}, "Test/scalable/mimetypes/image-bmp.svg"},
		{"SVG when there is no PNG", []string{"image-gif"}, "Test/scalable/mimetypes/image-gif.svg"},
		{"inherited theme", []string{"folder"}, "Parent/places/folder.png"},
		{"hicolor last", []string{"application-x-foo"}, "hicolor/48x48/mimetypes/application-x-foo.png"},
		{"first name the theme has", []string{"text-x-nope", "image-png"}, "Test/16x16/mimetypes/image-png.png"},
		{"symbolic directories skipped", []string{"audio-x-generic"}, ""},
		{"non-file contexts skipped", []string{"text-plain"}, ""},
		{"nothing found", []string{"nope"}, ""},
	} {
		want := tc.want
		if want != "" {
			want = filepath.Join(icons, want)
		}
		if got := l.find(tc.names, 32); got != want {
			t.Errorf("%s: find(%v) = %q, want %q", tc.desc, tc.names, got, want)
		}
	}
}

// A theme earlier in the chain wins even with a less specific name, so the
// icons keep one look.
func TestIconLookupThemeOrder(t *testing.T) {
	l, icons := testIconLookup(t)
	got := l.find([]string{"application-x-foo", "text-x-go"}, 32)
	if want := filepath.Join(icons, "Test/32x32/mimetypes/text-x-go.png"); got != want {
		t.Errorf("find = %q, want %q", got, want)
	}
}

func TestMimeIconNames(t *testing.T) {
	data := t.TempDir()
	writeFiles(t, data, map[string]string{
		"mime/globs2":        "50:text/x-go:*.go\n50:application/x-shellscript:*.sh\n",
		"mime/subclasses":    "application/x-shellscript text/plain\n",
		"mime/generic-icons": "application/x-shellscript:text-x-script\n",
	})
	db := loadMimeDB([]string{data})
	for _, tc := range []struct {
		mime string
		want []string
	}{
		{"text/x-go", []string{"text-x-go", "text-plain", "text-x-generic", "application-octet-stream", "application-x-generic"}},
		{"application/x-shellscript", []string{"application-x-shellscript", "text-x-script", "text-plain", "application-octet-stream", "application-x-generic"}},
		{mimeOctetStream, []string{"application-octet-stream", "application-x-generic"}},
	} {
		if got := mimeIconNames(db, tc.mime); !slices.Equal(got, tc.want) {
			t.Errorf("mimeIconNames(%s) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestIniValue(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"settings.ini": "[Other]\ngtk-icon-theme-name=Wrong\n[Settings]\ngtk-icon-theme-name = \"Papirus\"\n",
	})
	if got := iniValue(filepath.Join(dir, "settings.ini"), "Settings", "gtk-icon-theme-name"); got != "Papirus" {
		t.Errorf("iniValue = %q, want Papirus", got)
	}
}
