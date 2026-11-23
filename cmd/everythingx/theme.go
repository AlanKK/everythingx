package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// colorNameMatch is the colour of the matched part of a name in the results.
const colorNameMatch fyne.ThemeColorName = "everythingxMatch"

// namedColor is one of the swatches on the Appearance tab.
type namedColor struct {
	name  string
	label string
	color color.NRGBA
	// onColor is the text colour that reads on top of color.
	onColor color.NRGBA
}

var (
	onLight = color.NRGBA{R: 0x17, G: 0x17, B: 0x18, A: 0xff}
	onDark  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// accentColors are Fyne's own primary colours, so an accent picked here looks
// the same as one picked in Fyne's settings app. Blue is Fyne's default.
var accentColors = []namedColor{
	{theme.ColorBlue, "Blue", color.NRGBA{R: 0x29, G: 0x6f, B: 0xf6, A: 0xff}, onDark},
	{theme.ColorPurple, "Purple", color.NRGBA{R: 0x9c, G: 0x27, B: 0xb0, A: 0xff}, onDark},
	{theme.ColorRed, "Red", color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}, onDark},
	{theme.ColorOrange, "Orange", color.NRGBA{R: 0xff, G: 0x98, B: 0x00, A: 0xff}, onLight},
	{theme.ColorYellow, "Yellow", color.NRGBA{R: 0xff, G: 0xeb, B: 0x3b, A: 0xff}, onLight},
	{theme.ColorGreen, "Green", color.NRGBA{R: 0x8b, G: 0xc3, B: 0x4a, A: 0xff}, onLight},
	{theme.ColorBrown, "Brown", color.NRGBA{R: 0x79, G: 0x55, B: 0x48, A: 0xff}, onDark},
	{theme.ColorGray, "Gray", color.NRGBA{R: 0x9e, G: 0x9e, B: 0x9e, A: 0xff}, onLight},
}

// accentColor looks up a colour by name, reporting false for an unknown one.
// "" is valid and means the default: it returns a zero namedColor and true.
func accentColor(name string) (namedColor, bool) {
	if name == "" {
		return namedColor{}, true
	}
	for _, c := range accentColors {
		if c.name == name {
			return c, true
		}
	}
	return namedColor{}, false
}

// Default swatches: what "" looks like for the accent and the match colour.
// Fyne's default primary is blue and its warning colour, which the match
// highlight used before it could be changed, is orange.
const (
	defaultAccentName = theme.ColorBlue
	defaultMatchName  = theme.ColorOrange
)

// everythingxTheme is the app's theme. Its zero value is Fyne's default theme
// with the app's fonts and sizes. The appearance preferences are copied into
// it by setPrefs rather than read on each call, since Color and Size run for
// every object on every paint.
type everythingxTheme struct {
	// forced is true when the user picked Light or Dark rather than following
	// the system; variant is then the one they picked.
	forced  bool
	variant fyne.ThemeVariant

	accent *namedColor // nil for Fyne's default primary colour
	match  *namedColor // nil for the default match colour

	textScale float32 // 0 means 1

	font    string // fontSystem, fontFyne or fontMono; "" is fontSystem
	compact bool   // tighter rows and controls, like a GTK app
}

var _ fyne.Theme = (*everythingxTheme)(nil)

// setPrefs copies the appearance preferences into the theme. Call
// Settings().SetTheme with it afterwards so the windows repaint.
func (th *everythingxTheme) setPrefs(p appPrefs) {
	th.forced = p.Theme != themeSystem
	th.variant = theme.VariantLight
	if p.Theme == themeDark {
		th.variant = theme.VariantDark
	}

	th.accent = nil
	if c, _ := accentColor(p.Accent); c.name != "" {
		th.accent = &c
	}
	th.match = nil
	if c, _ := accentColor(p.MatchColor); c.name != "" {
		th.match = &c
	}
	th.textScale = float32(p.TextScale)
	th.font = p.Font
	th.compact = p.Density == densityCompact
}

// Variant is the variant the theme draws with: the forced one, or else the
// system's. Use it rather than Settings().ThemeVariant(), which reports the
// system's even when the user forced the other.
func (th *everythingxTheme) Variant() fyne.ThemeVariant {
	if th.forced {
		return th.variant
	}
	return fyne.CurrentApp().Settings().ThemeVariant()
}

// Font draws with the chosen font. Monospace text, such as the command on the
// Shortcut tab, is Roboto Mono whatever the choice.
func (th *everythingxTheme) Font(s fyne.TextStyle) fyne.Resource {
	if s.Monospace || th.font == fontMono {
		return robotoMono(s)
	}
	if th.font != fontFyne && systemFonts != nil {
		return systemFonts.face(s)
	}
	return theme.DefaultTheme().Font(s)
}

func robotoMono(s fyne.TextStyle) fyne.Resource {
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

func (th *everythingxTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if th.forced {
		v = th.variant
	}

	if n == colorNameMatch {
		if th.match != nil {
			return th.match.color
		}
		n = theme.ColorNameWarning
	}

	if a := th.accent; a != nil {
		// Fyne derives focus and selection from its own primary colour, so
		// derive them here too or the selected row clashes with the accent.
		// The alphas are the ones Fyne's default theme uses.
		switch n {
		case theme.ColorNamePrimary, theme.ColorNameHyperlink:
			return a.color
		case theme.ColorNameForegroundOnPrimary:
			return a.onColor
		case theme.ColorNameFocus:
			return withAlpha(a.color, 0x2a)
		case theme.ColorNameSelection:
			return withAlpha(a.color, 0x40)
		}
	}

	return theme.DefaultTheme().Color(n, v)
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// Icon delegates to the default theme so the file-type icons in the results
// table resolve to their own artwork. The app/about artwork references the
// bundled logo resources directly, not through the theme.
func (*everythingxTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

// compactIconSize is the inline icon size at Compact density, the 16px of a
// GTK list view's icons, rather than Fyne's 20.
const compactIconSize = 16

func (th *everythingxTheme) Size(n fyne.ThemeSizeName) float32 {

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
		// resize by dragging, so keep a small non-zero value. Lowering it
		// fits more rows on screen at the cost of a narrower drag target.
		return 4
	case theme.SizeNameInnerPadding:
		// The space inside a cell, entry or button, which sets the row
		// height. Compact matches the row spacing of GTK list views.
		if th.compact {
			return 3
		}
		return theme.DefaultTheme().Size(n)
	case theme.SizeNameText, theme.SizeNameCaptionText, theme.SizeNameHeadingText,
		theme.SizeNameSubHeadingText, theme.SizeNameInlineIcon:
		// Icons in rows scale with the text so rows keep their proportions.
		size := theme.DefaultTheme().Size(n)
		if n == theme.SizeNameInlineIcon && th.compact {
			size = compactIconSize
		}
		if th.textScale > 0 {
			size *= th.textScale
		}
		return size

	default:
		return theme.DefaultTheme().Size(n)
	}
}
