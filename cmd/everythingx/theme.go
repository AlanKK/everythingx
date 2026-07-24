package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type everythingxTheme struct{}

var _ fyne.Theme = (*everythingxTheme)(nil)

// Roboto Mono. All of these are monospace fonts
func (*everythingxTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Monospace {
		return resourceRobotoMonoRegular
	}
	if s.Bold {
		if s.Italic {
			return resourceRobotoMonoBoldItalic
		}
		return resourceRobotoMonoBold
	}
	if s.Italic {
		return resourceRobotoMonoItalic
	}
	return resourceRobotoMonoRegular
}

// func (*myTheme) Padding() int {
// 	return theme.DefaultTheme().Padding()
// }

func (*everythingxTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, v)
}

func (*everythingxTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return resourceFolderWhiteOrange5122xPng
}

func (*everythingxTheme) Size(n fyne.ThemeSizeName) float32 {

	switch n {
	// case theme.SizeNameWindowTitleBarHeight:
	// return 26 // does nothing
	// case theme.SizeNameHeadingText: // does nothing
	// return 24
	case theme.SizeNameSeparatorThickness:
		// Thin visible column/row dividers. Also makes the drag-to-resize
		// affordance discoverable in the table header.
		return 1
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNamePadding:
		// The table derives its column-divider grab zone from padding
		// (widget.Table.columnAt). A zero here makes columns impossible to
		// resize by dragging, so keep a small non-zero value.
		return 4

	default:
		return theme.DefaultTheme().Size(n)
	}
}
