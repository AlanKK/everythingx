package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
				base += "/"
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

	rows := make([]RowData, len(*tableData))
	copy(rows, *tableData)
	go preloadIcons(rows, t)

	var resultText string
	if len(results) == maxSearchResults {
		resultText = fmt.Sprintf("Showing first %d objects", maxSearchResults)
	} else {
		resultText = fmt.Sprintf("%d objects", len(results))
	}
	lastResultText = resultText
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
var tableData *[]RowData
var t *widget.Table
var mainWindow fyne.Window
var lastResultText string

// iconCache stores fyne.Resource (or nil) keyed by file extension / "__dir__".
var iconCache sync.Map

func iconKey(path string, isDir bool) string {
	if isDir {
		return "__dir__"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "__noext__"
	}
	return ext
}

func cachedIcon(path string, isDir bool) fyne.Resource {
	v, ok := iconCache.Load(iconKey(path, isDir))
	if !ok {
		return nil
	}
	if v == nil {
		return nil
	}
	return v.(fyne.Resource)
}

func preloadIcons(rows []RowData, tbl *widget.Table) {
	seen := make(map[string]bool)
	changed := false
	for _, row := range rows {
		path := row.SearchResult.Fullpath
		isDir := row.SearchResult.ObjectType == shared.ItemIsDir
		key := iconKey(path, isDir)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, loaded := iconCache.Load(key); loaded {
			continue
		}
		pngData := getFileIconPNG(path, 32)
		var res fyne.Resource
		if pngData != nil {
			res = fyne.NewStaticResource(key+".png", pngData)
		}
		iconCache.Store(key, res)
		changed = true
	}
	if changed {
		fyne.Do(func() { tbl.Refresh() })
	}
}

func makeTable() *widget.Table {
	data := make([]RowData, 0, maxSearchResults)
	tableData = &data

	t = widget.NewTableWithHeaders(
		// Length()
		func() (int, int) { return len(*tableData), 4 },
		// CreateCell()
		func() fyne.CanvasObject {
			iconImg := canvas.NewImageFromResource(nil)
			iconImg.FillMode = canvas.ImageFillContain
			iconImg.SetMinSize(fyne.NewSize(18, 18))
			cell := newTooltipCell()
			// Border layout: icon pinned left, richText expands to fill remaining width.
			// NewBorder stores objects as [center..., border_items...] so:
			//   Objects[0] = cell (center), Objects[1] = iconImg (left)
			return container.NewBorder(nil, nil, iconImg, nil, cell)
		},
		// UpdateCell()
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			cont := obj.(*fyne.Container)
			richText := cont.Objects[0].(*tooltipCell) // center
			iconImg := cont.Objects[1].(*canvas.Image) // left border
			row := (*tableData)[id.Row]
			richText.path = row.SearchResult.Fullpath
			richText.col = id.Col

			if id.Col == 0 {
				iconImg.Show()
				isDir := row.SearchResult.ObjectType == shared.ItemIsDir
				if res := cachedIcon(row.SearchResult.Fullpath, isDir); res != nil {
					iconImg.Resource = res
				} else if isDir {
					iconImg.Resource = theme.FolderIcon()
				} else {
					iconImg.Resource = theme.FileIcon()
				}
				iconImg.Refresh()
			} else {
				iconImg.Hide()
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
				}}
			case 2:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: row.Size,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignTrailing,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				}}
			case 3:
				richText.Segments = []widget.RichTextSegment{&widget.TextSegment{
					Text: row.Modified,
					Style: widget.RichTextStyle{Alignment: fyne.TextAlignLeading,
						TextStyle: fyne.TextStyle{Monospace: true},
					},
				}}
			}
			richText.Refresh()
		},
	)
	t.OnSelected = func(id widget.TableCellID) {
		t.UnselectAll()
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

**Version:** ` + version.ShortInfo() + `

**Author:** Alan Keister

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
	// printMemUsage()

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
