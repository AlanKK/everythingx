//go:build linux

package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"github.com/AlanKK/everythingx/internal/shared"
)

// This file finds the icon the desktop's file manager would show for a
// result: the item's MIME type from its name, then that type's icon in the
// user's icon theme, as the freedesktop icon theme specification describes.

// iconPixels is the size icons are looked up at: twice the 16px inline icon,
// so they stay sharp on HiDPI displays.
const iconPixels = 32

// themeQueryTimeout bounds asking gsettings for the icon theme.
const themeQueryTimeout = time.Second

// maxNameIcons bounds nameIcons; past it the cache starts over.
const maxNameIcons = 20000

// nameIcons caches the icon for each file name. Globs can match whole names
// (Makefile, *.tar.gz), so the name rather than the extension is the key;
// looking up its type means matching every glob, too slow for each refresh.
var nameIcons struct {
	sync.Mutex
	m map[string]fyne.Resource
}

// nativeFileIcon returns the icon theme's icon for the result's type, or nil
// when the theme has none.
func nativeFileIcon(r *shared.SearchResult) fyne.Resource {
	switch r.ObjectType {
	case shared.ItemIsDir:
		return cachedTypeIcon(typeIconKey(int(r.ObjectType), ""), func() fyne.Resource {
			return themedIcon("folder", "inode-directory")
		})
	case shared.ItemIsSymlink:
		return cachedTypeIcon(typeIconKey(int(r.ObjectType), ""), func() fyne.Resource {
			return themedIcon("inode-symlink")
		})
	case shared.ItemIsFile:
	default:
		return cachedTypeIcon(typeIconKey(int(r.ObjectType), ""), func() fyne.Resource {
			return themedIcon(mimeIconNames(currentMimeDB(), mimeOctetStream)...)
		})
	}

	name := filepath.Base(r.Fullpath)
	nameIcons.Lock()
	res, ok := nameIcons.m[name]
	nameIcons.Unlock()
	if ok {
		return res
	}
	db := currentMimeDB()
	t := mimeOctetStream
	if g := db.globType(name); g != "" {
		t = db.canonical(g)
	}
	res = cachedTypeIcon("mime:"+t, func() fyne.Resource {
		return themedIcon(mimeIconNames(db, t)...)
	})
	nameIcons.Lock()
	if nameIcons.m == nil || len(nameIcons.m) >= maxNameIcons {
		nameIcons.m = map[string]fyne.Resource{}
	}
	nameIcons.m[name] = res
	nameIcons.Unlock()
	return res
}

// mimeIconNames lists the icon names to try for type t, best first: the
// type's own icon and its generic icon, then those of its supertypes, then
// the generic icon of its media type, and last the generic file icon.
func mimeIconNames(db *mimeDB, t string) []string {
	var names []string
	add := func(n string) {
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	chain := db.typeChain(t)
	for _, c := range chain {
		if c == mimeOctetStream {
			continue
		}
		add(strings.ReplaceAll(c, "/", "-"))
		add(db.genericIcons[c])
	}
	if media, _, ok := strings.Cut(t, "/"); ok && media != "application" {
		add(media + "-x-generic")
	}
	add("application-octet-stream")
	add("application-x-generic")
	return names
}

// themedIcon returns the first of names that the icon theme has, as a
// resource, or nil.
func themedIcon(names ...string) fyne.Resource {
	path := currentIconLookup().find(names, iconPixels)
	if path == "" {
		return nil
	}
	res, err := fyne.LoadResourceFromPath(path)
	if err != nil {
		return nil
	}
	return res
}

// iconLookup is the user's icon theme with the themes it inherits from, most
// important first, ending with hicolor.
type iconLookup struct {
	bases  []string // directories holding themes, most important first
	themes []*iconTheme
}

type iconTheme struct {
	name    string
	dirs    []iconDir
	inherit []string
}

// iconDir is one of a theme's directories of icons, relative to the theme.
type iconDir struct {
	path     string
	size     int
	scale    int
	scalable bool
}

var (
	iconLookupOnce sync.Once
	iconLookupVal  *iconLookup
)

func currentIconLookup() *iconLookup {
	iconLookupOnce.Do(func() {
		iconLookupVal = newIconLookup(iconBaseDirs(), userIconTheme())
	})
	return iconLookupVal
}

// iconBaseDirs returns the directories icon themes are installed in, most
// important first: ~/.icons and each data directory's icons folder.
func iconBaseDirs() []string {
	var bases []string
	if h, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(h, ".icons"))
	}
	for _, d := range xdgDataDirs() {
		bases = append(bases, filepath.Join(d, "icons"))
	}
	return bases
}

