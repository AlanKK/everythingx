package main

import (
	"testing"
	"time"

	"github.com/AlanKK/everythingx/internal/shared"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestMain(m *testing.M) {
	test.NewApp() // icon and colour lookups need an app with a theme
	m.Run()
}

func TestIconForRow(t *testing.T) {
	tests := []struct {
		name     string
		result   shared.SearchResult
		wantIcon fyne.Resource
	}{
		{"directory", shared.SearchResult{Fullpath: "/tmp/stuff", ObjectType: shared.ItemIsDir}, theme.FolderIcon()},
		{"image", shared.SearchResult{Fullpath: "/tmp/a.PNG", ObjectType: shared.ItemIsFile}, theme.FileImageIcon()},
		{"source file", shared.SearchResult{Fullpath: "/tmp/main.go", ObjectType: shared.ItemIsFile}, theme.FileTextIcon()},
		{"audio", shared.SearchResult{Fullpath: "/tmp/song.mp3", ObjectType: shared.ItemIsFile}, theme.FileAudioIcon()},
		{"archive", shared.SearchResult{Fullpath: "/tmp/x.tar.gz", ObjectType: shared.ItemIsFile}, theme.FileApplicationIcon()},
		{"unknown extension", shared.SearchResult{Fullpath: "/tmp/x.qqq", ObjectType: shared.ItemIsFile}, theme.FileIcon()},
		{"no extension", shared.SearchResult{Fullpath: "/tmp/LICENSE", ObjectType: shared.ItemIsFile}, theme.FileIcon()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := iconForRow(&tc.result)
			if got.Name() != tc.wantIcon.Name() {
				t.Errorf("iconForRow(%s) = %s, want %s", tc.result.Fullpath, got.Name(), tc.wantIcon.Name())
			}
		})
	}
}

// A directory and a plain file must not share an icon, otherwise the column
// conveys nothing.
func TestIconForRowDistinguishesDirsFromFiles(t *testing.T) {
	dir := iconForRow(&shared.SearchResult{Fullpath: "/tmp/x", ObjectType: shared.ItemIsDir})
	file := iconForRow(&shared.SearchResult{Fullpath: "/tmp/x", ObjectType: shared.ItemIsFile})
	if dir.Name() == file.Name() {
		t.Errorf("directory and file share icon %s", dir.Name())
	}
}

func row(base string, size int64, mod time.Time) RowData {
	return RowData{Base: base, SizeBytes: size, ModTime: mod}
}

func baseNames(rows []RowData) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Base
	}
	return names
}

func assertOrder(t *testing.T, rows []RowData, want ...string) {
	t.Helper()
	got := baseNames(rows)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func setSort(col int, asc bool) func() {
	sortCol, sortAsc = col, asc
	return func() { sortCol, sortAsc = -1, true }
}

// Size must order by actual bytes: formatted "3.4M" sorts before "155.9K" as a
// string, which is the wrong answer.
func TestSortRowsBySize(t *testing.T) {
	defer setSort(2, true)()

	rows := []RowData{
		row("big", 3_565_158, time.Time{}),
		row("small", 159_641, time.Time{}),
		row("folder", -1, time.Time{}),
	}
	sortRows(rows)

	assertOrder(t, rows, "folder", "small", "big")
}

// Last Modified must order by real time, not by the "Jan 2 2006 15:04" string.
func TestSortRowsByModified(t *testing.T) {
	defer setSort(3, true)()

	apr := time.Date(2023, time.April, 27, 13, 15, 0, 0, time.UTC)
	aug := time.Date(2025, time.August, 7, 22, 16, 0, 0, time.UTC)
	feb := time.Date(2026, time.February, 21, 10, 55, 0, 0, time.UTC)

	rows := []RowData{row("feb2026", 0, feb), row("apr2023", 0, apr), row("aug2025", 0, aug)}
	sortRows(rows)

	assertOrder(t, rows, "apr2023", "aug2025", "feb2026")
}

func TestSortRowsByNameIgnoresCase(t *testing.T) {
	defer setSort(0, true)()

	rows := []RowData{row("Zebra", 0, time.Time{}), row("apple", 0, time.Time{}), row("Mango", 0, time.Time{})}
	sortRows(rows)

	assertOrder(t, rows, "apple", "Mango", "Zebra")
}

func TestSortRowsDescendingReversesOrder(t *testing.T) {
	defer setSort(2, false)()

	rows := []RowData{row("small", 10, time.Time{}), row("big", 900, time.Time{}), row("mid", 100, time.Time{})}
	sortRows(rows)

	assertOrder(t, rows, "big", "mid", "small")
}

// With no column chosen the database ordering must survive untouched.
func TestSortRowsUnsortedKeepsDatabaseOrder(t *testing.T) {
	defer setSort(-1, true)()

	rows := []RowData{row("c", 3, time.Time{}), row("a", 1, time.Time{}), row("b", 2, time.Time{})}
	sortRows(rows)

	assertOrder(t, rows, "c", "a", "b")
}

// The table cell is a nested container; UpdateCell reaches into it by index, so
// a layout change must not silently break icon or highlight rendering.
func TestUpdateCellRendersIconAndSelection(t *testing.T) {
	table := makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	tableData = []RowData{{
		Name:         []string{"", "report", ".pdf"},
		Base:         "report.pdf",
		Path:         "/tmp/",
		Size:         "1.0K",
		Modified:     "Jan 2 2026 10:00",
		SearchResult: &shared.SearchResult{Fullpath: "/tmp/report.pdf", ObjectType: shared.ItemIsFile},
	}}
	selectedRow = 0

	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)

	stack := cell.(*fyne.Container)
	bg := stack.Objects[0].(*canvas.Rectangle)
	inner := stack.Objects[1].(*fyne.Container)
	text := inner.Objects[0].(*tooltipCell)
	icon := inner.Objects[1].(*widget.Icon)

	if !icon.Visible() {
		t.Error("icon hidden in the Name column")
	}
	if icon.Resource == nil {
		t.Error("icon has no resource")
	}
	if text.path != "/tmp/report.pdf" {
		t.Errorf("cell path = %q, want /tmp/report.pdf", text.path)
	}
	if text.row != 0 {
		t.Errorf("cell row = %d, want 0", text.row)
	}
	if bg.FillColor == nil {
		t.Error("selected row has no highlight colour")
	}

	// Other columns are text only, and must not reserve width for an icon.
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 1}, cell)
	if icon.Visible() {
		t.Error("icon shown outside the Name column")
	}
}

