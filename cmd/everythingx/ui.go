package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"image/color"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// searchDebounce is how long to wait after the last keystroke before running a
// search, so fast typing doesn't spawn a search (and DB query) per character.
const searchDebounce = 120 * time.Millisecond

type RowData struct {
	Name         []string // alternating plain and matched parts (odd indexes are matches), for highlighting
	Base         string   // the whole file name, for sorting
	Path         string
	Size         string
	Modified     string
	SizeBytes    int64 // -1 for directories, which show "--"
	ModTime      time.Time
	SearchResult *shared.SearchResult
}

// Sort state for the results table. Column -1 keeps the order the database
// returned (filename ascending). UI thread only.
var sortCol = -1
var sortAsc = true

// sortRows orders results by the active sort column. UI thread only, as it
// reads the sort state that the column headers write.
func sortRows(rows []RowData) {
	if sortCol < 0 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		c := compareRows(rows[i], rows[j])
		if !sortAsc {
			c = -c
		}
		return c < 0
	})
}

func compareRows(a, b RowData) int {
	switch sortCol {
	case 0:
		return strings.Compare(strings.ToLower(a.Base), strings.ToLower(b.Base))
	case 1:
		return strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path))
	case 2:
		return cmp.Compare(a.SizeBytes, b.SizeBytes)
	case 3:
		return a.ModTime.Compare(b.ModTime)
	}
	return 0
}

// searchCounter is bumped on every search request. Each search goroutine
// captures its own id and bails out (including inside the final fyne.Do
// closure) once a newer search has started, so a slow/older search can never
// overwrite the results of a newer one.
var searchCounter atomic.Int64

// searchDebounceTimer coalesces rapid keystrokes into a single search. It is
// only ever touched from the UI thread (in entry.OnChanged).
var searchDebounceTimer *time.Timer

// indexedText is the size of the index, e.g. "5,785,272 files indexed", or ""
// until the count arrives. UI thread only.
var indexedText string

// showingDefault is true while the table holds the default view (the home
// directory's files) rather than search results. UI thread only.
var showingDefault bool

// defaultStatus is the status bar text for the default view. The index size
// has its own place at the other end of the bar (indexStatus).
func defaultStatus(items int) string {
	return fmt.Sprintf("%d files in ~", items)
}

// showDefaultView lists the files directly in the home directory, which is what the
// window shows whenever the search box is empty. The listing runs off the UI
// thread and, like a search, is dropped if a search starts before it lands.
// Must run on the UI thread.
func showDefaultView(t *widget.Table, statusBar *widget.Label) {
	myID := searchCounter.Add(1) // invalidate any in-flight search
	opts := prefs.searchOptions()

	go func() {
		var rows []RowData
		if home, err := os.UserHomeDir(); err == nil {
			results, err := homeResults(home, opts.showHidden)
			if err != nil {
				fmt.Println("Error listing home directory:", err)
			}
			rows = buildRows(results, "", opts)
		}

		fyne.Do(func() {
			applyRows(myID, rows, defaultStatus(len(rows)), true, t, statusBar)
		})
	}()
}

// homeResults lists the files directly in dir, not recursively, as search
// results sorted by name ignoring case. Dotfiles are included only when
// showHidden is set. Directories and symlinks to directories are skipped.
func homeResults(dir string, showHidden bool) ([]*shared.SearchResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	results := make([]*shared.SearchResult, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || (!showHidden && strings.HasPrefix(e.Name(), ".")) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		objType := shared.ItemIsFile
		if e.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				continue
			}
			objType = shared.ItemIsSymlink
		}
		results = append(results, &shared.SearchResult{Fullpath: path, ObjectType: objType})
	}

	sort.SliceStable(results, func(i, j int) bool {
		return strings.ToLower(results[i].Fullpath) < strings.ToLower(results[j].Fullpath)
	})
	return results, nil
}

// isoDateLayout is the ISO date format the Results tab offers, as file
// managers on Linux write dates.
const isoDateLayout = "2006-01-02 15:04"

// buildRows stats each result and splits its name around searchText for
// highlighting: around each literal part of a glob, or the first occurrence of
// a literal term. An empty searchText highlights nothing. Sizes and dates are
// written as opts says. Does file I/O, so run it off the UI thread.
func buildRows(results []*shared.SearchResult, searchText string, opts searchOptions) []RowData {
	rows := make([]RowData, 0, len(results))
	for _, r := range results {
		fullpath := r.Fullpath
		base := filepath.Base(r.Fullpath)
		dir := filepath.Dir(fullpath) + "/"
		size, modified, sizeBytes, modTime := shared.GetFileSizeMod(fullpath)
		if size != "" {
			size = shared.HumanizeSizeUnits(sizeBytes, opts.decimalSizes)
		}
		if opts.isoDates && modified != "" {
			modified = modTime.Format(isoDateLayout)
		}

		// A glob matches the whole name, so take its spans before the
		// trailing slash a directory row gets.
		var name []string
		if spans := ffdb.HighlightSpans(base, searchText); spans != nil {
			name = ffdb.SplitBySpans(base, spans)
		}

		if r.ObjectType == shared.ItemIsDir {
			base += "/"
			fullpath += "/"
			size = "--"
			sizeBytes = -1
			if name != nil {
				name[len(name)-1] += "/"
			}
		}

		if name == nil {
			beforeTerm, searchTerm, afterTerm := shared.SplitFileName(base, searchText)
			name = []string{beforeTerm, searchTerm, afterTerm}
		}

		rows = append(rows, RowData{
			Name:         name,
			Base:         base,
			Path:         dir,
			Size:         size,
			Modified:     modified,
			SizeBytes:    sizeBytes,
			ModTime:      modTime,
			SearchResult: r,
		})
	}
	return rows
}