// newIconLookup loads theme and the themes it inherits from, breadth first,
// then Adwaita and its parents, then hicolor, which every theme falls back
// to. Themes that are not installed are skipped.
func newIconLookup(bases []string, theme string) *iconLookup {
	l := &iconLookup{bases: bases}
	seen := map[string]bool{"": true, "hicolor": true}
	// add loads name and the themes it inherits from, breadth first.
	add := func(name string) {
		queue := []string{name}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if seen[n] {
				continue
			}
			seen[n] = true
			if th := loadIconTheme(bases, n); th != nil {
				l.themes = append(l.themes, th)
				queue = append(queue, th.inherit...)
			}
		}
	}
	// The user's theme and all of its parents come first, so an overlay
	// theme (Papirus-Dark over Papirus) gets its parent's icons rather than
	// Adwaita's. Adwaita then fills gaps, as GTK does, and hicolor is last.
	add(theme)
	add("Adwaita")
	if th := loadIconTheme(bases, "hicolor"); th != nil {
		l.themes = append(l.themes, th)
	}
	return l
}

// loadIconTheme reads the theme's index.theme from the first base directory
// that has one, keeping the directories of file-type and folder icons. It
// returns nil if the theme is not installed.
func loadIconTheme(bases []string, name string) *iconTheme {
	for _, b := range bases {
		f, err := os.Open(filepath.Join(b, name, "index.theme"))
		if err != nil {
			continue
		}
		defer f.Close()
		return parseIconTheme(name, bufio.NewScanner(f))
	}
	return nil
}

