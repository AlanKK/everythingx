package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
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

const maxSearchResults int = 1000

// searchDebounce is how long to wait after the last keystroke before running a
// search, so fast typing doesn't spawn a search (and DB query) per character.
const searchDebounce = 120 * time.Millisecond

type RowData struct {
	Name         []string // before/match/after, for highlighting the search term
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
// directory listing) rather than search results. UI thread only.
var showingDefault bool

// defaultStatus is the status bar text for the default view.
func defaultStatus(items int) string {
	text := fmt.Sprintf("%d items in ~", items)
	if indexedText != "" {
		text += " · " + indexedText
	}
	return text
}

// showDefaultView lists the top level of the home directory, which is what the
// window shows whenever the search box is empty. The listing runs off the UI
// thread and, like a search, is dropped if a search starts before it lands.
// Must run on the UI thread.
func showDefaultView(t *widget.Table, statusBar *widget.Label) {
	myID := searchCounter.Add(1) // invalidate any in-flight search

	go func() {
		var rows []RowData
		if home, err := os.UserHomeDir(); err == nil {
			results, err := homeResults(home)
			if err != nil {
				fmt.Println("Error listing home directory:", err)
			}
			rows = buildRows(results, "")
		}

		fyne.Do(func() {
			applyRows(myID, rows, defaultStatus(len(rows)), true, t, statusBar)
		})
	}()
}

// homeResults lists dir's own entries, not recursively, as search results:
// directories first, then by name ignoring case. Hidden entries are skipped.
func homeResults(dir string) ([]*shared.SearchResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	results := make([]*shared.SearchResult, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		objType := shared.ItemIsFile
		switch {
		case e.IsDir():
			objType = shared.ItemIsDir
		case e.Type()&fs.ModeSymlink != 0:
			objType = shared.ItemIsSymlink
		}
		results = append(results, &shared.SearchResult{
			Fullpath:   filepath.Join(dir, e.Name()),
			ObjectType: objType,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		iDir := results[i].ObjectType == shared.ItemIsDir
		jDir := results[j].ObjectType == shared.ItemIsDir
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(results[i].Fullpath) < strings.ToLower(results[j].Fullpath)
	})
	return results, nil
}

// buildRows stats each result and splits its name around searchText for
// highlighting. An empty searchText highlights nothing. Does file I/O, so run
// it off the UI thread.
func buildRows(results []*shared.SearchResult, searchText string) []RowData {
	rows := make([]RowData, 0, len(results))
	for _, r := range results {
		fullpath := r.Fullpath
		base := filepath.Base(r.Fullpath)
		dir := filepath.Dir(fullpath) + "/"
		size, modified, sizeBytes, modTime := shared.GetFileSizeMod(fullpath)

		if r.ObjectType == shared.ItemIsDir {
			base += "/"
			fullpath += "/"
			size = "--"
			sizeBytes = -1
		}

		beforeTerm, searchTerm, afterTerm := shared.SplitFileName(base, searchText)

		rows = append(rows, RowData{
			Name:         []string{beforeTerm, searchTerm, afterTerm},
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
			if showingDefault {
				lastResultText = defaultStatus(len(tableData))
				statusBar.SetText(lastResultText)
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
// per-file stat calls execute.
func handleAutoCompleteEntryChanged(searchText string, t *widget.Table, statusBar *widget.Label) {
	if len(searchText) == 0 {
		fyne.Do(func() { showDefaultView(t, statusBar) })
		return
	}

	myID := searchCounter.Add(1)

	go func() {
		start := time.Now()

		searchStart := time.Now()
		results, err := ffdb.PrefixSearch(searchText, maxSearchResults)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		searchElapsed := time.Since(searchStart)

		if searchCounter.Load() != myID {
			return
		}

		newData := buildRows(results, searchText)

		if searchCounter.Load() != myID {
			return
		}

		var resultText string
		if len(results) == maxSearchResults {
			resultText = fmt.Sprintf("Showing first %d objects", maxSearchResults)
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

// tooltipCell is a RichText cell that lazily fetches file info on hover.
type tooltipCell struct {
	ttwidget.RichText
	path    string
	col     int
	row     int
	hovered bool
}

func newTooltipCell() *tooltipCell {
	c := &tooltipCell{row: -1}
	c.Scroll = container.ScrollNone
	c.Truncation = fyne.TextTruncateEllipsis
	c.RichText.ExtendBaseWidget(c)
	return c
}

func (c *tooltipCell) MouseIn(e *desktop.MouseEvent) {
	// getToolTipForFile does blocking file I/O (os.Stat + user/group lookups).
	// Run it off the UI thread so hovering over rows never stalls the UI, then
	// set the text and start the hover timer together on the UI thread. Arming
	// the timer first would show whatever the previously hovered row left behind,
	// since cells are recycled.
	c.hovered = true
	setHoveredRow(c.row)
	path := c.path
	maxChars := tooltipMaxChars() // read window width on the UI thread
	go func() {
		tip := getToolTipForFile(path, maxChars)
		fyne.Do(func() {
			if !c.hovered || c.path != path {
				return
			}
			c.SetToolTip(tip)
			c.RichText.MouseIn(e)
		})
	}()
}

// MouseOut fires when the pointer leaves this cell, including on the way out of
// the table entirely, so it is where the row highlight is dropped. Moving to
// another cell clears and re-sets the highlight before the next paint, so a
// sweep across a row does not flicker.
func (c *tooltipCell) MouseOut() {
	setHoveredRow(-1)
	c.cancelToolTip()
}

// cancelToolTip drops an armed tool tip without touching the row highlight. The
// widget itself calls this when a cell is recycled or covered by a menu, where
// the pointer has not actually left the row.
func (c *tooltipCell) cancelToolTip() {
	c.hovered = false
	c.RichText.MouseOut()
}

// flashStatus shows a transient message in the status bar, then restores the
// result count. Must be called on the UI thread.
func flashStatus(msg string) {
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
func (c *tooltipCell) MouseDown(e *desktop.MouseEvent) {
	if e.Button == desktop.MouseButtonPrimary {
		c.selectRow()
	}
}

func (c *tooltipCell) MouseUp(_ *desktop.MouseEvent) {}

// DoubleTapped reveals the file in the system file manager.
func (c *tooltipCell) DoubleTapped(_ *fyne.PointEvent) {
	revealFile(c.path)
}

// TappedSecondary opens the row's context menu.
func (c *tooltipCell) TappedSecondary(e *fyne.PointEvent) {
	if c.path == "" {
		return
	}
	c.selectRow()

	// A tool tip draws into the layer on the window content, which the menu
	// overlay covers. Cancel any pending one before the overlay goes up, or it
	// fires under the menu with nowhere to draw.
	c.cancelToolTip()

	path := c.path
	menu := fyne.NewMenu("",
		fyne.NewMenuItem(revealLabel, func() { revealFile(path) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Copy Name", func() { copyToClipboard(filepath.Base(path)) }),
		fyne.NewMenuItem("Copy Full Path", func() { copyToClipboard(path) }),
		fyne.NewMenuItem("Copy Containing Folder", func() { copyToClipboard(filepath.Dir(path)) }),
	)
	widget.NewPopUpMenu(menu, mainWindow.Canvas()).ShowAtPosition(e.AbsolutePosition)
}

func (c *tooltipCell) selectRow() {
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
// It is fed from tooltipCell.MouseIn/MouseOut rather than the table's own
// OnHighlighted callback: the driver delivers hover to a single object, the
// innermost hoverable one, and every data cell is a hoverable tooltipCell. The
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

// revealFile launches the file manager off the UI thread; "open -R" and
// "xdg-open" take long enough to stall a click.
func revealFile(path string) {
	go handleOpenFile(path)
}

// openSelected reveals the highlighted result in the system file manager.
func openSelected() {
	if selectedRow >= 0 && selectedRow < len(tableData) {
		revealFile(tableData[selectedRow].SearchResult.Fullpath)
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
	e.SetPlaceHolder("Enter filename...")
	return e
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

// nameColWidth is the initial width of the Name column in pixels.
const nameColWidth = 400

// nameHighlightMaxChars is roughly how many monospace characters fit in the
// Name column. When the matched term sits beyond this, the Name cell renders as
// a single truncating segment instead of highlighted inline segments, so it
// never overflows into the Path column. Recomputed in makeTable.
var nameHighlightMaxChars = 47

// plainNameSegment builds a non-inline monospace segment. RichText truncates a
// trailing segment like this with an ellipsis when it is too wide to fit.
func plainNameSegment(text string) *widget.TextSegment {
	return &widget.TextSegment{
		Text: text,
		Style: widget.RichTextStyle{
			Alignment: fyne.TextAlignLeading,
			TextStyle: fyne.TextStyle{Monospace: true},
		},
	}
}

// inlineNameSegment builds an inline segment for the highlighted-name path.
// Highlighted segments are bold and use the warning (accent) color.
func inlineNameSegment(text string, highlight bool) *widget.TextSegment {
	style := widget.RichTextStyle{
		Inline:    true,
		TextStyle: fyne.TextStyle{Monospace: true},
	}
	if highlight {
		style.TextStyle.Bold = true
		style.ColorName = theme.ColorNameWarning
	}
	return &widget.TextSegment{Text: text, Style: style}
}

func makeTable() *widget.Table {
	tableData = make([]RowData, 0, maxSearchResults)

	t = widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			// Stack: selection highlight behind, icon + text in front. The icon
			// is only shown for the Name column; a hidden object is skipped by
			// the border layout, so the other columns keep their full width.
			icon := widget.NewIcon(nil)
			icon.Hide()
			return container.NewStack(
				canvas.NewRectangle(color.Transparent),
				container.NewBorder(nil, nil, icon, nil, newTooltipCell()),
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
			richText := inner.Objects[0].(*tooltipCell)
			icon := inner.Objects[1].(*widget.Icon)

			if richText.path != row.SearchResult.Fullpath {
				// Scrolling recycles this cell onto another row without the
				// pointer moving, so no MouseOut arrives. A tip already armed
				// would fire with the old file's details.
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
			var highlight color.Color = color.Transparent
			switch {
			case id.Row == selectedRow:
				highlight = theme.Color(theme.ColorNameSelection)
			case id.Row == hoveredRow:
				highlight = theme.Color(theme.ColorNameHover)
			}
			if bg.FillColor != highlight {
				bg.FillColor = highlight
				bg.Refresh()
			}

			switch id.Col {
			case 0:
				before, term, after := row.Name[0], row.Name[1], row.Name[2]
				var segments []widget.RichTextSegment
				if term == "" || len(before)+len(term) > nameHighlightMaxChars {
					// No match, or the highlighted term sits too far right to
					// fit before the column edge. Render the whole name as a
					// single trailing segment so RichText truncates it with an
					// ellipsis; its ellipsis only applies to the last segment,
					// so a long leading segment would otherwise overflow into
					// the Path column.
					segments = []widget.RichTextSegment{plainNameSegment(before + term + after)}
				} else {
					if before != "" {
						segments = append(segments, inlineNameSegment(before, false))
					}
					segments = append(segments, inlineNameSegment(term, true))
					if after != "" {
						segments = append(segments, inlineNameSegment(after, false))
					}
				}
				richText.Segments = segments
			case 1:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: row.Path,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignLeading,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			case 2:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: row.Size,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignTrailing,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			case 3:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: row.Modified,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignLeading,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			}

			showIcon := id.Col == 0
			if showIcon {
				// SetResource re-rasterises the SVG, so only pay for it when
				// the icon actually changes.
				if res := iconForRow(row.SearchResult); icon.Resource != res {
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

	t.SetColumnWidth(0, nameColWidth) // Name
	t.SetColumnWidth(1, 600)          // Path
	t.SetColumnWidth(2, 70)           // Size
	t.SetColumnWidth(3, 190)          // Last modified

	// Derive how many monospace characters fit in the Name column so the
	// highlighted-name path can fall back to plain truncation when the match
	// is too far right (see UpdateCell, case 0). The file-type icon and its
	// padding take width off the text, so the budget is what is left.
	if charW := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true}).Width; charW > 0 {
		nameHighlightMaxChars = int((nameColWidth - theme.IconInlineSize() - theme.Padding()) / charW)
	}

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
		text := headerTitles[cellID.Col]
		if sortCol == cellID.Col {
			if sortAsc {
				text += " ▲"
			} else {
				text += " ▼"
			}
		}
		h.SetText(text)
	}
	return t
}

var headerTitles = [4]string{"Name", "Path", "Size", "Last Modified"}

// sortHeader is a column header that sorts the results when tapped.
type sortHeader struct {
	widget.Label
	col int
}

func newSortHeader() *sortHeader {
	h := &sortHeader{col: -1}
	h.ExtendBaseWidget(h)
	h.TextStyle = fyne.TextStyle{Bold: true}
	return h
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

	sortRows(tableData)
	// row indexes no longer point at the same files
	selectedRow = -1
	hoveredRow = -1
	t.Refresh()
	t.ScrollToTop()
}

func getToolTipForFile(path string, maxChars int) string {
	info, err := shared.GetFileInfo(path)
	if err != nil {
		return "No access"
	}

	return fmt.Sprintf("%s\n\n%s", wrapPath(path, maxChars), info)
}

// tooltipMaxWidth mirrors the cap fyne-tooltip's internal layer uses when
// sizing the tooltip popup.
const tooltipMaxWidth = 600

// tooltipMaxChars returns how many monospace characters fit across the tooltip
// at the current window width, so long paths can be wrapped to fit rather than
// stretching the popup. Must be called on the UI thread (it reads window size).
func tooltipMaxChars() int {
	width := float32(tooltipMaxWidth)
	if mainWindow != nil {
		if cw := mainWindow.Canvas().Size().Width; cw > 0 && cw < width {
			width = cw
		}
	}
	width -= theme.InnerPadding() * 2

	charW := fyne.MeasureText("M", theme.CaptionTextSize(), fyne.TextStyle{Monospace: true}).Width
	if charW <= 0 {
		return 64
	}
	n := int(width / charW)
	if n < 16 {
		n = 16
	}
	return n
}

// wrapPath breaks a long path across multiple lines so it fits within maxChars
// columns, preferring to break just after a path separator for readability and
// falling back to a hard break for very long unbroken segments.
func wrapPath(path string, maxChars int) string {
	if maxChars < 8 {
		maxChars = 8
	}

	var b strings.Builder
	for len(path) > maxChars {
		cut := strings.LastIndex(path[:maxChars], "/")
		if cut <= 0 {
			cut = maxChars // no separator to break on; hard-break
		} else {
			cut++ // keep the separator at the end of the current line
		}
		b.WriteString(path[:cut])
		b.WriteByte('\n')
		path = path[cut:]
	}
	b.WriteString(path)
	return b.String()
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

	var img *canvas.Image
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		img = canvas.NewImageFromResource(resourceFolderWhiteOrange5122xPng) // Orange white 512
	} else {
		img = canvas.NewImageFromResource(resourceFolderOrangeBlack5122xPng) // orange black 512
	}
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

var statusBar *widget.Label

// makeEmptyState builds the logo and prompt shown when there is nothing to
// list, so the window never opens blank. Hidden until updateEmptyState decides.
func makeEmptyState() *fyne.Container {
	var img *canvas.Image
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		img = canvas.NewImageFromResource(resourceFolderWhiteOrange5122xPng)
	} else {
		img = canvas.NewImageFromResource(resourceFolderOrangeBlack5122xPng)
	}
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(128, 128))

	prompt := widget.NewLabelWithStyle("Type to search", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	indexLabel = widget.NewLabelWithStyle("Counting indexed files…", fyne.TextAlignCenter, fyne.TextStyle{})

	emptyState = container.NewCenter(container.NewVBox(img, prompt, indexLabel))
	emptyState.Hide()
	return emptyState
}

func loadUI(db *sql.DB) {
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
	a.Settings().SetTheme(&everythingxTheme{})
	w := a.NewWindow(AppName)
	mainWindow = w

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("EverythingX",
			fyne.NewMenuItem("Show EverythingX", func() {
				w.Show()
			}),
			fyne.NewMenuItem("About...", func() {
				showAbout()
			}))
		desk.SetSystemTrayMenu(m)
	}

	w.SetCloseIntercept(func() {
		w.Hide()
	})

	w.SetMainMenu(fyne.NewMainMenu(
		fyne.NewMenu("File", fyne.NewMenuItem("Close Window", w.Hide)),
		fyne.NewMenu("Help", fyne.NewMenuItem("About EverythingX", showAbout)),
	))

	table := makeTable()

	entry := newSearchEntry()

	statusBar = widget.NewLabel("")
	statusBar.TextStyle = fyne.TextStyle{Bold: true}

	// Debounce keystrokes: only search once typing pauses, so fast typing
	// doesn't fire a DB query per character. Runs on the UI thread.
	entry.OnChanged = func(s string) {
		if searchDebounceTimer != nil {
			searchDebounceTimer.Stop()
		}
		if len(s) == 0 {
			// Show the default view immediately; no need to wait for the
			// debounce.
			showDefaultView(table, statusBar)
			return
		}
		searchDebounceTimer = time.AfterFunc(searchDebounce, func() {
			handleAutoCompleteEntryChanged(s, table, statusBar)
		})
	}

	content := container.NewBorder(
		entry,
		statusBar,
		nil,
		nil,
		container.NewStack(table, makeEmptyState()),
	)

	w.SetContent(fynetooltip.AddWindowToolTipLayer(content, w.Canvas()))
	w.Canvas().Focus(entry)
	w.Resize(fyne.NewSize(1300, 800))

	showDefaultView(table, statusBar)
	loadIndexCount(db)

	shared.PrintMemUsage()
	w.ShowAndRun()
}
