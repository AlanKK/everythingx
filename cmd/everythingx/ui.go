package main

import (
	"fmt"
	"path/filepath"
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

// TODO:
// File icons

var maxSearchResults int = 1000

type RowData struct {
	Name         []string
	Path         string
	Size         string
	Modified     string
	SearchResult *shared.SearchResult
}

var searchCounter atomic.Int64

// handleAutoCompleteEntryChanged runs the search off the UI thread so that
// scrolling and other interactions remain responsive while the DB query and
// per-file stat calls execute.
func handleAutoCompleteEntryChanged(searchText string, t *widget.Table, statusBar *widget.Label) {
	if len(searchText) == 0 {
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
			size, modified := shared.GetFileSizeMod(fullpath)

			if r.ObjectType == shared.ItemIsDir {
				base += "/"
				fullpath += "/"
				size = "--"
			}

			beforeTerm, searchTerm, afterTerm := shared.SplitFileName(base, searchText)

			newData = append(newData, RowData{
				Name:         []string{beforeTerm, searchTerm, afterTerm},
				Path:         dir,
				Size:         size,
				Modified:     modified,
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
			tableData = newData
			lastResultText = resultText
			t.Refresh()
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
	path string
	col  int
}

func newTooltipCell() *tooltipCell {
	c := &tooltipCell{}
	c.Truncation = fyne.TextTruncateEllipsis
	c.RichText.ExtendBaseWidget(c)
	return c
}

func (c *tooltipCell) MouseIn(e *desktop.MouseEvent) {
	c.SetToolTip(getToolTipForFile(c.path))
	c.RichText.MouseIn(e)
}

func (c *tooltipCell) Tapped(_ *fyne.PointEvent) {
	if c.col == 0 && c.path != "" {
		mainWindow.Clipboard().SetContent(c.path)
		statusBar.SetText("✓ Copied!")
		time.AfterFunc(1500*time.Millisecond, func() {
			fyne.Do(func() {
				statusBar.SetText(lastResultText)
			})
		})
	}
}

// available for the table widget.
var tableData []RowData
var t *widget.Table
var mainWindow fyne.Window
var lastResultText string

func makeTable() *widget.Table {
	tableData = make([]RowData, 0, maxSearchResults)

	t = widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			return newTooltipCell()
		},
		// UpdateCell()
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			data := tableData
			if id.Row < 0 || id.Row >= len(data) {
				return
			}
			row := data[id.Row]
			richText := cell.(*tooltipCell)
			richText.path = row.SearchResult.Fullpath
			richText.col = id.Col

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
			richText.Refresh()
		},
	)

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

**License:** MIT 

More info on [Github](` + GithubURL + `)
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

func showSettings() {
	// Implement your settings window here
	w := fyne.CurrentApp().NewWindow("Settings")
	w.SetContent(widget.NewLabel("Settings content goes here"))
	w.Resize(fyne.NewSize(400, 300))
	w.Show()
}

var statusBar *widget.Label

func loadUI() {
	a := app.New()
	a.Settings().SetTheme(&everythingxTheme{})
	w := a.NewWindow("EverythingX")
	mainWindow = w

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("EverythingX",
			fyne.NewMenuItem("Show EverythingX", func() {
				w.Show()
			}),
			fyne.NewMenuItem("Settings", func() {
				showSettings()
			}),
			fyne.NewMenuItem("About...", func() {
				showAbout()
			}))
		desk.SetSystemTrayMenu(m)
	}

	w.SetCloseIntercept(func() {
		w.Hide()
	})

	table := makeTable()

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Enter filename...")

	statusBar = widget.NewLabel("0 objects")
	statusBar.TextStyle = fyne.TextStyle{Bold: true}

	entry.OnChanged = func(s string) {
		handleAutoCompleteEntryChanged(s, table, statusBar)
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