func parseIconTheme(name string, sc *bufio.Scanner) *iconTheme {
	th := &iconTheme{name: name}
	sections := map[string]map[string]string{}
	var cur map[string]string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			cur = map[string]string{}
			sections[line[1:len(line)-1]] = cur
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && cur != nil {
			cur[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	head := sections["Icon Theme"]
	if head == nil {
		return th
	}
	th.inherit = splitCommaList(head["Inherits"])
	for _, d := range splitCommaList(head["Directories"] + "," + head["ScaledDirectories"]) {
		sec := sections[d]
		if sec == nil || strings.Contains(d, "symbolic") {
			continue
		}
		switch sec["Context"] {
		case "MimeTypes", "Places":
		default:
			continue
		}
		size, err := strconv.Atoi(sec["Size"])
		if err != nil {
			continue
		}
		scale := 1
		if s, err := strconv.Atoi(sec["Scale"]); err == nil && s > 0 {
			scale = s
		}
		th.dirs = append(th.dirs, iconDir{path: d, size: size, scale: scale, scalable: sec["Type"] == "Scalable"})
	}
	return th
}

// splitCommaList splits a ","-separated index.theme value, dropping empty and
// repeated items.
func splitCommaList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// find returns the path of the first of names that a theme has, looking
// through each theme in turn so the icons keep one theme's look, then
// unthemed icons in /usr/share/pixmaps. It returns "" when none is found.
func (l *iconLookup) find(names []string, px int) string {
	for _, th := range l.themes {
		for _, n := range names {
			if p := l.findInTheme(th, n, px); p != "" {
				return p
			}
		}
	}
	for _, n := range names {
		for _, ext := range []string{".svg", ".png"} {
			p := filepath.Join("/usr/share/pixmaps", n+ext)
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

// svgPenalty ranks every SVG behind every PNG, and badSVGPenalty an SVG that
// Fyne cannot draw behind every other SVG. A theme's small PNGs are drawn by
// hand for their size and come out crisper in a list row than its 128px
// SVGs, whose fine strokes vanish when shrunk; and Fyne's SVG renderer lacks
// masks, filters, clip paths and inherited gradients, which current themes
// use (Adwaita's archive and audio icons draw as black blots). SVG-only
// themes such as Papirus still get their SVGs.
const (
	svgPenalty    = 1 << 20
	badSVGPenalty = 1 << 21
)

// unsupportedSVG matches the SVG features Fyne's renderer cannot draw.
var unsupportedSVG = regexp.MustCompile(`<(mask|filter|clipPath)[\s>/]|href="#`)

// badSVGs caches whether each SVG file uses features Fyne cannot draw.
var badSVGs sync.Map // path -> bool

func svgDrawable(path string) bool {
	if bad, ok := badSVGs.Load(path); ok {
		return !bad.(bool)
	}
	data, err := os.ReadFile(path)
	bad := err != nil || unsupportedSVG.Match(data)
	badSVGs.Store(path, bad)
	return !bad
}

// findInTheme returns the theme's best file for icon name at px pixels: a PNG
// at px or a little larger, then a larger one, then the largest smaller one;
// failing that an SVG, ranked the same way with scalable ones first and those
// Fyne cannot draw last of all.
func (l *iconLookup) findInTheme(th *iconTheme, name string, px int) string {
	best, bestScore := "", 0
	for _, d := range th.dirs {
		for _, ext := range []string{".png", ".svg"} {
			score := iconDirScore(d, px)
			if best != "" && score >= bestScore {
				continue
			}
			for _, b := range l.bases {
				p := filepath.Join(b, th.name, d.path, name+ext)
				if !fileExists(p) {
					continue
				}
				if ext == ".svg" {
					score += svgPenalty
					if !svgDrawable(p) {
						score += badSVGPenalty
					}
				}
				if best == "" || score < bestScore {
					best, bestScore = p, score
				}
				break
			}
		}
	}
	return best
}

// iconDirScore ranks a directory for icons drawn at px pixels; lower is
// better.
func iconDirScore(d iconDir, px int) int {
	size := d.size * d.scale
	switch {
	case d.scalable:
		return px + 1
	case size >= px && size <= 2*px:
		return size - px
	case size > 2*px:
		return size
	default:
		return 10000 + px - size
	}
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// userIconTheme returns the name of the desktop's icon theme: KDE's on a
// Plasma desktop, otherwise GNOME's setting, then GTK's configuration files.
// gsettings answers with its default even where GNOME's settings are unused,
// so KDE's own file is read first there. It returns "" when none is set.
func userIconTheme() string {
	kde := strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE")
	if kde {
		if name := kdeIconTheme(); name != "" {
			return name
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), themeQueryTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "icon-theme").Output(); err == nil {
		if name := strings.Trim(strings.TrimSpace(string(out)), "'\""); name != "" {
			return name
		}
	}
	for _, d := range xdgConfigDirs() {
		for _, f := range []string{"gtk-4.0/settings.ini", "gtk-3.0/settings.ini"} {
			if name := iniValue(filepath.Join(d, f), "Settings", "gtk-icon-theme-name"); name != "" {
				return name
			}
		}
	}
	if !kde {
		return kdeIconTheme()
	}
	return ""
}

// kdeIconTheme returns the icon theme set in KDE's kdeglobals, or "".
func kdeIconTheme() string {
	for _, d := range xdgConfigDirs() {
		if name := iniValue(filepath.Join(d, "kdeglobals"), "Icons", "Theme"); name != "" {
			return name
		}
	}
	return ""
}

// iniValue returns key's value in section of an ini file, or "".
func iniValue(path, section, key string) string {
	var val, cur string
	eachLine(path, func(line string) {
		if val != "" {
			return
		}
		if line[0] == '[' && line[len(line)-1] == ']' {
			cur = line[1 : len(line)-1]
			return
		}
		if k, v, ok := strings.Cut(line, "="); ok && cur == section && strings.TrimSpace(k) == key {
			val = strings.Trim(strings.TrimSpace(v), "'\"")
		}
	})
	return val
}
