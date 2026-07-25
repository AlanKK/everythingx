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

// Icon delegates to the default theme so the file-type icons in the results
// table resolve to their own artwork. The app/about artwork references the
// bundled logo resources directly, not through the theme.
func (*everythingxTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (*everythingxTheme) Size(n fyne.ThemeSizeName) float32 {

	switch n {
	// case theme.SizeNameWindowTitleBarHeight:
	// return 26 // does nothing
	// case theme.SizeNameHeadingText: // does nothing
	// return 24
	case theme.SizeNameSeparatorThickness:
		return 0
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNamePadding:
		// Table column dividers are exactly this wide, and Fyne only starts a
		// column resize drag when the pointer is inside one. At 0 the gap does
		// not exist, so columns cannot be resized and no dividers are drawn.
		// Raise this if the drag target feels too narrow to hit.
		return 2

	default:
		return theme.DefaultTheme().Size(n)
	}
}