// applyRows puts rows in the table unless a newer search or default view has
// started since id was taken. isDefault says whether rows are the default view
// or search results. Must run on the UI thread.
func applyRows(id int64, rows []RowData, resultText string, isDefault bool, t *widget.Table, statusBar *widget.Label) {
	// Re-check on the UI thread: a newer search may have started between the
	// caller's last check and this running. Without this, an older/slower
	// search could overwrite fresher results.
	if searchCounter.Load() != id {
		return
	}
	showingDefault = isDefault
	tableData = rows
	sortRows(tableData) // keep the chosen column ordering as results stream in
	lastResultText = resultText
	selectedRow = -1
	hoveredRow = -1
	t.Refresh()
	t.ScrollToTop() // reset scroll so new results start at the top
	statusBar.SetText(resultText)
	updateEmptyState()
}

// emptyState is the logo and prompt drawn over the table when the default view
// has nothing to list. Nil until loadUI builds it.
var emptyState *fyne.Container

// indexLabel shows the index size on the empty state.
var indexLabel *widget.Label

// updateEmptyState shows the logo only when the default view is empty. A
// search with no hits keeps the table, so "0 objects" reads as an answer. Must
// run on the UI thread.
func updateEmptyState() {
	if emptyState == nil {
		return
	}
	if showingDefault && len(tableData) == 0 {
		emptyState.Show()
	} else {
		emptyState.Hide()
	}
}

// loadIndexCount counts the indexed files off the UI thread, then shows the
// count in the empty state and, while the default view is up, the status bar.
// Showing it at all tells the user the index is there and being read.
func loadIndexCount(db *sql.DB) {
	go func() {
		count, err := ffdb.RecordCount(db)
		fyne.Do(func() {
			if err != nil {
				fmt.Println("Error counting indexed files:", err)
				indexedText = "Index not available. Is everythingxd running?"
			} else {
				indexedText = formatCount(count) + " files indexed"
			}
			indexLabel.SetText(indexedText)
			if indexStatus != nil {
				indexStatus.SetText(indexedText)
			}
		})
	}()
}