// Headers carry the sort indicator and the resize rule, and UpdateHeader
// reaches into a nested container by index to reach both.
func TestUpdateHeaderShowsSortAndGrip(t *testing.T) {
	table := makeTable()
	defer func() { sortCol, sortAsc = -1, true }()

	sortCol, sortAsc = 2, true
	header := table.CreateHeader()
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 2}, header)

	box := header.(*fyne.Container)
	label := box.Objects[0].(*sortHeader)
	grip := box.Objects[1].(*canvas.Rectangle)

	if label.Text != "Size ▲" {
		t.Errorf("header text = %q, want \"Size ▲\"", label.Text)
	}
	if !grip.Visible() {
		t.Error("resize rule hidden on a data column")
	}

	sortAsc = false
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 2}, header)
	if label.Text != "Size ▼" {
		t.Errorf("header text = %q, want \"Size ▼\"", label.Text)
	}

	// A column that is not sorted carries no arrow.
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: 0}, header)
	if label.Text != "Name" {
		t.Errorf("unsorted header = %q, want \"Name\"", label.Text)
	}

	// The blank row-header column has no width to resize.
	table.UpdateHeader(widget.TableCellID{Row: -1, Col: -1}, header)
	if grip.Visible() {
		t.Error("resize rule shown on the row header")
	}
}

// Tapping the active column reverses it; tapping a new one starts ascending.
func TestSortHeaderTapTogglesDirection(t *testing.T) {
	makeTable()
	defer func() { sortCol, sortAsc, tableData = -1, true, nil }()

	tableData = []RowData{{Base: "a", SearchResult: &shared.SearchResult{Fullpath: "/a"}}}
	h := newSortHeader()
	h.col = 1

	h.Tapped(nil)
	if sortCol != 1 || !sortAsc {
		t.Fatalf("first tap: col=%d asc=%v, want col=1 asc=true", sortCol, sortAsc)
	}

	h.Tapped(nil)
	if sortCol != 1 || sortAsc {
		t.Fatalf("second tap: col=%d asc=%v, want col=1 asc=false", sortCol, sortAsc)
	}

	other := newSortHeader()
	other.col = 3
	other.Tapped(nil)
	if sortCol != 3 || !sortAsc {
		t.Fatalf("new column: col=%d asc=%v, want col=3 asc=true", sortCol, sortAsc)
	}
}

