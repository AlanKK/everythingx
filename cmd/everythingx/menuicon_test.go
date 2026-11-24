package main

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestPadSVGWidensViewBox(t *testing.T) {
	got := string(padSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24px" height="24px"><path/></svg>`)))
	if !strings.Contains(got, `viewBox="-6 -6 36 36"`) {
		t.Errorf("viewBox not widened: %s", got)
	}
	if strings.Contains(got, "width=") || strings.Contains(got, "height=") {
		t.Errorf("fixed size kept: %s", got)
	}

	got = string(padSVG([]byte(`<svg width="16" height="16"><path/></svg>`)))
	if !strings.Contains(got, `viewBox="-4 -4 24 24"`) {
		t.Errorf("viewBox not made from the size: %s", got)
	}
	if padSVG([]byte(`<svg><path/></svg>`)) != nil {
		t.Error("an SVG with no size should be left alone")
	}
}

func TestPadRasterCentresImage(t *testing.T) {
	var src bytes.Buffer
	png.Encode(&src, image.NewNRGBA(image.Rect(0, 0, 32, 32)))
	out, _, err := image.Decode(bytes.NewReader(padRaster(src.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Bounds().Size(); got != image.Pt(48, 48) {
		t.Errorf("padded size = %v, want 48x48", got)
	}
}

func TestPadMenuIconKeepsThemedResource(t *testing.T) {
	got := padMenuIcon(theme.DocumentIcon())
	if _, ok := got.(*theme.ThemedResource); !ok {
		t.Fatalf("got %T, want a themed resource so it follows the text colour", got)
	}
	if padMenuIcon(theme.DocumentIcon()) != got {
		t.Error("padded icon not cached")
	}
	if padMenuIcon(nil) != nil {
		t.Error("nil icon should stay nil")
	}
}

func TestContextMenuUsesMenuTheme(t *testing.T) {
	old := appsForPath
	defer func() { appsForPath = old }()
	appsForPath = func(string) ([]appChoice, error) { return nil, nil }
	w := test.NewWindow(nil)
	defer w.Close()
	w.Resize(fyne.NewSize(600, 600))

	m := newContextMenu("/tmp/a.txt", w.Canvas())
	if got := m.Menu.Theme().Size(theme.SizeNameInnerPadding); got != menuRowPadding {
		t.Errorf("menu inner padding = %v, want %v", got, menuRowPadding)
	}
	if got := theme.InnerPadding(); got == menuRowPadding {
		t.Errorf("app inner padding changed to the menu's %v", got)
	}
	plain := m.Menu.Theme().Size(theme.SizeNameText) + 2*theme.InnerPadding()
	if h := m.Menu.Items[0].MinSize().Height; h >= plain {
		t.Errorf("menu row is %v tall, want shorter than the app theme's %v", h, plain)
	}
	if got, want := m.Size().Width, m.MinSize().Width+menuTrailingSpace; got != want {
		t.Errorf("menu width = %v, want %v", got, want)
	}
}
