package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// padMenuIcon returns res with a transparent margin around it, so that drawn
// in a menuIconSlot square its artwork is menuIconGlyph across. Fyne puts no
// space between a menu's edge, its icon and its label beyond the inner
// padding, which menuTheme keeps small to keep the rows short; the margin is
// what spaces them out. A resource it cannot pad comes back as it is.
func padMenuIcon(res fyne.Resource) fyne.Resource {
	if res == nil {
		return nil
	}
	if r, ok := paddedIcons.Load(res); ok {
		return r.(fyne.Resource)
	}
	out := res
	switch r := res.(type) {
	case *theme.ThemedResource:
		// Keep it themed so it still follows the text colour.
		if p := padIconBytes(r.Content(), r.Name()); p != nil {
			t := theme.NewThemedResource(fyne.NewStaticResource(r.Name(), p))
			t.ColorName = r.ColorName
			out = t
		}
	default:
		if p := padIconBytes(res.Content(), res.Name()); p != nil {
			out = fyne.NewStaticResource("padded-"+res.Name(), p)
		}
	}
	paddedIcons.Store(res, out)
	return out
}

// paddedIcons caches padMenuIcon's results, keyed by the resource padded.
// Menu icons are a handful of theme icons and app icons, so it stays small.
var paddedIcons sync.Map // fyne.Resource -> fyne.Resource

// padIconBytes pads an SVG or a raster image, returning nil when it cannot.
func padIconBytes(b []byte, name string) []byte {
	if bytes.Contains(b[:min(len(b), 512)], []byte("<svg")) || strings.HasSuffix(strings.ToLower(name), ".svg") {
		return padSVG(b)
	}
	return padRaster(b)
}

var (
	svgTag     = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	svgViewBox = regexp.MustCompile(`\sviewBox\s*=\s*["']([^"']*)["']`)
	svgWidth   = regexp.MustCompile(`\swidth\s*=\s*["']([\d.]+)(px)?["']`)
	svgHeight  = regexp.MustCompile(`\sheight\s*=\s*["']([\d.]+)(px)?["']`)
)

// padSVG widens the root element's viewBox by the margin on every side. An
// SVG with no viewBox gets one from its width and height.
func padSVG(b []byte) []byte {
	loc := svgTag.FindIndex(b)
	if loc == nil {
		return nil
	}
	tag := string(b[loc[0]:loc[1]])

	var x, y, w, h float64
	if m := svgViewBox.FindStringSubmatch(tag); m != nil {
		f := strings.FieldsFunc(m[1], func(r rune) bool { return r == ' ' || r == ',' })
		if len(f) != 4 {
			return nil
		}
		v := make([]float64, 4)
		for i, s := range f {
			n, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil
			}
			v[i] = n
		}
		x, y, w, h = v[0], v[1], v[2], v[3]
		tag = svgViewBox.ReplaceAllString(tag, "")
	} else {
		mw, mh := svgWidth.FindStringSubmatch(tag), svgHeight.FindStringSubmatch(tag)
		if mw == nil || mh == nil {
			return nil
		}
		w, _ = strconv.ParseFloat(mw[1], 64)
		h, _ = strconv.ParseFloat(mh[1], 64)
	}
	if w <= 0 || h <= 0 {
		return nil
	}
	// A fixed width and height would undo the wider viewBox; drop them so the
	// image takes the size it is drawn at.
	tag = svgWidth.ReplaceAllString(tag, "")
	tag = svgHeight.ReplaceAllString(tag, "")

	scale := float64(menuIconSlot) / menuIconGlyph
	mx, my := w*(scale-1)/2, h*(scale-1)/2
	vb := fmt.Sprintf(` viewBox="%g %g %g %g"`, x-mx, y-my, w+2*mx, h+2*my)
	tag = strings.Replace(tag, "<svg", "<svg"+vb, 1)

	out := make([]byte, 0, len(b)+len(vb))
	out = append(out, b[:loc[0]]...)
	out = append(out, tag...)
	return append(out, b[loc[1]:]...)
}

// padRaster draws a PNG or JPEG in the middle of a larger transparent PNG.
func padRaster(b []byte) []byte {
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	sb := src.Bounds()
	w := sb.Dx() * menuIconSlot / menuIconGlyph
	h := sb.Dy() * menuIconSlot / menuIconGlyph
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	at := image.Pt((w-sb.Dx())/2, (h-sb.Dy())/2)
	draw.Draw(dst, sb.Sub(sb.Min).Add(at), src, sb.Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil
	}
	return buf.Bytes()
}

// padMenuIcons pads the icon of every item of menu and its submenus.
func padMenuIcons(menu *fyne.Menu) {
	for _, it := range menu.Items {
		it.Icon = padMenuIcon(it.Icon)
		if it.ChildMenu != nil {
			padMenuIcons(it.ChildMenu)
		}
	}
}
