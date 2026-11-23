package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AlanKK/everythingx/internal/shared"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// rowHeight is the height a results row asks for under the given density.
func rowHeight(t *testing.T, density string) float32 {
	t.Helper()
	p := defaultPrefs()
	p.Density = density
	usePrefs(t, p)
	fyne.CurrentApp().Settings().SetTheme(appTheme)
	table := makeTable()
	defer func() { tableData = nil }()
	return table.CreateCell().MinSize().Height
}

// Compact rows drop most of the space above and below the text, which makes
// them as tight as a GTK list view's (about 25px apart with DejaVu Sans). The
// height itself depends on the font, so compare the two densities.
func TestCompactRowsAreShorter(t *testing.T) {
	compact := rowHeight(t, densityCompact)
	comfortable := rowHeight(t, densityComfortable)
	if comfortable-compact < 8 {
		t.Errorf("compact row %v, comfortable %v: want compact at least 8 shorter", compact, comfortable)
	}
}

func TestThemeFontChoice(t *testing.T) {
	defer func(f *fontSet) { systemFonts = f }(systemFonts)
	sys := &fontSet{
		regular:    fyne.NewStaticResource("sys.ttf", nil),
		bold:       fyne.NewStaticResource("sys-bold.ttf", nil),
		italic:     fyne.NewStaticResource("sys-italic.ttf", nil),
		boldItalic: fyne.NewStaticResource("sys-bolditalic.ttf", nil),
	}
	bold := fyne.TextStyle{Bold: true}
	mono := fyne.TextStyle{Monospace: true}

	tests := []struct {
		font    string
		system  *fontSet
		style   fyne.TextStyle
		want    fyne.Resource
		comment string
	}{
		{fontSystem, sys, fyne.TextStyle{}, sys.regular, "system regular"},
		{fontSystem, sys, bold, sys.bold, "system bold"},
		{fontSystem, nil, fyne.TextStyle{}, theme.DefaultTheme().Font(fyne.TextStyle{}), "system not found: Fyne's font"},
		{fontFyne, sys, bold, theme.DefaultTheme().Font(bold), "Fyne default"},
		{fontMono, sys, bold, resourceRobotoMonoBold, "monospace"},
		{fontSystem, sys, mono, resourceRobotoMonoRegular, "monospace text stays monospace"},
		{fontFyne, sys, mono, resourceRobotoMonoRegular, "monospace text stays monospace"},
	}
	for _, tc := range tests {
		systemFonts = tc.system
		th := &everythingxTheme{}
		p := defaultPrefs()
		p.Font = tc.font
		th.setPrefs(p)
		if got := th.Font(tc.style); got != tc.want {
			t.Errorf("%s: Font(%+v) = %s, want %s", tc.comment, tc.style, got.Name(), tc.want.Name())
		}
	}
}

func TestGTKFontFamily(t *testing.T) {
	for in, want := range map[string]string{
		"'Cantarell 11'\n":   "Cantarell",
		"'Ubuntu Sans 10.5'": "Ubuntu Sans",
		"'Noto Sans Bold 9'": "Noto Sans Bold",
		"'Inter'":            "Inter",
		"''":                 "",
	} {
		if got := gtkFontFamily(in); got != want {
			t.Errorf("gtkFontFamily(%q) = %q, want %q", in, got, want)
		}
	}
}

// Fyne draws TrueType and OpenType files; a collection can't be loaded as one.
func TestLoadFontFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.ttf", "b.OTF", "c.ttc"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("font"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if loadFontFile(filepath.Join(dir, "a.ttf")) == nil || loadFontFile(filepath.Join(dir, "b.OTF")) == nil {
		t.Error(".ttf or .otf not loaded")
	}
	if loadFontFile(filepath.Join(dir, "c.ttc")) != nil {
		t.Error("font collection loaded")
	}
	if loadFontFile(filepath.Join(dir, "missing.ttf")) != nil {
		t.Error("missing file loaded")
	}
}

// On the solid selection every part of the row is drawn in the colour that
// reads on it, the match included; elsewhere the match keeps its own colour.
func TestSelectedRowTextColour(t *testing.T) {
	for _, s := range nameSegments([]string{"", "down", "loads"}, true) {
		if c := s.(*widget.TextSegment).Style.ColorName; c != theme.ColorNameForegroundOnPrimary {
			t.Errorf("selected name segment colour = %q, want foreground on primary", c)
		}
	}
	if c := nameSegments([]string{"", "down", "loads"}, false)[0].(*widget.TextSegment).Style.ColorName; c != colorNameMatch {
		t.Errorf("unselected match colour = %q, want the match colour", c)
	}
	if c := textSegment("/tmp", fyne.TextAlignLeading, false).Style.ColorName; c != "" {
		t.Errorf("unselected text colour = %q, want the theme's foreground", c)
	}
}

// Folders take the accent colour, and every icon on the selected row the
// selection's text colour; the coloured icons are reused, not remade.
func TestRowIconColours(t *testing.T) {
	dir := &shared.SearchResult{Fullpath: "/tmp/d", ObjectType: shared.ItemIsDir}
	file := &shared.SearchResult{Fullpath: "/tmp/f.txt", ObjectType: shared.ItemIsFile}
	if got := rowIcon(file, false); got != iconForRow(file) {
		t.Errorf("unselected file icon = %s, want the theme's", got.Name())
	}
	if got := rowIcon(dir, false).(*theme.ThemedResource).ColorName; got != theme.ColorNamePrimary {
		t.Errorf("folder icon colour = %q, want primary", got)
	}
	if got := rowIcon(file, true).(*theme.ThemedResource).ColorName; got != theme.ColorNameForegroundOnPrimary {
		t.Errorf("selected icon colour = %q, want foreground on primary", got)
	}
	if rowIcon(dir, true) != rowIcon(dir, true) {
		t.Error("coloured icon made anew for each call")
	}
}

func TestBuildRowsISODates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	mod := time.Date(2026, 11, 23, 14, 3, 0, 0, time.Local)
	if err := os.Chtimes(file, mod, mod); err != nil {
		t.Fatal(err)
	}
	r := []*shared.SearchResult{{Fullpath: file, ObjectType: shared.ItemIsFile}}
	if got := buildRows(r, "", searchOptions{isoDates: true})[0].Modified; got != "2026-11-23 14:03" {
		t.Errorf("ISO date = %q, want 2026-11-23 14:03", got)
	}
	if got := buildRows(r, "", searchOptions{})[0].Modified; got != "Nov 23 2026 14:03" {
		t.Errorf("default date = %q, want Nov 23 2026 14:03", got)
	}
}

// The clear button is there only while there is something to clear, and
// clears the search.
func TestSearchClearButton(t *testing.T) {
	e := newSearchEntry()
	clear := e.ActionItem.(*widget.Button)
	if clear.Visible() {
		t.Error("clear button shown with an empty search")
	}
	e.SetText("x")
	e.showClear()
	if !clear.Visible() {
		t.Error("clear button hidden with text in the search")
	}
	clear.OnTapped()
	if e.Text != "" {
		t.Errorf("search after clear = %q, want empty", e.Text)
	}
}
