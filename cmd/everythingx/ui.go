package main

import (
	"cmp"
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
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

// clearResults empties the table and resets the status bar. Must run on the UI
// thread.
func clearResults(t *widget.Table, statusBar *widget.Label) {
	searchCounter.Add(1) // invalidate any in-flight search
	tableData = nil
	lastResultText = "0 objects"
	selectedRow = -1
	t.Refresh()
	t.ScrollToTop()
	statusBar.SetText(lastResultText)
}

// handleAutoCompleteEntryChanged runs the search off the UI thread so that
// scrolling and other interactions remain responsive while the DB query and
// per-file stat calls execute.
func handleAutoCompleteEntryChanged(searchText string, t *widget.Table, statusBar *widget.Label) {
	if len(searchText) == 0 {
		fyne.Do(func() { clearResults(t, statusBar) })
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

		newData := make([]RowData, 0, len(results))
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

			newData = append(newData, RowData{
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
			// Re-check on the UI thread: a newer search may have started
			// between the check above and this closure running. Without this,
			// an older/slower search could overwrite fresher results.
			if searchCounter.Load() != myID {
				return
			}
			tableData = newData
			sortRows(tableData) // keep the chosen column ordering as results stream in
			lastResultText = resultText
			selectedRow = -1
			t.Refresh()
			t.ScrollToTop() // reset scroll so new results start at the top
			statusBar.SetText(resultText)
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
	path := c.path
	go func() {
		tip := getToolTipForFile(path)
		fyne.Do(func() {
			if !c.hovered || c.path != path {
				return
			}
			c.SetToolTip(tip)
			c.RichText.MouseIn(e)
		})
	}()
}

func (c *tooltipCell) MouseOut() {
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
	c.MouseOut()

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
				richText.MouseOut()
				richText.path = row.SearchResult.Fullpath
			}
			richText.col = id.Col
			richText.row = id.Row

			// Only repaint what changed: this runs for every visible cell on
			// each refresh, so unconditional work here is felt as click lag.
			var highlight color.Color = color.Transparent
			if id.Row == selectedRow {
				highlight = theme.Color(theme.ColorNameSelection)
			}
			if bg.FillColor != highlight {
				bg.FillColor = highlight
				bg.Refresh()
			}

			switch id.Col {
			case 0:
				fileNameParts := row.Name
				var segments []widget.RichTextSegment
				if fileNameParts[0] != "" {
					segments = append(segments, &widget.TextSegment{Text: fileNameParts[0],
						Style: widget.RichTextStyle{
							Inline:    true,
							TextStyle: fyne.TextStyle{Monospace: true},
						},
					})
				}
				if fileNameParts[1] != "" {
					segments = append(segments, &widget.TextSegment{Text: fileNameParts[1],
						Style: widget.RichTextStyle{
							Inline:    true,
							TextStyle: fyne.TextStyle{Bold: true},
							ColorName: theme.ColorNameWarning,
						},
					})
				}
				if fileNameParts[2] != "" {
					segments = append(segments, &widget.TextSegment{Text: fileNameParts[2],
						Style: widget.RichTextStyle{
							Inline:    true,
							TextStyle: fyne.TextStyle{Monospace: true},
						},
					})
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

	t.SetColumnWidth(0, 400) // Name
	t.SetColumnWidth(1, 600) // Path
	t.SetColumnWidth(2, 70)  // Size
	t.SetColumnWidth(3, 190) // Last modified

	// Define custom headers
	t.CreateHeader = func() fyne.CanvasObject {
		// The trailing rule marks where a column can be grabbed to resize it.
		// Raising the theme's separator thickness would instead rule every row.
		grip := canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))
		grip.SetMinSize(fyne.NewSize(1, 0))
		return container.NewBorder(nil, nil, nil, grip, newSortHeader())
	}
	t.UpdateHeader = func(cellID widget.TableCellID, header fyne.CanvasObject) {
		box := header.(*fyne.Container)
		h := box.Objects[0].(*sortHeader)
		grip := box.Objects[1].(*canvas.Rectangle)
		h.col = cellID.Col

		// The blank row-header column has no width to resize.
		if showGrip := cellID.Col != -1; showGrip != grip.Visible() {
			if showGrip {
				grip.Show()
			} else {
				grip.Hide()
			}
			box.Refresh()
		}

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
	selectedRow = -1 // row indexes no longer point at the same files
	t.Refresh()
	t.ScrollToTop()
}

func getToolTipForFile(path string) string {
	lsFormat, err := shared.GetFileInfo(path)
	if err != nil {
		return "No access"
	}

	return fmt.Sprintf("%s\n\n%s\n", path, lsFormat)
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

func loadUI() {
	// Declare the app identity and the fyne.Do migration in code rather than in
	// a FyneApp.toml: `make app` runs `fyne package -executable`, which wraps the
	// already-built binary, so TOML metadata is never compiled in and is not
	// readable from inside the .app bundle. Without an ID, Fyne invents a new
	// throwaway one on every launch (breaking the Preferences/Storage APIs).
	//
	// Migrations["fyneDo"] tells Fyne that every UI mutation from a goroutine
	// already goes through fyne.Do — keep it that way when adding UI code, as
	// this also switches off Fyne's wrong-thread runtime warnings.
	app.SetMetadata(fyne.AppMetadata{
		ID:         AppID,
		Name:       AppName,
		Version:    version.ShortInfo(),
		Migrations: map[string]bool{"fyneDo": true},
	})

	a := app.NewWithID(AppID)
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

	statusBar = widget.NewLabel("0 objects")
	statusBar.TextStyle = fyne.TextStyle{Bold: true}

	// Debounce keystrokes: only search once typing pauses, so fast typing
	// doesn't fire a DB query per character. Runs on the UI thread.
	entry.OnChanged = func(s string) {
		if searchDebounceTimer != nil {
			searchDebounceTimer.Stop()
		}
		if len(s) == 0 {
			// Clear immediately; no need to wait for the debounce.
			clearResults(table, statusBar)
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
		table,
	)

	w.SetContent(fynetooltip.AddWindowToolTipLayer(content, w.Canvas()))
	w.Canvas().Focus(entry)
	w.Resize(fyne.NewSize(1300, 800))

	shared.PrintMemUsage()
	w.ShowAndRun()
}
