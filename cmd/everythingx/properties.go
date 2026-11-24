package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlanKK/everythingx/internal/shared"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const propertiesLabel = "Properties"

// propRow is one labelled value in the Properties dialog.
type propRow struct{ label, value string }

// propSection is a titled group of rows in the Properties dialog.
type propSection struct {
	title string
	rows  []propRow
}

// fileProperties is what the Properties dialog shows for a file.
type fileProperties struct {
	name       string
	objectType shared.ObjectType
	mode       os.FileMode
	sections   []propSection
	icon       fyne.Resource // the item's own system icon; nil to use the row's
}

// readProperties stats path and gathers what the Properties dialog shows. It
// does file I/O (stat, owner lookups, listing a folder), so call it off the UI
// thread. A symlink is described as itself, with its target alongside.
func readProperties(path string, decimalSizes bool) (fileProperties, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return fileProperties{}, err
	}
	mode := info.Mode()
	p := fileProperties{name: filepath.Base(path), objectType: objectTypeOf(mode), mode: mode}

	general := []propRow{
		{"Type", typeDescription(path, mode)},
		{"Location", filepath.Dir(path)},
	}
	if mode&os.ModeSymlink != 0 {
		if target, err := os.Readlink(path); err == nil {
			general = append(general, propRow{"Link Target", target})
		}
	}
	switch {
	case mode.IsDir():
		general = append(general, propRow{"Contents", folderContents(path)})
	case mode.IsRegular():
		general = append(general, propRow{"Size", sizeDescription(info.Size(), decimalSizes)})
	}
	general = append(general, propRow{"Modified", info.ModTime().Format(propertiesTimeFormat)})

	owner, group := shared.FileOwnerGroup(info)
	perms := []propRow{
		{"Owner", owner},
		{"Group", group},
		{"Access", fmt.Sprintf("%s  (%04o)", mode.Perm(), mode.Perm())},
	}

	p.sections = []propSection{{"General", general}, {"Permissions", perms}}
	return p, nil
}

// propertiesTimeFormat spells the date out in full, as the dialog has the room
// the Last Modified column does not.
const propertiesTimeFormat = "Monday, January 2, 2006 at 15:04:05"

func objectTypeOf(mode os.FileMode) shared.ObjectType {
	switch {
	case mode&os.ModeSymlink != 0:
		return shared.ItemIsSymlink
	case mode.IsDir():
		return shared.ItemIsDir
	case mode&os.ModeNamedPipe != 0:
		return shared.ItemIsNamedPipe
	case mode&os.ModeSocket != 0:
		return shared.ItemIsSocket
	case mode&os.ModeCharDevice != 0:
		return shared.ItemIsCharDevice
	case mode&os.ModeDevice != 0:
		return shared.ItemIsBlockDevice
	}
	return shared.ItemIsFile
}

// typeDescription names the kind of item, using the extension for a file:
// "PDF file", or plain "File" when it has none.
func typeDescription(path string, mode os.FileMode) string {
	switch {
	case mode&os.ModeSymlink != 0:
		return "Symbolic link"
	case mode.IsDir():
		return "Folder"
	case mode&os.ModeNamedPipe != 0:
		return "Named pipe"
	case mode&os.ModeSocket != 0:
		return "Socket"
	case mode&os.ModeCharDevice != 0:
		return "Character device"
	case mode&os.ModeDevice != 0:
		return "Block device"
	}
	if ext := strings.TrimPrefix(filepath.Ext(path), "."); ext != "" {
		return strings.ToUpper(ext) + " file"
	}
	return "File"
}

// sizeDescription gives the size in the user's units with the exact byte
// count after it, as file managers do: "1.5K (1,536 bytes)".
func sizeDescription(size int64, decimalSizes bool) string {
	bytes := groupDigits(size) + " bytes"
	if size == 1 {
		bytes = "1 byte"
	}
	base := int64(1024)
	if decimalSizes {
		base = 1000
	}
	if size < base {
		return bytes // the short form would say nothing more
	}
	return fmt.Sprintf("%s (%s)", shared.HumanizeSizeUnits(size, decimalSizes), bytes)
}

// groupDigits writes n with thousands separators: 1536 -> "1,536".
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// folderContents counts a folder's direct entries: "12 items".
func folderContents(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "Unreadable"
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return "Unreadable"
	}
	switch len(names) {
	case 0:
		return "Empty"
	case 1:
		return "1 item"
	}
	return groupDigits(int64(len(names))) + " items"
}

