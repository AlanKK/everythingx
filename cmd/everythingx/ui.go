package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/shared"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	fynetooltip "github.com/dweymouth/fyne-tooltip"
)

// TODO:
// File icons
// copy path to clipboard
// tooltips

var maxSearchResults int = 1000

type RowData struct {
	Name         []string
	Path         string
	Size         string
	Modified     string
	SearchResult *shared.SearchResult
}

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

	searchElapsed := time.Since(searchStart)

	// Preallocate the TableData slice
	*tableData = make([]RowData, 0, len(results))
	var fullpath, base, dir, size, modified string

	if len(results) > 0 {
		for _, r := range results {
			fullpath = r.Fullpath
			base = filepath.Base(r.Fullpath)
			dir = filepath.Dir(fullpath) + "/"
			size, modified = shared.GetFileSizeMod(fullpath)

			if r.ObjectType == shared.ItemIsDir {
				//base += "/"
				fullpath += "/"
				size = "--"
			}

			beforeTerm, searchTerm, afterTerm := shared.SplitFileName(base, e.Text)

			*tableData = append(*tableData, RowData{
				Name:         []string{beforeTerm, searchTerm, afterTerm},
				Path:         dir,
				Size:         size,
				Modified:     modified,
				SearchResult: r,
			})
		}
	}
	t.Refresh()

	var resultText string
	if len(results) == maxSearchResults {
		resultText = fmt.Sprintf("Showing first %d objects", maxSearchResults)
	} else {
		resultText = fmt.Sprintf("%d objects", len(results))
	}
	statusBar.SetText(resultText)

	shared.PrintMemUsage()

	elapsed := time.Since(start)
	fmt.Printf(
		"\tSearch: %s, Results: %d, prefixSearch: %s, handleEntryChanged %s.\n",
		e.Text,
		len(results),
		searchElapsed,
		elapsed,
	)
}

// available for the table widget.
var tableData *[]RowData
var t *widget.Table

func makeTable() *widget.Table {
	data := make([]RowData, 0, maxSearchResults)
	tableData = &data

	t = widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(*tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			l := widget.NewRichText()
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		// UpdateCell()
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			richText := cell.(*widget.RichText)

			switch id.Col {
			case 0:
				fileNameParts := (*tableData)[id.Row].Name
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
					Text: (*tableData)[id.Row].Path,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignLeading,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			case 2:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: (*tableData)[id.Row].Size,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignTrailing,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			case 3:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: (*tableData)[id.Row].Modified,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignLeading,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				},
				}
			}
			cell.Refresh()
		},
	)
	t.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			return
		}
		handleOpenFile((*tableData)[id.Row].Path + strings.Join((*tableData)[id.Row].Name, ""))
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

**Version:** v0.1

**Author:** Alan Keister

**License:** MIT 

More info on [Github](` + GithubURL + `)
`)

	var img *canvas.Image
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantDark {
		img = canvas.NewImageFromResource(ResourceEverythingXLogo32x32monochromeicon)
	} else {
		img = canvas.NewImageFromResource(resourceIconblack64x64Png)
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

func loadUI() {
	var statusBar *widget.Label
	// printMemUsage()

	a := app.New()
	a.Settings().SetTheme(&everythingxTheme{})
	w := a.NewWindow("EverythingX")

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
	statusBar.TextStyle = fyne.TextStyle{Bold: true}

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

	shared.PrintMemUsage()
	w.ShowAndRun()

	// anything below will not be executed until app is closed
}
