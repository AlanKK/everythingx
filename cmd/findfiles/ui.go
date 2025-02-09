package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/models"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// TODO:
// - Add system tray icon: https://docs.fyne.io/explore/systray.html

var maxSearchResults int = 1000

func handleOpenFile(pathname string) {
	if pathname == "" {
		return
	}

	cmd := exec.Command("open", "-R", pathname)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error:", err)
	}
}

func handleAutoCompleteEntryChanged(e *widget.Entry, t *widget.Table, statusBar *widget.Label) {
	start := time.Now()

	if len(e.Text) == 0 {
		return
	}

	searchStart := time.Now()
	results, err := ffdb.PrefixSearch(e.Text, maxSearchResults)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if len(results) == 0 {
		return
	}

	searchElapsed := time.Since(searchStart)

	// Preallocate the TableData slice
	*tableData = make([]RowData, 0, len(results))
	var fullpath, dir, base, size, modified string

	for _, r := range results {
		base = filepath.Base(r.Fullpath)
		dir = filepath.Dir(r.Fullpath) + "/"
		size, modified = getFileSizeMod(r.Fullpath)

		if r.ObjectType == models.ItemIsDir {
			base += "/"
			fullpath += "/"
			size = "--"
		}
		*tableData = append(*tableData, RowData{
			Name:         base,
			Path:         dir,
			Size:         size,
			Modified:     modified,
			SearchResult: r,
		})
	}
	t.Refresh()

	var resultText string
	if len(results) == maxSearchResults {
		resultText = fmt.Sprintf("Showing first %d objects", maxSearchResults)
	} else {
		resultText = fmt.Sprintf("%d objects", len(results))
	}
	statusBar.SetText(resultText)

	printMemUsage()

	elapsed := time.Since(start)
	fmt.Printf(
		"\tSearch: %s, Results: %d, prefixSearch: %s, handleEntryChanged %s.\n",
		e.Text,
		len(results),
		searchElapsed,
		elapsed,
	)
}

type RowData struct {
	Name         string
	Path         string
	Size         string
	Modified     string
	SearchResult *models.SearchResult
}

// tableData holds the search results to be displayed in the table. Must be always
// available for the table widget.
var tableData *[]RowData

func makeTable() *widget.Table {
	data := make([]RowData, 0, maxSearchResults)
	tableData = &data

	t := widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(*tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			l := ttwidget.NewLabel("Template")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		// UpdateCell()
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*ttwidget.Label)

			fynetooltip.SetToolTipTextStyle(fyne.TextStyle{Monospace: true})
			toolTipText := getToolTipForFile((*tableData)[id.Row].SearchResult.Fullpath)

			switch id.Col {
			case 0:
				label.SetText((*tableData)[id.Row].Name)
				label.SetToolTip(toolTipText)
				label.Alignment = fyne.TextAlignLeading
			case 1:
				label.SetText((*tableData)[id.Row].Path)
				label.SetToolTip(toolTipText)
				label.Alignment = fyne.TextAlignLeading
			case 2:
				label.SetText((*tableData)[id.Row].Size)
				label.Alignment = fyne.TextAlignTrailing
			case 3:
				label.SetText((*tableData)[id.Row].Modified)
				label.Alignment = fyne.TextAlignLeading
			}
		},
	)
	t.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			return
		}
		handleOpenFile((*tableData)[id.Row].Path + (*tableData)[id.Row].Name)
	}

	t.SetColumnWidth(0, 400) // Name
	t.SetColumnWidth(1, 600) // Path
	t.SetColumnWidth(2, 70)  // Size
	t.SetColumnWidth(3, 190) // Last modified

	// Define custom headers
	t.CreateHeader = func() fyne.CanvasObject {
		return widget.NewLabel("")
	}
	t.UpdateHeader = func(cellID widget.TableCellID, header fyne.CanvasObject) {
		label := header.(*widget.Label)

		if cellID.Col != -1 {
			label.TextStyle = fyne.TextStyle{Bold: true}
			switch cellID.Col {
			case 0:
				label.SetText("Name")
			case 1:
				label.SetText("Path")
			case 2:
				label.SetText("Size")
			case 3:
				label.SetText("Last Modified")
			}
		}
	}
	return t
}

func getToolTipForFile(path string) string {
	lsFormat, err := getFileInfo(path)
	if err != nil {
		return "No access"
	}

	return fmt.Sprintf("%s\n\n%s\n", path, lsFormat)
}

func loadUI() {
	var statusBar *widget.Label
	printMemUsage()

	a := app.New()
	w := a.NewWindow("EverythingX")
	a.SetIcon(ResourceEverythingXLogo32x32monochromeicon)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("EverythingX",
			fyne.NewMenuItem("Show EverythingX", func() {
				w.Show()
			}))
		desk.SetSystemTrayMenu(m)
	}

	w.SetContent(widget.NewLabel("EverythingX"))
	w.SetCloseIntercept(func() {
		w.Hide()
	})

	table := makeTable()

	// Autocomplete entry box
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Enter filename...")
	entry.OnChanged = func(s string) {
		handleAutoCompleteEntryChanged(entry, table, statusBar)
	}
	w.SetContent(container.NewVBox(entry))
	w.Canvas().Focus(entry)

	statusBar = widget.NewLabel("0 objects")

	content := container.NewBorder(
		entry,
		statusBar,
		nil,
		nil,
		table,
	)

	w.SetContent(content)
	w.SetContent(fynetooltip.AddWindowToolTipLayer(content, w.Canvas()))

	w.Resize(fyne.NewSize(1300, 800))

	printMemUsage()
	w.ShowAndRun()

	// anything below will not be executed until app is closed
}