// showProperties reads the item's details off the UI thread and then shows
// them in a dialog over the main window.
func showProperties(path string) {
	decimalSizes := prefs.DecimalSizes
	go func() {
		props, err := readProperties(path, decimalSizes)
		if err == nil {
			props.icon = propertiesIcon(path, props.mode)
		}
		fyne.Do(func() {
			if err != nil {
				showDialog(dialog.NewError(fmt.Errorf("Can't read %s: %w", filepath.Base(path), err), mainWindow))
				return
			}
			showDialog(dialog.NewCustom(propertiesLabel, "Close", propertiesContent(path, props), mainWindow))
		})
	}()
}

// propertiesIconSize is how big the dialog draws the item's icon, and
// propertiesIconPixels the size the system icon is fetched at: twice that,
// so it stays sharp on HiDPI displays.
const (
	propertiesIconSize   = 48
	propertiesIconPixels = 2 * propertiesIconSize
)

// propertiesWidth is how wide the dialog's content is. Long values (paths,
// names) wrap within it rather than widening the dialog.
const propertiesWidth float32 = 480

// propertiesContent lays out the dialog: the item's icon and name at the top,
// then each section as a heading over a two-column grid, labels on the left
// in a softer colour and values on the right. All the grids share one label
// column width, so the values line up from section to section.
func propertiesContent(path string, p fileProperties) fyne.CanvasObject {
	res := p.icon
	if res == nil {
		res = rowIcon(&shared.SearchResult{Fullpath: path, ObjectType: p.objectType}, false)
	}
	icon := canvas.NewImageFromResource(res)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSquareSize(propertiesIconSize))

	name := widget.NewLabelWithStyle(p.name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	name.Wrapping = fyne.TextWrapBreak
	name.SizeName = theme.SizeNameSubHeadingText
	name.Selectable = true

	labelW := icon.MinSize().Width
	type section struct {
		heading *widget.Label
		cells   []fyne.CanvasObject
	}
	var sections []section
	for _, s := range p.sections {
		sec := section{heading: widget.NewLabelWithStyle(s.title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})}
		for _, r := range s.rows {
			label := widget.NewRichText(&widget.TextSegment{
				Text:  r.label,
				Style: widget.RichTextStyle{Alignment: fyne.TextAlignTrailing, ColorName: theme.ColorNamePlaceHolder},
			})
			labelW = max(labelW, label.MinSize().Width)
			value := widget.NewLabel(r.value)
			value.Wrapping = fyne.TextWrapBreak
			value.Selectable = true
			sec.cells = append(sec.cells, label, value)
		}
		sections = append(sections, sec)
	}

	grid := func(cells ...fyne.CanvasObject) *fyne.Container {
		return container.New(&propGrid{labelW: labelW, width: propertiesWidth}, cells...)
	}
	body := container.NewVBox(grid(container.NewCenter(icon), name))
	for _, sec := range sections {
		body.Add(widget.NewSeparator())
		body.Add(sec.heading)
		body.Add(grid(sec.cells...))
	}
	return body
}

// propGrid lays its objects out in label/value pairs, one pair per row, with
// a fixed label column and a fixed overall width. Fixing the width up front
// is what lets a wrapping value report its real height: Fyne's FormLayout
// measures a wrapping label before it has a width, and gets one word per line.
type propGrid struct {
	labelW, width float32
}

func (g *propGrid) valueW() float32 { return g.width - g.labelW - theme.Padding() }

// rowHeight is the taller of a pair, with the value measured at its column
// width.
func (g *propGrid) rowHeight(label, value fyne.CanvasObject) float32 {
	value.Resize(fyne.NewSize(g.valueW(), value.Size().Height))
	return max(label.MinSize().Height, value.MinSize().Height)
}

func (g *propGrid) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var h float32
	for i := 0; i+1 < len(objects); i += 2 {
		h += g.rowHeight(objects[i], objects[i+1])
	}
	return fyne.NewSize(g.width, h)
}

func (g *propGrid) Layout(objects []fyne.CanvasObject, _ fyne.Size) {
	var y float32
	for i := 0; i+1 < len(objects); i += 2 {
		label, value := objects[i], objects[i+1]
		h := g.rowHeight(label, value)
		label.Move(fyne.NewPos(0, y))
		label.Resize(fyne.NewSize(g.labelW, label.MinSize().Height))
		value.Move(fyne.NewPos(g.labelW+theme.Padding(), y))
		value.Resize(fyne.NewSize(g.valueW(), h))
		y += h
	}
}
