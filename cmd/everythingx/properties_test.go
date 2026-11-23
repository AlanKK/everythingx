package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/AlanKK/everythingx/internal/shared"
)

// propValue finds a row's value by its label, across all sections.
func propValue(p fileProperties, label string) (string, bool) {
	for _, s := range p.sections {
		for _, r := range s.rows {
			if r.label == label {
				return r.value, true
			}
		}
	}
	return "", false
}

func TestReadPropertiesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, make([]byte, 1536), 0o640); err != nil {
		t.Fatal(err)
	}
	p, err := readProperties(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.name != "report.pdf" || p.objectType != shared.ItemIsFile {
		t.Errorf("name, type = %q, %v", p.name, p.objectType)
	}
	want := map[string]string{
		"Type":     "PDF file",
		"Location": dir,
		"Size":     "1.5K (1,536 bytes)",
		"Access":   "-rw-r-----  (0640)",
	}
	for label, w := range want {
		if got, _ := propValue(p, label); got != w {
			t.Errorf("%s = %q, want %q", label, got, w)
		}
	}
	for _, label := range []string{"Modified", "Owner", "Group"} {
		if got, ok := propValue(p, label); !ok || got == "" {
			t.Errorf("%s missing", label)
		}
	}
	if _, ok := propValue(p, "Contents"); ok {
		t.Error("a file has a Contents row")
	}
}

func TestReadPropertiesFolderAndLink(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := readProperties(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := propValue(p, "Contents"); got != "2 items" {
		t.Errorf("Contents = %q, want 2 items", got)
	}
	if got, _ := propValue(p, "Type"); got != "Folder" || p.objectType != shared.ItemIsDir {
		t.Errorf("Type = %q (%v), want Folder", got, p.objectType)
	}
	if _, ok := propValue(p, "Size"); ok {
		t.Error("a folder has a Size row")
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "a"), link); err != nil {
		t.Fatal(err)
	}
	p, err = readProperties(link, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := propValue(p, "Link Target"); got != filepath.Join(dir, "a") {
		t.Errorf("Link Target = %q", got)
	}
	if p.objectType != shared.ItemIsSymlink {
		t.Errorf("type = %v, want symlink", p.objectType)
	}
}

func TestReadPropertiesMissing(t *testing.T) {
	if _, err := readProperties(filepath.Join(t.TempDir(), "gone"), false); err == nil {
		t.Error("no error for a missing file")
	}
}

func TestSizeDescription(t *testing.T) {
	cases := []struct {
		size    int64
		decimal bool
		want    string
	}{
		{0, false, "0 bytes"},
		{1, false, "1 byte"},
		{999, true, "999 bytes"},
		{1000, true, "1.0kB (1,000 bytes)"},
		{1000, false, "1,000 bytes"},
		{1234567, false, "1.2M (1,234,567 bytes)"},
	}
	for _, c := range cases {
		if got := sizeDescription(c.size, c.decimal); got != c.want {
			t.Errorf("sizeDescription(%d, %v) = %q, want %q", c.size, c.decimal, got, c.want)
		}
	}
}

func TestTypeDescription(t *testing.T) {
	if got := typeDescription("/x/Makefile", 0); got != "File" {
		t.Errorf("no extension: %q", got)
	}
	if got := typeDescription("/x/a.tar.gz", 0); got != "GZ file" {
		t.Errorf("a.tar.gz: %q", got)
	}
}

// The dialog content builds without a window and shows every row.
func TestPropertiesContent(t *testing.T) {
	test.NewTempApp(t)
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := readProperties(path, false)
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(propertiesContent(path, p))
	defer w.Close()
	var text strings.Builder
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch o := o.(type) {
		case *widget.Label:
			text.WriteString(o.Text + "\n")
		case *widget.RichText:
			text.WriteString(o.String() + "\n")
		case *fyne.Container:
			for _, c := range o.Objects {
				walk(c)
			}
		}
	}
	walk(w.Content())
	for _, want := range []string{"notes.txt", "General", "Permissions", "TXT file", "2 bytes", "Owner"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("dialog lacks %q; has:\n%s", want, text.String())
		}
	}
}
