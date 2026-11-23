package main

import (
	"image/color"
	"os"
	"reflect"
	"runtime"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// preferencesLabel is the menu item's label. Fyne moves an item with exactly
// this label into the application menu on macOS, where Preferences belongs.
const preferencesLabel = "Preferences…"

// preferencesShortcut is Ctrl+, (Cmd+, on macOS).
var preferencesShortcut = &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}

// appTheme is the theme the app runs with. The Appearance tab changes it in
// place and sets it again to repaint.
var appTheme = &everythingxTheme{}

// searchBox is the search entry, so a change to the Results tab can run the
// current search again. Nil until loadUI builds it.
var searchBox *searchEntry

// prefsWindow is the open Preferences window, or nil. There is only ever one:
// asking again brings it forward. UI thread only.
var prefsWindow fyne.Window

// showPreferences opens the Preferences window, or raises it if it is open.
func showPreferences() {
	if prefsWindow != nil {
		prefsWindow.Show()
		prefsWindow.RequestFocus()
		return
	}
	w := fyne.CurrentApp().NewWindow("EverythingX Preferences")
	prefsWindow = w
	w.SetContent(buildPreferences(w))
	w.SetOnClosed(func() { prefsWindow = nil })
	w.Resize(fyne.NewSize(620, 460))
	w.CenterOnScreen()
	w.Show()
}

// buildPreferences lays out the Preferences window. Every control applies its
// change at once; there is no OK or Apply.
func buildPreferences(w fyne.Window) fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Appearance", theme.ColorPaletteIcon(), appearanceTab()),
		container.NewTabItemWithIcon("Results", theme.ListIcon(), resultsTab()),
		container.NewTabItemWithIcon("Window", theme.ViewFullScreenIcon(), windowTab()),
		container.NewTabItemWithIcon("Shortcut", theme.ComputerIcon(), shortcutTab(w)),
	)

	restore := widget.NewButton("Restore Defaults", func() {
		restoreDefaults()
		// Rebuild so every control shows the defaults.
		w.SetContent(buildPreferences(w))
	})
	closeButton := widget.NewButton("Close", w.Close)
	bottom := container.NewHBox(restore, layout.NewSpacer(), closeButton)

	return container.NewBorder(nil, container.NewPadded(bottom), nil, nil, tabs)
}

func appearanceTab() fyne.CanvasObject {
	themeLabels := []string{"System", "Light", "Dark"}
	themeModes := []string{themeSystem, themeLight, themeDark}
	themes := widget.NewRadioGroup(themeLabels, nil)
	themes.Horizontal = true
	themes.Required = true
	themes.Selected = themeLabels[max(slices.Index(themeModes, prefs.Theme), 0)]
	themes.OnChanged = func(label string) {
		if i := slices.Index(themeLabels, label); i >= 0 {
			prefs.Theme = themeModes[i]
			appearanceChanged()
		}
	}

	accent := newSwatchRow(defaultAccentName,
		func() string { return prefs.Accent },
		func(name string) { prefs.Accent = name; appearanceChanged() })
	match := newSwatchRow(defaultMatchName,
		func() string { return prefs.MatchColor },
		func(name string) { prefs.MatchColor = name; appearanceChanged() })

	sizeLabels := make([]string, len(textScales))
	for i, s := range textScales {
		sizeLabels[i] = s.label
	}
	textSize := widget.NewSelect(sizeLabels, nil)
	for _, s := range textScales {
		if s.scale == prefs.TextScale {
			textSize.Selected = s.label
		}
	}
	textSize.OnChanged = func(label string) {
		for _, s := range textScales {
			if s.label == label && s.scale != prefs.TextScale {
				ratio := s.scale / prefs.TextScale
				prefs.TextScale = s.scale
				appearanceChanged()
				scaleColumnWidths(float32(ratio))
			}
		}
	}

	font := widget.NewSelect(fontLabels, nil)
	font.Selected = fontLabels[max(slices.Index(fontChoices, prefs.Font), 0)]
	font.OnChanged = func(label string) {
		if i := slices.Index(fontLabels, label); i >= 0 && fontChoices[i] != prefs.Font {
			prefs.Font = fontChoices[i]
			appearanceChanged()
		}
	}

	densityLabels := []string{"Compact", "Comfortable"}
	densities := []string{densityCompact, densityComfortable}
	density := widget.NewRadioGroup(densityLabels, nil)
	density.Horizontal = true
	density.Required = true
	density.Selected = densityLabels[max(slices.Index(densities, prefs.Density), 0)]
	density.OnChanged = func(label string) {
		if i := slices.Index(densityLabels, label); i >= 0 {
			prefs.Density = densities[i]
			appearanceChanged()
		}
	}

	return container.NewVScroll(widget.NewForm(
		widget.NewFormItem("Theme", themes),
		widget.NewFormItem("Accent color", accent),
		&widget.FormItem{Text: "Match color", Widget: match,
			HintText: "The part of a name that matches the search"},
		&widget.FormItem{Text: "Font", Widget: container.NewHBox(font),
			HintText: "System uses your desktop's interface font"},
		widget.NewFormItem("Text size", container.NewHBox(textSize)),
		widget.NewFormItem("Density", density),
	))
}