// An unselected row must not paint a highlight over its neighbours.
func TestUpdateCellClearsHighlightOnUnselectedRows(t *testing.T) {
	table := makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	result := &shared.SearchResult{Fullpath: "/tmp/a", ObjectType: shared.ItemIsFile}
	tableData = []RowData{
		{Name: []string{"a", "", ""}, Base: "a", SearchResult: result},
		{Name: []string{"b", "", ""}, Base: "b", SearchResult: result},
	}
	selectedRow = 0

	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	selected := cell.(*fyne.Container).Objects[0].(*canvas.Rectangle).FillColor

	table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)
	unselected := cell.(*fyne.Container).Objects[0].(*canvas.Rectangle).FillColor

	if selected == unselected {
		t.Error("selected and unselected rows painted the same")
	}
}

// Selection must happen on press. Fyne defers Tapped on a DoubleTappable
// widget for the system double-click interval, so selecting from Tapped shows
// up as a visible lag before the row highlights.
func TestCellSelectsOnMouseDown(t *testing.T) {
	table := makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	result := &shared.SearchResult{Fullpath: "/tmp/a", ObjectType: shared.ItemIsFile}
	tableData = []RowData{
		{Name: []string{"a", "", ""}, Base: "a", SearchResult: result},
		{Name: []string{"b", "", ""}, Base: "b", SearchResult: result},
	}
	selectedRow = -1

	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)
	text := cell.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*tooltipCell)

	text.MouseDown(&desktop.MouseEvent{Button: desktop.MouseButtonPrimary})

	if selectedRow != 1 {
		t.Errorf("selected row = %d after press, want 1", selectedRow)
	}
}

// The tool tip layer lives on the window content, which the menu overlay
// covers. A tip left armed when the menu opens fires with nowhere to draw and
// logs "no tool tip layer created for current overlay".
func TestContextMenuCancelsPendingToolTip(t *testing.T) {
	table := makeTable()
	mainWindow = test.NewWindow(nil)
	defer func() { tableData, selectedRow, mainWindow = nil, -1, nil }()

	result := &shared.SearchResult{Fullpath: "/tmp/a", ObjectType: shared.ItemIsFile}
	tableData = []RowData{{Name: []string{"a", "", ""}, Base: "a", SearchResult: result}}

	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	text := cell.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*tooltipCell)

	// Set by MouseIn on entry; not calling it keeps the test on one goroutine,
	// as the test driver runs fyne.Do inline rather than on the UI thread.
	text.hovered = true

	text.TappedSecondary(&fyne.PointEvent{})

	if text.hovered {
		t.Error("cell still hovered after the context menu opened, so a tool tip can fire under it")
	}
}

// Scrolling repoints a cell at another row with the pointer sitting still, so
// no MouseOut arrives. A tip armed for the old row would show its details on
// the new one.
func TestRecyclingCellCancelsPendingToolTip(t *testing.T) {
	table := makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	tableData = []RowData{
		{Name: []string{"a", "", ""}, Base: "a", SearchResult: &shared.SearchResult{Fullpath: "/tmp/a", ObjectType: shared.ItemIsFile}},
		{Name: []string{"b", "", ""}, Base: "b", SearchResult: &shared.SearchResult{Fullpath: "/tmp/b", ObjectType: shared.ItemIsFile}},
	}

	cell := table.CreateCell()
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	text := cell.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*tooltipCell)

	text.hovered = true // set by MouseIn on entry
	table.UpdateCell(widget.TableCellID{Row: 0, Col: 0}, cell)
	if !text.hovered {
		t.Error("a refresh that left the cell on the same file dropped the pending tool tip")
	}

	table.UpdateCell(widget.TableCellID{Row: 1, Col: 0}, cell)
	if text.hovered {
		t.Error("cell still hovered after moving to another file, so its tool tip shows the wrong details")
	}
}

// Down from nothing selected lands on the first row, and the selection must
// stay inside the results.
func TestMoveSelectionClampsToResults(t *testing.T) {
	makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	result := &shared.SearchResult{Fullpath: "/tmp/a", ObjectType: shared.ItemIsFile}
	tableData = []RowData{{SearchResult: result}, {SearchResult: result}}
	selectedRow = -1

	moveSelection(1)
	if selectedRow != 0 {
		t.Fatalf("first Down selected row %d, want 0", selectedRow)
	}

	moveSelection(1)
	moveSelection(1)
	if selectedRow != 1 {
		t.Errorf("selection ran past the last row: %d, want 1", selectedRow)
	}

	moveSelection(-5)
	if selectedRow != 0 {
		t.Errorf("selection ran before the first row: %d, want 0", selectedRow)
	}
}

func TestMoveSelectionWithNoResults(t *testing.T) {
	makeTable()
	defer func() { tableData, selectedRow = nil, -1 }()

	tableData = nil
	selectedRow = -1

	moveSelection(1) // must not panic or select anything

	if selectedRow != -1 {
		t.Errorf("selected row %d with no results, want -1", selectedRow)
	}
}
