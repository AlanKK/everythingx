package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type myTheme struct{}

var _ fyne.Theme = (*myTheme)(nil)

// return bundled font resource
func (*myTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Monospace {
		return resourceRobotoMonoRegularTtf
	}
	if s.Bold {
		if s.Italic {
			return theme.DefaultTheme().Font(s)
		}
		return resourceRobotoMonoBoldTtf
	}
	if s.Italic {
		return resourceRobotoMonoItalicTtf
	}
	return resourceRobotoMonoRegularTtf
}

// func (*myTheme) Padding() int {
// 	return theme.DefaultTheme().Padding()
// }

func (*myTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, v)
}

func (*myTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return ResourceEverythingXLogo32x32monochromeicon
}

func (*myTheme) Size(n fyne.ThemeSizeName) float32 {

	switch n {
	// case theme.SizeNameSeparatorThickness:
	// 	return 1
	case theme.SizeNameLineSpacing:
		return 4
	case theme.SizeNamePadding:
		return 1
	case theme.SizeNameHeadingText:
		return 24
	case theme.SizeNameWindowTitleBarHeight:
		return 26

	default:
		return theme.DefaultTheme().Size(n)
	}
}
