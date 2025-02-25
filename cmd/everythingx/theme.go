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
	return resourceLogo512x512OrangewhitefolderPng
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
		return 0

	default:
		return theme.DefaultTheme().Size(n)
	}
}