// formatCount writes n with thousands separators, e.g. 5785272 as "5,785,272".
func formatCount(n int) string {
	if n < 0 {
		return "-" + formatCount(-n)
	}
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// handleAutoCompleteEntryChanged runs the search off the UI thread so that
// scrolling and other interactions remain responsive while the DB query and
// per-file stat calls execute. opts are the preferences at the time of the
// keystroke, taken on the UI thread.
func handleAutoCompleteEntryChanged(searchText string, opts searchOptions, t *widget.Table, statusBar *widget.Label) {
	if len(searchText) == 0 {
		fyne.Do(func() { showDefaultView(t, statusBar) })
		return
	}

	myID := searchCounter.Add(1)

	go func() {
		start := time.Now()

		searchStart := time.Now()
		results, err := ffdb.Search(searchText, opts.limit, ffdb.SearchOptions{HideHidden: !opts.showHidden})
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		searchElapsed := time.Since(searchStart)

		if searchCounter.Load() != myID {
			return
		}

		newData := buildRows(results, searchText, opts)

		if searchCounter.Load() != myID {
			return
		}

		var resultText string
		if len(results) == opts.limit {
			resultText = fmt.Sprintf("Showing first %s objects", formatCount(opts.limit))
		} else {
			resultText = fmt.Sprintf("%d objects", len(results))
		}

		fyne.Do(func() {
			applyRows(myID, newData, resultText, false, t, statusBar)
		})

		shared.PrintMemUsage()

		elapsed := time.Since(start)
		fmt.Printf(
			"\tSearch: %s, Results: %d, prefixSearch: %s, handleEntryChanged %s.\n",
			searchText,
			len(results),
			searchElapsed,
			elapsed,
		)
	}()
}

// resultCell is a RichText cell in the results table. It takes the pointer
// and clicks for its row: hover and selection highlights, opening, and the
// context menu. When its text is cut short with an ellipsis, hovering shows
// the whole of it in a tool tip.
type resultCell struct {
	ttwidget.RichText
	path string
	col  int
	row  int
}

func newResultCell() *resultCell {
	c := &resultCell{row: -1}
	c.Scroll = container.ScrollNone
	c.Truncation = fyne.TextTruncateEllipsis
	c.RichText.ExtendBaseWidget(c)
	return c
}

// MouseIn lights the row the pointer has entered and, if the cell's text
// doesn't fit, arms a tool tip with all of it.
func (c *resultCell) MouseIn(e *desktop.MouseEvent) {
	setHoveredRow(c.row)
	c.SetToolTip(c.truncatedText())
	c.RichText.MouseIn(e)
}

// MouseOut fires when the pointer leaves this cell, including on the way out of
// the table entirely, so it is where the row highlight is dropped. Moving to
// another cell clears and re-sets the highlight before the next paint, so a
// sweep across a row does not flicker.
func (c *resultCell) MouseOut() {
	setHoveredRow(-1)
	c.cancelToolTip()
}

// cancelToolTip drops a pending or showing tool tip without touching the row
// highlight, for when the cell changes under a pointer that hasn't left it.
func (c *resultCell) cancelToolTip() {
	c.RichText.MouseOut()
}

// truncatedText is the cell's full text when it is too wide for the cell, so
// it is drawn cut short, or "" when it all shows.
func (c *resultCell) truncatedText() string {
	th := c.Theme()
	avail := c.Size().Width - 2*th.Size(theme.SizeNameInnerPadding)
	var width float32
	var text strings.Builder
	for _, seg := range c.Segments {
		ts, ok := seg.(*widget.TextSegment)
		if !ok {
			continue
		}
		size := ts.Style.SizeName
		if size == "" {
			size = theme.SizeNameText
		}
		width += fyne.MeasureText(ts.Text, th.Size(size), ts.Style.TextStyle).Width
		text.WriteString(ts.Text)
	}
	if width <= avail {
		return ""
	}
	return text.String()
}

// flashStatus shows a transient message in the status bar, then restores the
// result count. Must be called on the UI thread.
func flashStatus(msg string) {
	if statusBar == nil {
		return
	}
	statusBar.SetText(msg)
	time.AfterFunc(1500*time.Millisecond, func() {
		fyne.Do(func() {
			statusBar.SetText(lastResultText)
		})
	})
}

func copyToClipboard(text string) {
	mainWindow.Clipboard().SetContent(text)
	flashStatus("✓ Copied!")
}

// MouseDown selects on press, as a file manager does. Selecting from Tapped
// instead would feel laggy: Fyne defers Tapped on a DoubleTappable widget until
// the system double-click interval has passed, to see if a second click lands.
func (c *resultCell) MouseDown(e *desktop.MouseEvent) {
	c.cancelToolTip() // a click means the pointer is busy; don't pop up over it
	if e.Button == desktop.MouseButtonPrimary {
		c.selectRow()
	}
}

func (c *resultCell) MouseUp(_ *desktop.MouseEvent) {}

// nameColumn and pathColumn are the table columns holding the file name and
// the containing folder.
const (
	nameColumn = 0
	pathColumn = 1
)

// Tapped on the Name column opens the item, like a link. Fyne only delivers it
// once the double-click interval passes with no second click, so a double-click
// opens the item once, through DoubleTapped.
func (c *resultCell) Tapped(_ *fyne.PointEvent) {
	if c.col == nameColumn {
		openFile(c.path)
	}
}

// Cursor shows a pointing hand over the Name column, where a click opens.
func (c *resultCell) Cursor() desktop.Cursor {
	if c.col == nameColumn {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// DoubleTapped on the Path column shows the item selected in its folder, which
// is what that column is about. Anywhere else it opens the item itself.
func (c *resultCell) DoubleTapped(_ *fyne.PointEvent) {
	if c.col == pathColumn {
		revealFile(c.path)
		return
	}
	openFile(c.path)
}

// TappedSecondary opens the row's context menu.
func (c *resultCell) TappedSecondary(e *fyne.PointEvent) {
	if c.path == "" {
		return
	}
	c.selectRow()

	// A tool tip draws into the layer on the window content, which the menu
	// overlay covers. Cancel any pending one before the overlay goes up, or it
	// fires under the menu with nowhere to draw.
	c.cancelToolTip()

	widget.NewPopUpMenu(contextMenu(c.path), mainWindow.Canvas()).ShowAtPosition(e.AbsolutePosition)
}

// contextMenu is the right-click menu for a result. Actions get an icon, as
// in a GTK app's menus; Open With, whose submenu carries the app icons, gets
// a blank one so its label lines up with the rest.
func contextMenu(path string) *fyne.Menu {
	openWith := openWithMenuItem(path)
	openWith.Icon = blankMenuIcon
	return fyne.NewMenu("",
		fyne.NewMenuItemWithIcon("Open", theme.DocumentIcon(), func() { openFile(path) }),
		openWith,
		fyne.NewMenuItemWithIcon(revealLabel, theme.FolderOpenIcon(), func() { revealFile(path) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon("Copy Name", theme.ContentCopyIcon(), func() { copyToClipboard(filepath.Base(path)) }),
		fyne.NewMenuItemWithIcon("Copy Full Path", theme.ContentCopyIcon(), func() { copyToClipboard(path) }),
		fyne.NewMenuItemWithIcon("Copy Directory Path", theme.ContentCopyIcon(), func() { copyToClipboard(filepath.Dir(path)) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(propertiesLabel, theme.InfoIcon(), func() { showProperties(path) }),
	)
}

// blankMenuIcon is an empty icon for a menu item that has none. Fyne starts
// the label where the icon ends, so an item without one would sit out of line
// with its neighbours'.
var blankMenuIcon = fyne.NewStaticResource("blank.svg",
	[]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"/>`))

// cellIcon is the file-type icon beside a Name cell. A plain widget.Icon takes
// no input, and the table offers no double-click of its own, so clicks on the
// icon would do nothing. It passes them to the cell it sits beside instead.
type cellIcon struct {
	widget.Icon
	cell *resultCell
}

func newCellIcon(cell *resultCell) *cellIcon {
	i := &cellIcon{cell: cell}
	i.ExtendBaseWidget(i)
	return i
}

func (i *cellIcon) MouseDown(e *desktop.MouseEvent)    { i.cell.MouseDown(e) }
func (i *cellIcon) MouseUp(e *desktop.MouseEvent)      { i.cell.MouseUp(e) }
func (i *cellIcon) Tapped(e *fyne.PointEvent)          { i.cell.Tapped(e) }
func (i *cellIcon) Cursor() desktop.Cursor             { return i.cell.Cursor() }
func (i *cellIcon) DoubleTapped(e *fyne.PointEvent)    { i.cell.DoubleTapped(e) }
func (i *cellIcon) TappedSecondary(e *fyne.PointEvent) { i.cell.TappedSecondary(e) }

func (c *resultCell) selectRow() {
	if c.row >= 0 {
		selectRow(c.row)
	}
}

// available for the table widget.
var tableData []RowData
var t *widget.Table
var mainWindow fyne.Window
var lastResultText string

// selectedRow is the highlighted result, or -1 for none. Fyne's own table
// selection highlights a single cell; a file list wants the whole row, so the
// highlight is drawn per-cell from this value instead. UI thread only.
var selectedRow = -1

// hoveredRow is the row under the pointer, or -1 for none. Drawn per-cell like
// selectedRow, so the whole row lights up rather than just the cell. UI thread
// only.
//
// It is fed from resultCell.MouseIn/MouseOut rather than the table's own
// OnHighlighted callback: the driver delivers hover to a single object, the
// innermost hoverable one, and every data cell is a hoverable resultCell. The
// table therefore only ever sees the positions between and around cells, which
// it treats as "no cell", so OnHighlighted never fires over a result row.
var hoveredRow = -1

// setHoveredRow moves the hover highlight, repainting only the two rows that
// change so sweeping the pointer down the list doesn't refresh the whole table.
func setHoveredRow(row int) {
	if row == hoveredRow {
		return
	}
	previous := hoveredRow
	hoveredRow = row
	refreshRow(previous)
	refreshRow(row)
}

// refreshRow repaints one row's cells. Refreshing the whole table to move a
// highlight re-runs every visible cell and is felt as a delay on click.
func refreshRow(row int) {
	if row < 0 || row >= len(tableData) {
		return
	}
	for col := range headerTitles {
		t.RefreshItem(widget.TableCellID{Row: row, Col: col})
	}
}

// selectRow highlights a row. Must run on the UI thread.
func selectRow(row int) {
	if row < 0 || row >= len(tableData) {
		return
	}
	previous := selectedRow
	selectedRow = row
	refreshRow(previous)
	refreshRow(row)
}

// moveSelection walks the selection by delta rows, keeping it on screen. The
// first keypress with nothing selected lands on the first row.
func moveSelection(delta int) {
	if len(tableData) == 0 {
		return
	}
	next := 0
	if selectedRow >= 0 {
		next = min(max(selectedRow+delta, 0), len(tableData)-1)
	}
	selectRow(next)
	t.ScrollTo(widget.TableCellID{Row: next, Col: 0})
}

// openSelected opens the highlighted result, as double-clicking its name does.
func openSelected() {
	if selectedRow >= 0 && selectedRow < len(tableData) {
		openFile(tableData[selectedRow].SearchResult.Fullpath)
	}
}

// searchEntry is the search box. It forwards list-navigation keys to the
// results table so a search can be driven entirely from the keyboard.
type searchEntry struct {
	widget.Entry
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	e.SetPlaceHolder("Search")
	e.Icon = theme.SearchIcon()
	clear := widget.NewButtonWithIcon("", theme.ContentClearIcon(), func() {
		e.SetText("")
		if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
			c.Focus(e)
		}
	})
	clear.Importance = widget.LowImportance
	clear.Hide()
	e.ActionItem = clear
	return e
}

// showClear shows the clear button only while there is text to clear, as GTK
// search entries do.
func (e *searchEntry) showClear() {
	if e.ActionItem == nil {
		return
	}
	if e.Text == "" {
		e.ActionItem.Hide()
	} else {
		e.ActionItem.Show()
	}
}

func (e *searchEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyDown:
		moveSelection(1)
	case fyne.KeyUp:
		moveSelection(-1)
	case fyne.KeyReturn, fyne.KeyEnter:
		openSelected()
	case fyne.KeyEscape:
		e.SetText("")
	default:
		e.Entry.TypedKey(key)
	}
}

var iconsByExt = map[string]fyne.ThemeIconName{}

func registerIcons(icon fyne.ThemeIconName, exts ...string) {
	for _, e := range exts {
		iconsByExt[e] = icon
	}
}

func init() {
	registerIcons(theme.IconNameFileImage,
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".heic", ".tiff", ".ico")
	registerIcons(theme.IconNameFileAudio,
		".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a")
	registerIcons(theme.IconNameFileVideo,
		".mp4", ".mov", ".avi", ".mkv", ".webm", ".m4v")
	registerIcons(theme.IconNameFileText,
		".txt", ".md", ".go", ".py", ".js", ".ts", ".json", ".yaml", ".yml", ".toml",
		".c", ".h", ".cpp", ".rs", ".java", ".sh", ".html", ".css", ".xml", ".csv", ".log")
	registerIcons(theme.IconNameFileApplication,
		".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar",
		".app", ".exe", ".dmg", ".pkg", ".deb", ".rpm")
}

// iconForRow picks the icon for a result from its object type and, for regular
// files, its extension.
func iconForRow(r *shared.SearchResult) fyne.Resource {
	switch r.ObjectType {
	case shared.ItemIsDir:
		return theme.FolderIcon()
	case shared.ItemIsSymlink:
		// Fyne has no link icon; the redo arrow reads as an alias/shortcut.
		return theme.Icon(theme.IconNameContentRedo)
	case shared.ItemIsFile:
		if name, ok := iconsByExt[strings.ToLower(filepath.Ext(r.Fullpath))]; ok {
			return theme.Icon(name)
		}
	}
	return theme.FileIcon()
}

// rowIcons caches the recoloured icons rowIcon hands out. A coloured resource
// is a new value each time it is made, and the cell re-rasterises its icon
// whenever the resource changes, so reuse one per icon and colour.
var rowIcons = map[[2]string]fyne.Resource{}

// rowIcon is iconForRow's icon in the colour the row draws it: the text
// colour on a selected row, the accent colour for a folder (as GTK file lists
// draw folders) and the theme's own colour otherwise.
func rowIcon(r *shared.SearchResult, selected bool) fyne.Resource {
	res := iconForRow(r)
	var colorName fyne.ThemeColorName
	switch {
	case selected:
		colorName = theme.ColorNameForegroundOnPrimary
	case r.ObjectType == shared.ItemIsDir:
		colorName = theme.ColorNamePrimary
	default:
		return res
	}
	key := [2]string{res.Name(), string(colorName)}
	if c, ok := rowIcons[key]; ok {
		return c
	}
	c := theme.NewColoredResource(res, colorName)
	rowIcons[key] = c
	return c
}

// nameHighlightMaxWidth is how wide the text in the Name column can be. When
// the matched term ends beyond this, the Name cell renders as a single
// truncating segment instead of highlighted inline segments, so it never
// overflows into the Path column. Recomputed by updateNameBudget.
var nameHighlightMaxWidth float32 = 360

// cellColor is the text colour for a cell: the one that reads on the solid
// selection, or "" for the theme's foreground.
func cellColor(selected bool) fyne.ThemeColorName {
	if selected {
		return theme.ColorNameForegroundOnPrimary
	}
	return ""
}

// textSegment builds a plain cell segment. RichText truncates a trailing
// segment like this with an ellipsis when it is too wide to fit.
func textSegment(text string, align fyne.TextAlign, selected bool) *widget.TextSegment {
	return &widget.TextSegment{
		Text:  text,
		Style: widget.RichTextStyle{Alignment: align, ColorName: cellColor(selected)},
	}
}

// inlineNameSegment builds an inline segment for the highlighted-name path.
// Highlighted segments are bold and use the match color, which the user picks
// on the Appearance tab; on a selected row they keep the selection's text
// colour, as a GTK list does.
func inlineNameSegment(text string, highlight, selected bool) *widget.TextSegment {
	style := widget.RichTextStyle{Inline: true, ColorName: cellColor(selected)}
	if highlight {
		style.TextStyle.Bold = true
		if !selected {
			style.ColorName = colorNameMatch
		}
	}
	return &widget.TextSegment{Text: text, Style: style}
}

// nameSegments renders a row's name parts (see RowData.Name), highlighting the
// odd-indexed ones.
func nameSegments(parts []string, selected bool) []widget.RichTextSegment {
	// last is the index of the last non-empty highlighted part, and width how
	// wide the name is up to its end.
	last := -1
	for i, p := range parts {
		if i%2 == 1 && p != "" {
			last = i
		}
	}
	var width float32
	for i := 0; i <= last; i++ {
		style := fyne.TextStyle{Bold: i%2 == 1}
		width += fyne.MeasureText(parts[i], theme.TextSize(), style).Width
	}
	if last < 0 || width > nameHighlightMaxWidth {
		// No match, or the highlighting reaches too far right to fit before
		// the column edge. Render the whole name as a single trailing segment
		// so RichText truncates it with an ellipsis; its ellipsis only applies
		// to the last segment, so a long leading segment would otherwise
		// overflow into the Path column.
		return []widget.RichTextSegment{textSegment(strings.Join(parts, ""), fyne.TextAlignLeading, selected)}
	}
	var segments []widget.RichTextSegment
	for i, p := range parts {
		if p != "" {
			segments = append(segments, inlineNameSegment(p, i%2 == 1, selected))
		}
	}
	return segments
}

func makeTable() *widget.Table {
	tableData = make([]RowData, 0, prefs.ResultLimit)

	t = widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			// Stack: selection highlight behind, icon + text in front. The icon
			// is only shown for the Name column; a hidden object is skipped by
			// the border layout, so the other columns keep their full width.
			text := newResultCell()
			icon := newCellIcon(text)
			icon.Hide()
			return container.New(rowBandLayout{},
				canvas.NewRectangle(color.Transparent),
				container.NewBorder(nil, nil, icon, nil, text),
				canvas.NewRectangle(theme.Color(colorNameGrid)),
			)
		},
		// UpdateCell()
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			data := tableData
			if id.Row < 0 || id.Row >= len(data) {
				return
			}
			row := data[id.Row]
			stack := cell.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			inner := stack.Objects[1].(*fyne.Container)
			richText := inner.Objects[0].(*resultCell)
			icon := inner.Objects[1].(*cellIcon)

			if richText.path != row.SearchResult.Fullpath {
				// Scrolling recycles this cell onto another row without the
				// pointer moving, so no MouseOut arrives. A tip already armed
				// would show the old row's text.
				if richText.path != "" {
					// Taking a different file (rather than being filled in for
					// the first time) means the list moved under a pointer that
					// never left, so the lit row is no longer the one being
					// pointed at. Assigned rather than going through
					// setHoveredRow, which would refresh from inside a refresh.
					hoveredRow = -1
				}
				richText.cancelToolTip()
				richText.path = row.SearchResult.Fullpath
			}
			richText.col = id.Col
			richText.row = id.Row

			// Only repaint what changed: this runs for every visible cell on
			// each refresh, so unconditional work here is felt as click lag.
			// The selected row is filled with the accent colour and its text
			// drawn in the colour that reads on it, as in a GTK list.
			selected := id.Row == selectedRow
			var highlight color.Color = color.Transparent
			switch {
			case selected:
				highlight = theme.Color(theme.ColorNamePrimary)
			case id.Row == hoveredRow:
				highlight = theme.Color(theme.ColorNameHover)
			}
			if bg.FillColor != highlight {
				bg.FillColor = highlight
				bg.Refresh()
			}
			// The column divider follows the theme, which can change under it.
			if div := stack.Objects[2].(*canvas.Rectangle); div.FillColor != theme.Color(colorNameGrid) {
				div.FillColor = theme.Color(colorNameGrid)
				div.Refresh()
			}

			switch id.Col {
			case 0:
				richText.Segments = nameSegments(row.Name, selected)
			case 1:
				richText.Segments = []widget.RichTextSegment{textSegment(row.Path, fyne.TextAlignLeading, selected)}
			case 2:
				richText.Segments = []widget.RichTextSegment{textSegment(row.Size, fyne.TextAlignTrailing, selected)}
			case 3:
				richText.Segments = []widget.RichTextSegment{textSegment(row.Modified, fyne.TextAlignLeading, selected)}
			}

			showIcon := id.Col == 0
			if showIcon {
				// SetResource re-rasterises the SVG, so only pay for it when
				// the icon actually changes.
				if res := rowIcon(row.SearchResult, selected); icon.Resource != res {
					icon.SetResource(res)
				}
			}
			if showIcon != icon.Visible() {
				if showIcon {
					icon.Show()
				} else {
					icon.Hide()
				}
				// Visibility decides how much width the text gets, and cells are
				// recycled between columns, so lay the row out again rather than
				// keep the geometry it was created with.
				inner.Refresh()
			} else {
				richText.Refresh()
			}
		},
	)

	// No row-number column, and no lines between rows: a file list in GTK
	// has neither. The cells and headers draw the column dividers themselves
	// (rowBandLayout, sortHeader), as Fyne's would cross every row too.
	t.ShowHeaderColumn = false
	t.HideSeparators = true

	applyColumnWidths()

	// Define custom headers
	t.CreateHeader = func() fyne.CanvasObject {
		return newSortHeader()
	}
	t.UpdateHeader = func(cellID widget.TableCellID, header fyne.CanvasObject) {
		h := header.(*sortHeader)
		h.col = cellID.Col

		if cellID.Col == -1 {
			return
		}
		h.sortDir = 0
		if sortCol == cellID.Col {
			h.sortDir = -1
			if sortAsc {
				h.sortDir = 1
			}
		}
		h.Text = headerTitles[cellID.Col]
		h.Refresh()
	}
	return t
}

// rowBandLayout lays out a cell: the highlight behind it, the content, and the
// column divider at its right. The table leaves its padding between cells, so
// the highlight grows by half of it on every side, and the highlights of a
// row's cells meet in one unbroken band as a selected row in a GTK list does.
// The divider sits in the middle of the gap to the next column and grows the
// same way, so the dividers of a column's cells join into one line down the
// list.
type rowBandLayout struct{}

func (rowBandLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	half := theme.Padding() / 2
	objects[0].Move(fyne.NewPos(-half, -half))
	objects[0].Resize(size.AddWidthHeight(2*half, 2*half))
	objects[1].Move(fyne.NewPos(0, 0))
	objects[1].Resize(size)
	line := theme.SeparatorThicknessSize()
	objects[2].Move(fyne.NewPos(size.Width+half-line/2, -half))
	objects[2].Resize(fyne.NewSize(line, size.Height+2*half))
}

func (rowBandLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return objects[1].MinSize()
}

var headerTitles = [4]string{"Name", "Path", "Size", "Last Modified"}

// sortHeader is a column header that sorts the results when tapped. Like a
// GTK column header, it shows the sort direction as a triangle at its right
// edge, and draws a divider there and rules along its top and bottom.
type sortHeader struct {
	widget.Label
	col int
	// sortDir is 1 when the results are sorted ascending (A to Z, smallest
	// or oldest first) by this column, -1 when descending, and 0 when they
	// are sorted by another.
	sortDir int
}

func newSortHeader() *sortHeader {
	h := &sortHeader{col: -1}
	h.ExtendBaseWidget(h)
	h.Truncation = fyne.TextTruncateEllipsis
	return h
}

func (h *sortHeader) CreateRenderer() fyne.WidgetRenderer {
	sep := theme.Color(colorNameGrid)
	r := &sortHeaderRenderer{
		WidgetRenderer: h.Label.CreateRenderer(),
		h:              h,
		arrow:          canvas.NewImageFromResource(nil),
		divider:        canvas.NewRectangle(sep),
		top:            canvas.NewRectangle(sep),
		bottom:         canvas.NewRectangle(sep),
	}
	r.arrow.FillMode = canvas.ImageFillContain
	r.updateArrow()
	return r
}

type sortHeaderRenderer struct {
	fyne.WidgetRenderer // the label's
	h                   *sortHeader
	arrow               *canvas.Image
	divider, top        *canvas.Rectangle
	bottom              *canvas.Rectangle
}

// Layout gives the title the width the arrow leaves, puts the arrow at the
// right edge, the divider in the middle of the gap the table leaves between
// columns, and runs the rules under the gap too so they are unbroken.
func (r *sortHeaderRenderer) Layout(size fyne.Size) {
	pad := theme.Padding()
	icon := theme.IconInlineSize()
	title := size
	if r.h.sortDir != 0 {
		title.Width = max(size.Width-icon-pad, 0)
	}
	r.WidgetRenderer.Layout(title)
	r.arrow.Move(fyne.NewPos(size.Width-icon-pad, (size.Height-icon)/2))
	r.arrow.Resize(fyne.NewSquareSize(icon))

	line := theme.SeparatorThicknessSize()
	r.divider.Move(fyne.NewPos(size.Width+(pad-line)/2, 0))
	r.divider.Resize(fyne.NewSize(line, size.Height))
	r.top.Move(fyne.NewPos(0, 0))
	r.top.Resize(fyne.NewSize(size.Width+pad, line))
	r.bottom.Move(fyne.NewPos(0, size.Height-line))
	r.bottom.Resize(fyne.NewSize(size.Width+pad, line))
}

func (r *sortHeaderRenderer) MinSize() fyne.Size {
	return r.WidgetRenderer.MinSize().AddWidthHeight(theme.IconInlineSize()+theme.Padding(), 0)
}

func (r *sortHeaderRenderer) Objects() []fyne.CanvasObject {
	return append(r.WidgetRenderer.Objects(), r.arrow, r.divider, r.top, r.bottom)
}

// sortUpIcon and sortDownIcon are the sort triangles: solid, 10 by 5 in a 16
// square, as GTK draws them. Fyne's menu arrows are smaller and outlined.
// Themed, so they take the text colour.
var (
	sortUpIcon = theme.NewThemedResource(fyne.NewStaticResource("sort-up.svg", []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M3 10.5h10l-5-5z"/></svg>`)))
	sortDownIcon = theme.NewThemedResource(fyne.NewStaticResource("sort-down.svg", []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path d="M3 5.5h10l-5 5z"/></svg>`)))
)

// updateArrow shows the triangle for the header's sort direction, or none.
// As in GTK, ascending points down and descending up.
func (r *sortHeaderRenderer) updateArrow() {
	switch r.h.sortDir {
	case 1:
		r.arrow.Resource = sortDownIcon
	case -1:
		r.arrow.Resource = sortUpIcon
	default:
		r.arrow.Resource = nil
	}
	r.arrow.Hidden = r.h.sortDir == 0
}

func (r *sortHeaderRenderer) Refresh() {
	r.updateArrow()
	r.arrow.Refresh()
	sep := theme.Color(colorNameGrid)
	for _, l := range []*canvas.Rectangle{r.divider, r.top, r.bottom} {
		l.FillColor = sep
		l.Refresh()
	}
	r.WidgetRenderer.Refresh()
	r.Layout(r.h.Size())
}

func (h *sortHeader) Tapped(_ *fyne.PointEvent) {
	if h.col < 0 {
		return
	}
	if sortCol == h.col {
		sortAsc = !sortAsc
	} else {
		sortCol = h.col
		sortAsc = true
	}
	saveLayout()

	sortRows(tableData)
	// row indexes no longer point at the same files
	selectedRow = -1
	hoveredRow = -1
	t.Refresh()
	t.ScrollToTop()
}

func showAbout() {
	w := fyne.CurrentApp().NewWindow("About")

	rich := widget.NewRichTextFromMarkdown(`
# EverythingX 

**Version:** ` + version.ShortInfo() + `

**Author:** AlanKK

More info on [Github](` + GithubURL + `)

Report issues to [GitHub Issues](https://github.com/AlanKK/everythingx/issues)
`)

	img := canvas.NewImageFromResource(logoResource())
	img.SetMinSize(fyne.NewSize(128, 128))
	imgContainer := container.NewCenter(img)

	okButton := container.NewVBox(container.NewCenter(
		&widget.Button{
			Text:     "OK",
			OnTapped: w.Hide}))
	text := container.NewCenter(rich)
	content := container.NewBorder(imgContainer, okButton, nil, nil, text)

	w.SetContent(content)
	w.Resize(fyne.NewSize(400, 300))
	w.Show()
}

// statusBar shows the result count, or a passing message, at the left of the
// bottom bar. indexStatus shows the size of the index at the right.
var statusBar *widget.Label
var indexStatus *widget.Label

// logoResource is the app logo that reads on the current background: white
// and orange on dark, orange and black on light.
func logoResource() fyne.Resource {
	if appTheme.Variant() == theme.VariantDark {
		return resourceFolderWhiteOrange5122xPng
	}
	return resourceFolderOrangeBlack5122xPng
}

// emptyLogo is the logo on the empty state. It is swapped when the theme
// changes between light and dark (see loadUI).
var emptyLogo *canvas.Image

// makeEmptyState builds the logo and prompt shown when there is nothing to
// list, so the window never opens blank. Hidden until updateEmptyState decides.
func makeEmptyState() *fyne.Container {
	img := canvas.NewImageFromResource(logoResource())
	emptyLogo = img
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(128, 128))

	prompt := widget.NewLabelWithStyle("Type to search", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	indexLabel = widget.NewLabelWithStyle("Counting indexed files…", fyne.TextAlignCenter, fyne.TextStyle{})

	emptyState = container.NewCenter(container.NewVBox(img, prompt, indexLabel))
	emptyState.Hide()
	return emptyState
}

func loadUI(db *sql.DB, instanceLn net.Listener) {
	// Declare the app identity and the fyne.Do migration in code rather than in
	// a FyneApp.toml: `make app` runs `fyne package -executable`, which wraps the
	// already-built binary, so TOML metadata is never compiled in and is not
	// readable from inside the .app bundle. Without an ID, Fyne invents a new
	// throwaway one on every launch (breaking the Preferences/Storage APIs).
	//
	// Migrations["fyneDo"] tells Fyne that every UI mutation from a goroutine
	// already goes through fyne.Do. Keep it that way when adding UI code, as
	// this also switches off Fyne's wrong-thread runtime warnings.
	app.SetMetadata(fyne.AppMetadata{
		ID:         AppID,
		Name:       AppName,
		Version:    version.ShortInfo(),
		Migrations: map[string]bool{"fyneDo": true},
	})

	a := app.NewWithID(AppID)
	// The menu bar tray falls back to the app icon, and to a broken-image glyph
	// when there is none. Metadata compiled by "fyne package -executable" does
	// not carry one, so set it here.
	a.SetIcon(resourceFolderWhiteOrange5122xPng)
	prefs = loadPrefs(a.Preferences())
	sortCol, sortAsc = defaultPrefs().SortCol, defaultPrefs().SortAsc
	if prefs.RememberLayout {
		sortCol, sortAsc = prefs.SortCol, prefs.SortAsc
	}
	loadSystemFonts()
	appTheme.setPrefs(prefs)
	a.Settings().SetTheme(appTheme)
	w := a.NewWindow(AppName)
	mainWindow = w

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("EverythingX",
			fyne.NewMenuItem("Show EverythingX", func() {
				w.Show()
			}),
			fyne.NewMenuItem(preferencesLabel, showPreferences),
			fyne.NewMenuItem("About...", func() {
				showAbout()
			}))
		desk.SetSystemTrayMenu(m)
	}

	// Hiding the window is the last chance to store column widths when the
	// app is then ended from outside (logout, kill), which runs no OnStopped.
	hideWindow := func() {
		saveLayout()
		w.Hide()
	}
	w.SetCloseIntercept(hideWindow)

	prefsItem := fyne.NewMenuItem(preferencesLabel, showPreferences)
	prefsItem.Shortcut = preferencesShortcut
	w.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File",
			prefsItem,
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Close Window", hideWindow)),
		fyne.NewMenu("Help", fyne.NewMenuItem("About EverythingX", showAbout)),
	))
	// The macOS menu bar handles its own shortcuts; elsewhere, menu shortcuts
	// are only drawn in the menu, so the window has to listen for it too.
	if runtime.GOOS != "darwin" {
		w.Canvas().AddShortcut(preferencesShortcut, func(fyne.Shortcut) { showPreferences() })
	}

	table := makeTable()

	entry := newSearchEntry()
	searchBox = entry

	statusBar = widget.NewLabel("")
	statusBar.Truncation = fyne.TextTruncateEllipsis
	indexStatus = widget.NewLabel("")

	// Debounce keystrokes: only search once typing pauses, so fast typing
	// doesn't fire a DB query per character. Runs on the UI thread.
	entry.OnChanged = func(s string) {
		entry.showClear()
		if searchDebounceTimer != nil {
			searchDebounceTimer.Stop()
		}
		if len(s) == 0 {
			// Show the default view immediately; no need to wait for the
			// debounce.
			showDefaultView(table, statusBar)
			return
		}
		opts := prefs.searchOptions()
		searchDebounceTimer = time.AfterFunc(searchDebounce, func() {
			handleAutoCompleteEntryChanged(s, opts, table, statusBar)
		})
	}

	content := container.NewBorder(
		container.NewPadded(entry),
		container.NewVBox(widget.NewSeparator(),
			container.NewBorder(nil, nil, nil, indexStatus, statusBar)),
		nil,
		nil,
		container.NewStack(table, makeEmptyState()),
	)

	w.SetContent(fynetooltip.AddWindowToolTipLayer(content, w.Canvas()))
	w.Canvas().Focus(entry)
	w.Resize(fyne.NewSize(1300, 800))

	// Bring the window forward when the user launches the app again. Show
	// restores a window hidden to the tray; RequestFocus raises and
	// un-minimizes it on X11 and macOS. On Wayland Fyne can't request focus,
	// so the compositor may leave a minimized window where it is.
	showWindow := func() {
		fyne.Do(func() {
			w.Show()
			w.RequestFocus()
			w.Canvas().Focus(entry)
		})
	}
	// A second process (Linux, or the macOS binary run directly) hands off
	// over the single-instance socket.
	if instanceLn != nil {
		serveInstance(instanceLn, showWindow)
	}
	a.Lifecycle().SetOnStopped(func() {
		saveLayout()
		if instanceLn != nil {
			instanceLn.Close()
		}
	})

	// A switch between light and dark, from the Appearance tab or the
	// desktop, needs the logo that reads on the new background.
	a.Settings().AddListener(func(fyne.Settings) {
		fyne.Do(func() {
			if res := logoResource(); emptyLogo != nil && emptyLogo.Resource != res {
				emptyLogo.Resource = res
				emptyLogo.Refresh()
			}
		})
	})
	// Reopening the macOS .app doesn't start a second process; see watchReopen.
	a.Lifecycle().SetOnStarted(func() { watchReopen(showWindow) })

	showDefaultView(table, statusBar)
	loadIndexCount(db)

	shared.PrintMemUsage()
	w.ShowAndRun()
}
