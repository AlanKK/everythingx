package main

import (
	"slices"

	"fyne.io/fyne/v2"
)

// Theme modes. themeSystem follows the desktop's light or dark setting.
const (
	themeSystem = "system"
	themeLight  = "light"
	themeDark   = "dark"
)

// resultLimits are the choices for the maximum number of results. Every
// result is stat'ed to fill in its size and date, so the top end is kept to
// what that can do while typing.
var resultLimits = []int{100, 500, 1000, 5000, 10000}

// textScale is a text size choice, as a multiple of Fyne's text size.
type textScale struct {
	label string
	scale float64
}

var textScales = []textScale{
	{"Small", 0.85},
	{"Default", 1},
	{"Large", 1.15},
	{"Larger", 1.3},
}

// numColumns is the number of columns in the results table.
const numColumns = len(headerTitles)

// defaultColumnWidths are the results table's column widths before the user
// drags them: Name, Path, Size, Last Modified. Size fits the widest size
// label, a decimal one such as "999.9kB".
var defaultColumnWidths = [numColumns]float32{400, 590, 80, 190}

// appPrefs holds the user's preferences. Read and written on the UI thread
// only; code running elsewhere is handed the values it needs (see
// searchOptions).
type appPrefs struct {
	Theme      string // themeSystem, themeLight or themeDark
	Accent     string // a name from accentColors, or "" for Fyne's default
	MatchColor string // a name from accentColors, or "" for the default orange
	TextScale  float64

	ResultLimit  int
	ShowHidden   bool
	DecimalSizes bool

	// RememberLayout keeps the column widths and sort order across launches.
	RememberLayout bool
	ColumnWidths   [numColumns]float32 // 0 means the default width
	SortCol        int                 // -1 for the database order
	SortAsc        bool
}

func defaultPrefs() appPrefs {
	return appPrefs{
		Theme:          themeSystem,
		TextScale:      1,
		ResultLimit:    1000,
		ShowHidden:     true,
		RememberLayout: true,
		SortCol:        -1,
		SortAsc:        true,
	}
}

// prefs is the live set of preferences. UI thread only.
var prefs = defaultPrefs()

// Preference keys, as stored by fyne.Preferences.
const (
	keyTheme          = "theme"
	keyAccent         = "accentColor"
	keyMatchColor     = "matchColor"
	keyTextScale      = "textScale"
	keyResultLimit    = "resultLimit"
	keyShowHidden     = "showHidden"
	keyDecimalSizes   = "decimalSizes"
	keyRememberLayout = "rememberLayout"
	keyColumnWidths   = "columnWidths"
	keySortCol        = "sortColumn"
	keySortAsc        = "sortAscending"
)

// loadPrefs reads the stored preferences, falling back to the default for
// anything missing or out of range (a hand-edited or older preferences file).
func loadPrefs(store fyne.Preferences) appPrefs {
	d := defaultPrefs()
	p := appPrefs{
		Theme:          store.StringWithFallback(keyTheme, d.Theme),
		Accent:         store.StringWithFallback(keyAccent, d.Accent),
		MatchColor:     store.StringWithFallback(keyMatchColor, d.MatchColor),
		TextScale:      store.FloatWithFallback(keyTextScale, d.TextScale),
		ResultLimit:    store.IntWithFallback(keyResultLimit, d.ResultLimit),
		ShowHidden:     store.BoolWithFallback(keyShowHidden, d.ShowHidden),
		DecimalSizes:   store.BoolWithFallback(keyDecimalSizes, d.DecimalSizes),
		RememberLayout: store.BoolWithFallback(keyRememberLayout, d.RememberLayout),
		SortCol:        store.IntWithFallback(keySortCol, d.SortCol),
		SortAsc:        store.BoolWithFallback(keySortAsc, d.SortAsc),
	}

	if p.Theme != themeLight && p.Theme != themeDark {
		p.Theme = themeSystem
	}
	if _, ok := accentColor(p.Accent); !ok {
		p.Accent = d.Accent
	}
	if _, ok := accentColor(p.MatchColor); !ok {
		p.MatchColor = d.MatchColor
	}
	if !slices.ContainsFunc(textScales, func(s textScale) bool { return s.scale == p.TextScale }) {
		p.TextScale = d.TextScale
	}
	if !slices.Contains(resultLimits, p.ResultLimit) {
		p.ResultLimit = d.ResultLimit
	}
	if p.SortCol < -1 || p.SortCol >= numColumns {
		p.SortCol = d.SortCol
	}
	for i, w := range store.FloatList(keyColumnWidths) {
		if i < numColumns && w > 0 {
			p.ColumnWidths[i] = float32(w)
		}
	}
	return p
}

// save writes every preference to store.
func (p appPrefs) save(store fyne.Preferences) {
	store.SetString(keyTheme, p.Theme)
	store.SetString(keyAccent, p.Accent)
	store.SetString(keyMatchColor, p.MatchColor)
	store.SetFloat(keyTextScale, p.TextScale)
	store.SetInt(keyResultLimit, p.ResultLimit)
	store.SetBool(keyShowHidden, p.ShowHidden)
	store.SetBool(keyDecimalSizes, p.DecimalSizes)
	store.SetBool(keyRememberLayout, p.RememberLayout)
	widths := make([]float64, numColumns)
	for i, w := range p.ColumnWidths {
		widths[i] = float64(w)
	}
	store.SetFloatList(keyColumnWidths, widths)
	store.SetInt(keySortCol, p.SortCol)
	store.SetBool(keySortAsc, p.SortAsc)
}

// columnWidth is the width to give column col: the remembered one, if there
// is one and layout is remembered, else the default fitted to the text size
// (see fitColumnWidths). A remembered width was set at the text size in use,
// so it is taken as is.
func (p appPrefs) columnWidth(col int) float32 {
	if p.RememberLayout && p.ColumnWidths[col] > 0 {
		return p.ColumnWidths[col]
	}
	return fitColumnWidths(defaultColumnWidths, float32(p.TextScale))[col]
}

// minPathWidth is as narrow as fitColumnWidths makes the Path column.
const minPathWidth = 200

// fitColumnWidths adjusts widths set at one text size for a size ratio times
// as large. Size and Last Modified hold text of a fixed length, which has to
// fit, so they scale with the text. Path gives up (or takes back) the
// difference, so the table keeps its overall width and those columns stay on
// screen; a long path truncates with an ellipsis anyway. Name keeps its width.
func fitColumnWidths(widths [numColumns]float32, ratio float32) [numColumns]float32 {
	var grown float32
	for _, col := range []int{2, 3} {
		w := widths[col] * ratio
		grown += w - widths[col]
		widths[col] = w
	}
	widths[1] = max(widths[1]-grown, minPathWidth)
	return widths
}

// searchOptions are the preferences a search needs, copied on the UI thread
// so the search goroutine never reads prefs.
type searchOptions struct {
	limit        int
	showHidden   bool
	decimalSizes bool
}

func (p appPrefs) searchOptions() searchOptions {
	return searchOptions{
		limit:        p.ResultLimit,
		showHidden:   p.ShowHidden,
		decimalSizes: p.DecimalSizes,
	}
}

// savePrefs stores prefs in the app's preferences. UI thread only.
func savePrefs() {
	prefs.save(fyne.CurrentApp().Preferences())
}