func resultsTab() fyne.CanvasObject {
	limitLabels := make([]string, len(resultLimits))
	for i, n := range resultLimits {
		limitLabels[i] = formatCount(n)
	}
	limit := widget.NewSelect(limitLabels, nil)
	limit.Selected = formatCount(prefs.ResultLimit)
	limit.OnChanged = func(label string) {
		if i := slices.Index(limitLabels, label); i >= 0 {
			prefs.ResultLimit = resultLimits[i]
			resultsChanged()
		}
	}

	hidden := widget.NewCheck("Show hidden files and folders", nil)
	hidden.Checked = prefs.ShowHidden
	hidden.OnChanged = func(on bool) {
		prefs.ShowHidden = on
		resultsChanged()
	}

	unitLabels := []string{"Binary (1.5K = 1,536 bytes)", "Decimal (1.5kB = 1,500 bytes)"}
	units := widget.NewRadioGroup(unitLabels, nil)
	units.Required = true
	units.Selected = unitLabels[0]
	if prefs.DecimalSizes {
		units.Selected = unitLabels[1]
	}
	units.OnChanged = func(label string) {
		prefs.DecimalSizes = label == unitLabels[1]
		resultsChanged()
	}

	dateLabels := []string{"Nov 23 2026 14:23", "2026-11-23 14:23"}
	dates := widget.NewRadioGroup(dateLabels, nil)
	dates.Required = true
	dates.Selected = dateLabels[0]
	if prefs.ISODates {
		dates.Selected = dateLabels[1]
	}
	dates.OnChanged = func(label string) {
		prefs.ISODates = label == dateLabels[1]
		resultsChanged()
	}

	layoutCheck := widget.NewCheck("Remember column widths and sort order", nil)
	layoutCheck.Checked = prefs.RememberLayout
	layoutCheck.OnChanged = func(on bool) {
		prefs.RememberLayout = on
		if on {
			saveLayout() // start from what is on screen now
			return
		}
		// Forget the stored layout, so turning this back on later doesn't
		// bring back an old one. The window keeps its current layout.
		prefs.ColumnWidths = [numColumns]float32{}
		prefs.SortCol, prefs.SortAsc = defaultPrefs().SortCol, defaultPrefs().SortAsc
		savePrefs()
	}

	return container.NewVScroll(widget.NewForm(
		widget.NewFormItem("Maximum results", container.NewHBox(limit)),
		&widget.FormItem{Text: "Hidden files", Widget: hidden,
			HintText: "Names starting with a dot, and everything inside such folders"},
		widget.NewFormItem("Size units", units),
		widget.NewFormItem("Dates", dates),
		widget.NewFormItem("Columns", layoutCheck),
	))
}

func windowTab() fyne.CanvasObject {
	exit := widget.NewCheck("Quit EverythingX when the window is closed", nil)
	exit.Checked = prefs.ExitOnClose
	exit.OnChanged = func(on bool) {
		prefs.ExitOnClose = on
		savePrefs()
	}

	escape := widget.NewCheck("Close the window when Escape is pressed", nil)
	escape.Checked = prefs.CloseOnEscape
	escape.OnChanged = func(on bool) {
		prefs.CloseOnEscape = on
		savePrefs()
	}

	return container.NewVScroll(widget.NewForm(
		&widget.FormItem{Text: "On close", Widget: exit,
			HintText: "Otherwise the window hides and EverythingX keeps running in the tray"},
		&widget.FormItem{Text: "Escape key", Widget: escape,
			HintText: "Otherwise Escape clears the search. Quits instead when the option above is on"},
	))
}

