package main

import (
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AlanKK/everythingx/internal/shared"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// usePrefs gives a test its own preferences, theme and stored values, and
// puts the shared ones back afterwards.
func usePrefs(tb testing.TB, p appPrefs) {
	tb.Helper()
	store := fyne.CurrentApp().Preferences()
	for _, k := range []string{keyTheme, keyAccent, keyMatchColor, keyTextScale, keyFont, keyDensity, keyISODates, keyResultLimit,
		keyShowHidden, keyDecimalSizes, keyRememberLayout, keyColumnWidths, keySortCol, keySortAsc} {
		store.RemoveValue(k)
	}
	prefs = p
	appTheme = &everythingxTheme{}
	appTheme.setPrefs(p)
	tb.Cleanup(func() {
		prefs = defaultPrefs()
		appTheme = &everythingxTheme{}
		fyne.CurrentApp().Settings().SetTheme(&everythingxTheme{})
		sortCol, sortAsc = -1, true
		if prefsWindow != nil {
			prefsWindow.Close()
		}
	})
}

func TestLoadPrefsDefaults(t *testing.T) {
	usePrefs(t, defaultPrefs())
	got := loadPrefs(fyne.CurrentApp().Preferences())
	if got != defaultPrefs() {
		t.Errorf("loadPrefs on an empty store = %+v, want the defaults %+v", got, defaultPrefs())
	}
	d := defaultPrefs()
	if !d.ShowHidden || d.ResultLimit != 1000 || d.DecimalSizes || d.Theme != themeSystem {
		t.Errorf("defaults %+v must keep today's behaviour: hidden shown, 1000 results, binary sizes, system theme", d)
	}
}

func TestPrefsRoundTrip(t *testing.T) {
	usePrefs(t, defaultPrefs())
	store := fyne.CurrentApp().Preferences()
	want := appPrefs{
		Theme:          themeDark,
		Accent:         theme.ColorPurple,
		MatchColor:     theme.ColorGreen,
		TextScale:      1.15,
		Font:           fontMono,
		Density:        densityComfortable,
		ResultLimit:    5000,
		ShowHidden:     false,
		DecimalSizes:   true,
		ISODates:       true,
		RememberLayout: true,
		ColumnWidths:   [numColumns]float32{300, 500, 80, 200},
		SortCol:        2,
		SortAsc:        false,
	}
	want.save(store)
	if got := loadPrefs(store); got != want {
		t.Errorf("loadPrefs after save = %+v, want %+v", got, want)
	}
}

// A hand-edited or older preferences file can hold values the window doesn't
// offer; each falls back to its default rather than breaking the app.
func TestLoadPrefsRejectsBadValues(t *testing.T) {
	usePrefs(t, defaultPrefs())
	store := fyne.CurrentApp().Preferences()
	store.SetString(keyTheme, "neon")
	store.SetString(keyAccent, "chartreuse")
	store.SetString(keyMatchColor, "plaid")
	store.SetFloat(keyTextScale, 9)
	store.SetString(keyFont, "comic")
	store.SetString(keyDensity, "cozy")
	store.SetInt(keyResultLimit, 1_000_000)
	store.SetInt(keySortCol, 12)
	store.SetFloatList(keyColumnWidths, []float64{-5, 0, 90, 100, 300})

	got := loadPrefs(store)
	want := defaultPrefs()
	want.ColumnWidths = [numColumns]float32{0, 0, 90, 100}
	if got != want {
		t.Errorf("loadPrefs with bad values = %+v, want %+v", got, want)
	}
}

func TestColumnWidthUsesRememberedOnlyWhenOn(t *testing.T) {
	p := defaultPrefs()
	p.ColumnWidths[1] = 321
	if got := p.columnWidth(1); got != 321 {
		t.Errorf("remembered width = %v, want 321", got)
	}
	if got := p.columnWidth(0); got != defaultColumnWidths[0] {
		t.Errorf("unsized column = %v, want the default %v", got, defaultColumnWidths[0])
	}
	p.RememberLayout = false
	if got := p.columnWidth(1); got != defaultColumnWidths[1] {
		t.Errorf("width with layout not remembered = %v, want the default %v", got, defaultColumnWidths[1])
	}
	// Default widths fit the text, or a larger size cuts off sizes.
	p.TextScale = 1.3
	if got, want := p.columnWidth(2), defaultColumnWidths[2]*1.3; got != want {
		t.Errorf("default Size width at text size 1.3 = %v, want %v", got, want)
	}
}

// Growing the text grows Size and Last Modified and takes the room from
// Path, so the table keeps its width and those columns stay on screen.
func TestFitColumnWidths(t *testing.T) {
	widths := [numColumns]float32{400, 600, 100, 200}
	got := fitColumnWidths(widths, 1.5)
	want := [numColumns]float32{400, 450, 150, 300}
	if got != want {
		t.Errorf("fitColumnWidths x1.5 = %v, want %v", got, want)
	}
	var sum, sumBefore float32
	for i := range got {
		sum += got[i]
		sumBefore += widths[i]
	}
	if sum != sumBefore {
		t.Errorf("total width %v, want it kept at %v", sum, sumBefore)
	}
	if back := fitColumnWidths(got, 1/1.5); back != widths {
		t.Errorf("shrinking back = %v, want %v", back, widths)
	}
	if got := fitColumnWidths([numColumns]float32{400, 250, 100, 200}, 2); got[1] != minPathWidth {
		t.Errorf("Path = %v, want no narrower than %v", got[1], float32(minPathWidth))
	}
}

// Changing the text size scales the columns with it.
func TestTextSizeScalesColumns(t *testing.T) {
	usePrefs(t, defaultPrefs())
	table := makeTable()
	defer func() { tableData = nil }()
	applyColumnWidths()
	table.SetColumnWidth(1, 500)

	content := appearanceTab()
	test.NewTempWindow(t, content)
	sel := findWidgetWith(content, func(s *widget.Select) bool { return slices.Contains(s.Options, "Larger") })
	if sel == nil {
		t.Fatal("no text size choice on the Appearance tab")
	}
	sel.SetSelected("Larger")

	before := [numColumns]float32{defaultColumnWidths[0], 500, defaultColumnWidths[2], defaultColumnWidths[3]}
	want := fitColumnWidths(before, 1.3)
	widths := tableColumnWidths(table)
	for col := range numColumns {
		if widths[col] != want[col] {
			t.Errorf("widths after Larger = %v, want %v", widths, want)
			break
		}
	}
	if prefs.TextScale != 1.3 {
		t.Errorf("text scale = %v, want 1.3", prefs.TextScale)
	}
}

func TestThemeForcedVariant(t *testing.T) {
	var th everythingxTheme
	p := defaultPrefs()

	p.Theme = themeDark
	th.setPrefs(p)
	want := theme.DefaultTheme().Color(theme.ColorNameBackground, theme.VariantDark)
	if got := th.Color(theme.ColorNameBackground, theme.VariantLight); got != want {
		t.Errorf("dark theme on a light system: background %v, want %v", got, want)
	}
	if th.Variant() != theme.VariantDark {
		t.Errorf("Variant() = %v, want dark", th.Variant())
	}

	p.Theme = themeLight
	th.setPrefs(p)
	want = theme.DefaultTheme().Color(theme.ColorNameBackground, theme.VariantLight)
	if got := th.Color(theme.ColorNameBackground, theme.VariantDark); got != want {
		t.Errorf("light theme on a dark system: background %v, want %v", got, want)
	}

	p.Theme = themeSystem
	th.setPrefs(p)
	for _, v := range []fyne.ThemeVariant{theme.VariantLight, theme.VariantDark} {
		want := theme.DefaultTheme().Color(theme.ColorNameBackground, v)
		if got := th.Color(theme.ColorNameBackground, v); got != want {
			t.Errorf("system theme, variant %v: background %v, want %v", v, got, want)
		}
	}
}

func TestThemeAccentColor(t *testing.T) {
	var th everythingxTheme
	def := theme.DefaultTheme()
	for _, n := range []fyne.ThemeColorName{theme.ColorNamePrimary, theme.ColorNameSelection, theme.ColorNameFocus} {
		if got, want := th.Color(n, theme.VariantLight), def.Color(n, theme.VariantLight); got != want {
			t.Errorf("default accent: %s = %v, want Fyne's %v", n, got, want)
		}
	}

	p := defaultPrefs()
	p.Accent = theme.ColorRed
	th.setPrefs(p)
	red, _ := accentColor(theme.ColorRed)
	if got := th.Color(theme.ColorNamePrimary, theme.VariantLight); got != red.color {
		t.Errorf("red accent: primary = %v, want %v", got, red.color)
	}
	// Selection and focus follow the accent, so the selected row matches it.
	sel := toNRGBA(th.Color(theme.ColorNameSelection, theme.VariantLight))
	if sel.R != red.color.R || sel.G != red.color.G || sel.B != red.color.B || sel.A == 0xff {
		t.Errorf("red accent: selection = %v, want translucent %v", sel, red.color)
	}
	if got := th.Color(theme.ColorNameForegroundOnPrimary, theme.VariantLight); got != red.onColor {
		t.Errorf("red accent: foreground on primary = %v, want %v", got, red.onColor)
	}
	// Colours the accent doesn't own are untouched.
	if got, want := th.Color(theme.ColorNameError, theme.VariantLight), def.Color(theme.ColorNameError, theme.VariantLight); got != want {
		t.Errorf("red accent: error colour = %v, want Fyne's %v", got, want)
	}
}

func TestThemeMatchColor(t *testing.T) {
	var th everythingxTheme
	want := theme.DefaultTheme().Color(theme.ColorNameWarning, theme.VariantLight)
	if got := th.Color(colorNameMatch, theme.VariantLight); got != want {
		t.Errorf("default match colour = %v, want the warning colour %v", got, want)
	}

	p := defaultPrefs()
	p.MatchColor = theme.ColorGreen
	th.setPrefs(p)
	green, _ := accentColor(theme.ColorGreen)
	if got := th.Color(colorNameMatch, theme.VariantLight); got != green.color {
		t.Errorf("green match colour = %v, want %v", got, green.color)
	}
}

func TestThemeTextScale(t *testing.T) {
	var th everythingxTheme
	base := theme.DefaultTheme().Size(theme.SizeNameText)
	if got := th.Size(theme.SizeNameText); got != base {
		t.Errorf("unscaled text size = %v, want %v", got, base)
	}
	p := defaultPrefs()
	p.TextScale = 1.3
	th.setPrefs(p)
	if got := th.Size(theme.SizeNameText); got != base*1.3 {
		t.Errorf("text size at 1.3 = %v, want %v", got, base*1.3)
	}
	if got := th.Size(theme.SizeNamePadding); got != 4 {
		t.Errorf("padding = %v, want 4 whatever the text size", got)
	}
}

// Column widths are read from an unexported Fyne field. If an upgrade renames
// it, this fails instead of layouts quietly not being remembered.
func TestTableColumnWidthsReadsFyne(t *testing.T) {
	table := widget.NewTable(func() (int, int) { return 0, 2 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(widget.TableCellID, fyne.CanvasObject) {})
	table.SetColumnWidth(0, 123)
	table.SetColumnWidth(1, 45.5)
	got := tableColumnWidths(table)
	if len(got) != 2 || got[0] != 123 || got[1] != 45.5 {
		t.Fatalf("tableColumnWidths = %v, want map[0:123 1:45.5]", got)
	}
}

func TestSaveLayout(t *testing.T) {
	usePrefs(t, defaultPrefs())
	table := makeTable()
	defer func() { tableData = nil }()

	table.SetColumnWidth(1, 444)
	sortCol, sortAsc = 3, false
	saveLayout()
	got := loadPrefs(fyne.CurrentApp().Preferences())
	if got.ColumnWidths[1] != 444 || got.ColumnWidths[0] != defaultColumnWidths[0] || got.SortCol != 3 || got.SortAsc {
		t.Errorf("stored layout = widths %v sort %d/%v, want Path 444 and sort 3 descending", got.ColumnWidths, got.SortCol, got.SortAsc)
	}

	// Off, nothing is stored.
	usePrefs(t, defaultPrefs())
	prefs.RememberLayout = false
	table.SetColumnWidth(1, 555)
	saveLayout()
	if got := loadPrefs(fyne.CurrentApp().Preferences()); got.ColumnWidths[1] != 0 {
		t.Errorf("layout stored with remembering off: widths %v", got.ColumnWidths)
	}
}

// A highlighted name that fits the Name column at one text size can run past
// it at a larger one, and must then fall back to plain truncation rather than
// overflow into the Path column.
func TestApplyAppearanceUpdatesNameBudget(t *testing.T) {
	usePrefs(t, defaultPrefs())
	makeTable()
	defer func() { tableData = nil }()
	applyAppearance()

	// A name whose match ends at about 90% of the column at the default size.
	prefix := ""
	for fyne.MeasureText(prefix+"x", theme.TextSize(), fyne.TextStyle{}).Width < nameHighlightMaxWidth*0.9 {
		prefix += "x"
	}
	parts := []string{prefix, "hit", ""}
	if segs := nameSegments(parts, false); len(segs) != 2 {
		t.Fatalf("name fitting the column: %d segments, want it highlighted", len(segs))
	}

	prefs.TextScale = 1.3
	applyAppearance()
	if segs := nameSegments(parts, false); len(segs) != 1 {
		t.Errorf("name past the column at text size 1.3: %d segments, want one plain segment", len(segs))
	}
}

func TestBuildRowsSizeUnits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f.bin")
	if err := os.WriteFile(file, make([]byte, 1536), 0o644); err != nil {
		t.Fatal(err)
	}
	r := []*shared.SearchResult{{Fullpath: file, ObjectType: shared.ItemIsFile}}
	if got := buildRows(r, "", searchOptions{})[0].Size; got != "1.5K" {
		t.Errorf("binary size = %q, want 1.5K", got)
	}
	if got := buildRows(r, "", searchOptions{decimalSizes: true})[0].Size; got != "1.5kB" {
		t.Errorf("decimal size = %q, want 1.5kB", got)
	}
}

func TestShowPreferencesReusesWindow(t *testing.T) {
	usePrefs(t, defaultPrefs())
	showPreferences()
	first := prefsWindow
	if first == nil {
		t.Fatal("showPreferences opened no window")
	}
	showPreferences()
	if prefsWindow != first {
		t.Error("a second showPreferences opened another window")
	}
	first.Close()
	if prefsWindow != nil {
		t.Error("closing the window didn't forget it")
	}
}

func TestPreferencesThemeChoiceApplies(t *testing.T) {
	usePrefs(t, defaultPrefs())
	content := appearanceTab()
	test.NewTempWindow(t, content)

	radio := findWidget[*widget.RadioGroup](content)
	if radio == nil {
		t.Fatal("no theme choice on the Appearance tab")
	}
	if radio.Selected != "System" {
		t.Errorf("theme choice = %q, want System", radio.Selected)
	}
	radio.SetSelected("Dark")
	if prefs.Theme != themeDark || appTheme.Variant() != theme.VariantDark {
		t.Errorf("after choosing Dark: prefs.Theme %q, variant %v", prefs.Theme, appTheme.Variant())
	}
	if got := loadPrefs(fyne.CurrentApp().Preferences()).Theme; got != themeDark {
		t.Errorf("stored theme = %q, want dark", got)
	}
	if fyne.CurrentApp().Settings().Theme() != appTheme {
		t.Error("the app isn't painting with the changed theme")
	}
}

func TestSwatchPicksAccent(t *testing.T) {
	usePrefs(t, defaultPrefs())
	row := newSwatchRow(defaultAccentName,
		func() string { return prefs.Accent },
		func(name string) { prefs.Accent = name })
	swatches := row.(*fyne.Container).Objects

	var purple, blue *colorSwatch
	for i, c := range accentColors {
		switch c.name {
		case theme.ColorPurple:
			purple = swatches[i].(*colorSwatch)
		case defaultAccentName:
			blue = swatches[i].(*colorSwatch)
		}
	}
	if !blue.selected() || purple.selected() {
		t.Fatal("with no accent chosen, the default swatch should be the selected one")
	}
	test.Tap(purple)
	if prefs.Accent != theme.ColorPurple || !purple.selected() || blue.selected() {
		t.Errorf("after tapping purple: accent %q", prefs.Accent)
	}
	// Picking the default stores "", so Fyne's default stays in charge.
	test.Tap(blue)
	if prefs.Accent != "" {
		t.Errorf("after tapping the default: accent %q, want \"\"", prefs.Accent)
	}
}

func TestRestoreDefaults(t *testing.T) {
	usePrefs(t, defaultPrefs())
	table := makeTable()
	defer func() { tableData = nil }()
	prefs.Theme, prefs.ResultLimit, prefs.ShowHidden = themeDark, 100, false
	table.SetColumnWidth(0, 50)
	sortCol = 2

	restoreDefaults()
	if prefs != defaultPrefs() || sortCol != 0 || !sortAsc {
		t.Errorf("after restore: prefs %+v, sort %d", prefs, sortCol)
	}
	if w := tableColumnWidths(table)[0]; w != defaultColumnWidths[0] {
		t.Errorf("Name column after restore = %v, want %v", w, defaultColumnWidths[0])
	}
	if appTheme.forced {
		t.Error("theme still forced after restore")
	}
}

// findWidget returns the first object of type T in o's tree.
func findWidget[T fyne.CanvasObject](o fyne.CanvasObject) T {
	return findWidgetWith(o, func(T) bool { return true })
}

// findWidgetWith returns the first object of type T in o's tree that match
// accepts.
func findWidgetWith[T fyne.CanvasObject](o fyne.CanvasObject, match func(T) bool) T {
	var zero T
	if v, ok := o.(T); ok && match(v) {
		return v
	}
	var children []fyne.CanvasObject
	switch c := o.(type) {
	case *fyne.Container:
		children = c.Objects
	case *widget.Form:
		for _, item := range c.Items {
			children = append(children, item.Widget)
		}
	case fyne.Widget:
		children = test.WidgetRenderer(c).Objects()
	}
	for _, child := range children {
		if v := findWidgetWith(child, match); any(v) != any(zero) {
			return v
		}
	}
	return zero
}

func toNRGBA(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}