// shortcutTab explains how to open EverythingX from a key. Fyne can't grab a
// global hotkey, and Wayland doesn't let apps grab one at all, but running the
// app again brings the open window forward (see serveInstance and
// watchReopen), so a desktop shortcut that runs it does the job.
func shortcutTab(w fyne.Window) fyne.CanvasObject {
	command := launchCommand()

	intro := widget.NewLabel("EverythingX can't set a global keyboard shortcut itself. " +
		"Add one in your desktop's keyboard settings that runs the command below. " +
		"When EverythingX is already open, running it again shows its window instead of starting another copy.")
	intro.Wrapping = fyne.TextWrapWord

	cmdLabel := widget.NewLabelWithStyle(command, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	cmdLabel.Selectable = true
	cmdLabel.Truncation = fyne.TextTruncateEllipsis
	copyButton := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		w.Clipboard().SetContent(command)
	})
	cmdRow := container.NewBorder(nil, nil, nil, copyButton, cmdLabel)

	steps := widget.NewRichTextFromMarkdown(shortcutSteps())
	steps.Wrapping = fyne.TextWrapWord

	return container.NewVScroll(container.NewVBox(intro, cmdRow, steps))
}

// launchCommand is the command that opens EverythingX.
func launchCommand() string {
	if runtime.GOOS == "darwin" {
		return "open -a EverythingX"
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "everythingx"
}

func shortcutSteps() string {
	if runtime.GOOS == "darwin" {
		return "**Shortcuts app:** create a shortcut with the *Open App* action set to EverythingX, " +
			"then add a keyboard shortcut in the shortcut's details.\n\n" +
			"Launchers and hotkey tools that run commands can use the command above."
	}
	return "**GNOME:** Settings → Keyboard → View and Customize Shortcuts → Custom Shortcuts\n\n" +
		"**KDE Plasma:** System Settings → Keyboard → Shortcuts → Add New → Command or Script\n\n" +
		"**Other desktops:** look for custom shortcuts in the keyboard or shortcut settings.\n\n" +
		"On Wayland, apps can't raise their own windows, so a minimized EverythingX window may stay minimized."
}

// appearanceChanged stores the appearance preferences and repaints with them.
func appearanceChanged() {
	savePrefs()
	applyAppearance()
}

// applyAppearance repaints every window with the appearance preferences.
func applyAppearance() {
	appTheme.setPrefs(prefs)
	fyne.CurrentApp().Settings().SetTheme(appTheme)
	// The text size decides how many characters fit in the Name column.
	updateNameBudget()
	if t != nil {
		t.Refresh()
	}
}

// resultsChanged stores the results preferences and shows the current search
// again with them.
func resultsChanged() {
	savePrefs()
	rerunSearch()
}

// rerunSearch runs the search in the search box again, or shows the default
// view when the box is empty.
func rerunSearch() {
	if t == nil || statusBar == nil || searchBox == nil {
		return
	}
	// A search still waiting out the debounce took the old preferences.
	if searchDebounceTimer != nil {
		searchDebounceTimer.Stop()
	}
	if searchBox.Text == "" {
		showDefaultView(t, statusBar)
		return
	}
	handleAutoCompleteEntryChanged(searchBox.Text, prefs.searchOptions(), t, statusBar)
}

// restoreDefaults puts every preference back to its default and applies it,
// including the column widths and sort order on screen.
func restoreDefaults() {
	prefs = defaultPrefs()
	savePrefs()
	applyAppearance()
	sortCol, sortAsc = prefs.SortCol, prefs.SortAsc
	applyColumnWidths()
	rerunSearch()
}

// applyColumnWidths sizes the results table's columns from the preferences.
func applyColumnWidths() {
	if t == nil {
		return
	}
	for col := range numColumns {
		t.SetColumnWidth(col, prefs.columnWidth(col))
	}
	updateNameBudget()
}

// scaleColumnWidths refits the columns for text ratio times as large, so a
// size or date isn't cut off (see fitColumnWidths).
func scaleColumnWidths(ratio float32) {
	if t == nil {
		return
	}
	set := tableColumnWidths(t)
	var widths [numColumns]float32
	for col := range numColumns {
		w, ok := set[col]
		if !ok {
			w = defaultColumnWidths[col]
		}
		widths[col] = w
	}
	for col, w := range fitColumnWidths(widths, ratio) {
		t.SetColumnWidth(col, w)
	}
	updateNameBudget()
	saveLayout()
}

// saveLayout stores the column widths and sort order on screen, when the user
// wants them remembered. UI thread only.
func saveLayout() {
	if !prefs.RememberLayout || t == nil {
		return
	}
	for col, w := range tableColumnWidths(t) {
		if col >= 0 && col < numColumns {
			prefs.ColumnWidths[col] = w
		}
	}
	prefs.SortCol, prefs.SortAsc = sortCol, sortAsc
	savePrefs()
}

// tableColumnWidths returns the widths of the columns that have been sized,
// by SetColumnWidth or by dragging a divider. Fyne has no getter for them and
// no callback for a drag, so this reads the table's unexported columnWidths
// field. Reading an unexported field through reflect is allowed; writing is
// not, and isn't needed. TestTableColumnWidthsReadsFyne fails if a Fyne
// upgrade renames the field, rather than layouts silently not being saved.
func tableColumnWidths(table *widget.Table) map[int]float32 {
	field := reflect.ValueOf(table).Elem().FieldByName("columnWidths")
	if field.Kind() != reflect.Map {
		return nil
	}
	widths := make(map[int]float32, field.Len())
	iter := field.MapRange()
	for iter.Next() {
		widths[int(iter.Key().Int())] = float32(iter.Value().Float())
	}
	return widths
}

// updateNameBudget works out how wide the Name column's text can be, so a
// highlighted name can fall back to plain truncation when the match is too far
// right (see nameSegments). The file-type icon, the space beside it and the
// text's own inset take width off the column. Run it whenever the text size,
// font, density or the Name column's width is set.
func updateNameBudget() {
	width := defaultColumnWidths[0]
	if t != nil {
		if w, ok := tableColumnWidths(t)[0]; ok {
			width = w
		}
	}
	nameHighlightMaxWidth = width - theme.IconInlineSize() - theme.Padding() - 2*theme.InnerPadding()
}

// newSwatchRow is a row of colour swatches with the chosen colour's name
// after it. get and set read and write the preference; picking the default
// colour stores "", so the default can change with the app.
func newSwatchRow(defaultName string, get func() string, set func(string)) fyne.CanvasObject {
	current := func() string {
		if name := get(); name != "" {
			return name
		}
		return defaultName
	}

	name := widget.NewLabel("")
	updateName := func() {
		for _, c := range accentColors {
			if c.name == current() {
				name.SetText(c.label)
				if c.name == defaultName {
					name.SetText(c.label + " (default)")
				}
			}
		}
	}

	var swatches []fyne.CanvasObject
	for _, c := range accentColors {
		s := &colorSwatch{color: c.color}
		s.selected = func() bool { return current() == c.name }
		s.onTapped = func() {
			if c.name == defaultName {
				set("")
			} else {
				set(c.name)
			}
			for _, o := range swatches {
				o.Refresh()
			}
			updateName()
		}
		s.ExtendBaseWidget(s)
		swatches = append(swatches, s)
	}
	updateName()

	return container.NewHBox(append(swatches, name)...)
}

// colorSwatch is a tappable square of colour, outlined when selected.
type colorSwatch struct {
	widget.BaseWidget
	color    color.Color
	selected func() bool
	onTapped func()
}

func (s *colorSwatch) Tapped(*fyne.PointEvent) {
	if s.onTapped != nil {
		s.onTapped()
	}
}

func (s *colorSwatch) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (s *colorSwatch) CreateRenderer() fyne.WidgetRenderer {
	r := &swatchRenderer{s: s, rect: canvas.NewRectangle(s.color)}
	r.Refresh()
	return r
}

type swatchRenderer struct {
	s    *colorSwatch
	rect *canvas.Rectangle
}

func (r *swatchRenderer) Layout(size fyne.Size) { r.rect.Resize(size) }

func (r *swatchRenderer) MinSize() fyne.Size { return fyne.NewSquareSize(28) }

func (r *swatchRenderer) Refresh() {
	r.rect.FillColor = r.s.color
	r.rect.CornerRadius = theme.SelectionRadiusSize()
	r.rect.StrokeWidth = 0
	if r.s.selected != nil && r.s.selected() {
		r.rect.StrokeColor = theme.Color(theme.ColorNameForeground)
		r.rect.StrokeWidth = 3
	}
	r.rect.Refresh()
}

func (r *swatchRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }

func (r *swatchRenderer) Destroy() {}
