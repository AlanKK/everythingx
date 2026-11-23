//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// This file lists the applications for a MIME type the way the Linux file
// managers do, following the freedesktop.org specifications:
//   - MIME Applications Associations: mimeapps.list and mimeinfo.cache
//   - Desktop Entry: the .desktop files, including the Exec line

// desktopEntry is an application's .desktop file, reduced to what Open With
// needs.
type desktopEntry struct {
	ID        string // desktop-file id, e.g. "org.gnome.TextEditor.desktop"
	Path      string
	Name      string // localised
	Exec      string // with string escapes already undone
	Icon      string
	WorkDir   string
	MimeTypes []string

	valid      bool // an Application with a Name and Exec, not Hidden or Terminal
	onlyShowIn []string
	notShowIn  []string
	tryExec    string
}

// usable reports whether the app can be offered here and now. The desktop and
// TryExec checks are made on each call, not when the file is parsed, since
// parsed entries are cached.
func (e *desktopEntry) usable() bool {
	return e.valid && showInCurrentDesktop(e.onlyShowIn, e.notShowIn) && tryExecOK(e.tryExec)
}

// --- XDG base directories ---

func xdgDataDirs() []string {
	return xdgDirs("XDG_DATA_HOME", ".local/share", "XDG_DATA_DIRS", "/usr/local/share:/usr/share")
}

func xdgConfigDirs() []string {
	return xdgDirs("XDG_CONFIG_HOME", ".config", "XDG_CONFIG_DIRS", "/etc/xdg")
}

// xdgDirs returns the user directory followed by the system ones, most
// important first. Relative paths are invalid in these variables and ignored.
func xdgDirs(homeVar, homeDefault, sysVar, sysDefault string) []string {
	var dirs []string
	home := os.Getenv(homeVar)
	if !filepath.IsAbs(home) {
		home = ""
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, homeDefault)
		}
	}
	if home != "" {
		dirs = append(dirs, home)
	}
	sys := os.Getenv(sysVar)
	if sys == "" {
		sys = sysDefault
	}
	for _, d := range filepath.SplitList(sys) {
		if filepath.IsAbs(d) && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func applicationDirs() []string {
	var dirs []string
	for _, d := range xdgDataDirs() {
		dirs = append(dirs, filepath.Join(d, "applications"))
	}
	return dirs
}

// currentDesktops returns the names in XDG_CURRENT_DESKTOP, e.g. "ubuntu",
// "GNOME".
func currentDesktops() []string {
	var names []string
	for _, n := range strings.Split(os.Getenv("XDG_CURRENT_DESKTOP"), ":") {
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

// --- mimeapps.list ---

type mimeappsList struct {
	defaults, added, removed map[string][]string
}

// mimeappsListPaths returns the mimeapps.list files in the order the spec says
// to consult them: config dirs then data application dirs, each with the
// desktop-specific "<desktop>-mimeapps.list" before the plain one.
func mimeappsListPaths() []string {
	dirs := append(xdgConfigDirs(), applicationDirs()...)
	var paths []string
	for _, d := range dirs {
		for _, desk := range currentDesktops() {
			paths = append(paths, filepath.Join(d, strings.ToLower(desk)+"-mimeapps.list"))
		}
		paths = append(paths, filepath.Join(d, "mimeapps.list"))
	}
	return paths
}

func readMimeappsList(path string, db *mimeDB) *mimeappsList {
	l := &mimeappsList{defaults: map[string][]string{}, added: map[string][]string{}, removed: map[string][]string{}}
	var section map[string][]string
	eachLine(path, func(line string) {
		if strings.HasPrefix(line, "[") {
			switch line {
			case "[Default Applications]":
				section = l.defaults
			case "[Added Associations]":
				section = l.added
			case "[Removed Associations]":
				section = l.removed
			default:
				section = nil
			}
			return
		}
		if section == nil {
			return
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return
		}
		t := db.canonical(strings.TrimSpace(k))
		section[t] = append(section[t], splitList(strings.TrimSpace(v))...)
	})
	return l
}

// --- desktop files ---

// appIndex maps each desktop-file id to the file that defines it: the one in
// the most important applications directory. It also records which ids each
// directory holds, for directories without a mimeinfo.cache.
type appIndex struct {
	dirs  []string
	path  map[string]string        // id -> file
	inDir map[string][]desktopFile // dir -> files found there, in walk order
}

type desktopFile struct{ id, path string }

func scanApplications(dirs []string) *appIndex {
	idx := &appIndex{dirs: dirs, path: map[string]string{}, inDir: map[string][]desktopFile{}}
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".desktop") {
				return nil
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return nil
			}
			id := strings.ReplaceAll(rel, string(filepath.Separator), "-")
			idx.inDir[dir] = append(idx.inDir[dir], desktopFile{id, p})
			if _, seen := idx.path[id]; !seen {
				idx.path[id] = p
			}
			return nil
		})
	}
	return idx
}

// entry returns the parsed, usable entry for id, or nil.
func (idx *appIndex) entry(id string) *desktopEntry {
	p, ok := idx.path[id]
	if !ok {
		return nil
	}
	e, err := loadDesktopEntry(p, id)
	if err != nil || !e.usable() {
		return nil
	}
	return e
}

// mimeCacheFor returns the type -> ids map for one applications directory,
// from its mimeinfo.cache, or by reading its desktop files when there is none.
func (idx *appIndex) mimeCacheFor(dir string, db *mimeDB) map[string][]string {
	m := map[string][]string{}
	cache := filepath.Join(dir, "mimeinfo.cache")
	if _, err := os.Stat(cache); err == nil {
		inSection := false
		eachLine(cache, func(line string) {
			if strings.HasPrefix(line, "[") {
				inSection = line == "[MIME Cache]"
				return
			}
			if k, v, ok := strings.Cut(line, "="); ok && inSection {
				t := db.canonical(k)
				m[t] = append(m[t], splitList(v)...)
			}
		})
		return m
	}
	for _, f := range idx.inDir[dir] {
		e, err := loadDesktopEntry(f.path, f.id)
		if err != nil {
			continue
		}
		for _, t := range e.MimeTypes {
			t = db.canonical(t)
			m[t] = append(m[t], f.id)
		}
	}
	return m
}

// xdgAppsFor lists the usable apps for the types in chain, most specific type
// first, and the default app, which is also in the list. def is nil when the
// list is empty.
func xdgAppsFor(db *mimeDB, chain []string) (apps []*desktopEntry, def *desktopEntry) {
	idx := scanApplications(applicationDirs())
	var lists []*mimeappsList
	for _, p := range mimeappsListPaths() {
		lists = append(lists, readMimeappsList(p, db))
	}
	caches := make([]map[string][]string, len(idx.dirs))
	for i, dir := range idx.dirs {
		caches[i] = idx.mimeCacheFor(dir, db)
	}

	seen := map[string]bool{}
	add := func(id string) {
		if seen[id] {
			return
		}
		if e := idx.entry(id); e != nil {
			seen[id] = true
			apps = append(apps, e)
		}
	}

	// The default is the first usable app named under Default Applications,
	// for the most specific type that has one.
findDefault:
	for _, t := range chain {
		for _, l := range lists {
			for _, id := range l.defaults[t] {
				if e := idx.entry(id); e != nil {
					def = e
					break findDefault
				}
			}
		}
	}
	if def != nil {
		add(def.ID)
	}

	for _, t := range chain {
		removed := map[string]bool{}
		for _, l := range lists {
			for _, id := range l.added[t] {
				if !removed[id] {
					add(id)
				}
			}
			for _, id := range l.removed[t] {
				removed[id] = true
			}
		}
		for _, c := range caches {
			for _, id := range c[t] {
				if !removed[id] {
					add(id)
				}
			}
		}
	}
	if def == nil && len(apps) > 0 {
		def = apps[0]
	}
	return apps, def
}

// Parsed entries are kept by path and reparsed when the file changes, so a
// directory without a mimeinfo.cache is read in full only once.
var entryCache struct {
	sync.Mutex
	m map[string]cachedEntry
}

type cachedEntry struct {
	mod   time.Time
	size  int64
	entry *desktopEntry
}

func loadDesktopEntry(path, id string) (*desktopEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	entryCache.Lock()
	c, ok := entryCache.m[path]
	entryCache.Unlock()
	if ok && c.mod.Equal(info.ModTime()) && c.size == info.Size() && c.entry.ID == id {
		return c.entry, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	e := parseDesktopEntry(string(data), path, id)
	entryCache.Lock()
	if entryCache.m == nil {
		entryCache.m = map[string]cachedEntry{}
	}
	entryCache.m[path] = cachedEntry{mod: info.ModTime(), size: info.Size(), entry: e}
	entryCache.Unlock()
	return e, nil
}

// parseDesktopEntry reads the [Desktop Entry] group and decides whether the
// app can be offered here.
func parseDesktopEntry(data, path, id string) *desktopEntry {
	keys := map[string]string{}
	inGroup := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			inGroup = line == "[Desktop Entry]"
			continue
		}
		if !inGroup {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			if _, dup := keys[k]; !dup {
				keys[k] = strings.TrimSpace(v)
			}
		}
	}

	e := &desktopEntry{
		ID:        id,
		Path:      path,
		Name:      unescapeString(localizedKey(keys, "Name")),
		Exec:      unescapeString(keys["Exec"]),
		Icon:      unescapeString(keys["Icon"]),
		WorkDir:   unescapeString(keys["Path"]),
		MimeTypes: splitList(keys["MimeType"]),

		onlyShowIn: splitList(keys["OnlyShowIn"]),
		notShowIn:  splitList(keys["NotShowIn"]),
		tryExec:    unescapeString(keys["TryExec"]),
	}
	e.valid = keys["Type"] == "Application" &&
		keys["Hidden"] != "true" &&
		keys["Terminal"] != "true" &&
		e.Name != "" && e.Exec != ""
	return e
}

func showInCurrentDesktop(only, not []string) bool {
	desktops := currentDesktops()
	matches := func(list []string) bool {
		for _, d := range desktops {
			for _, l := range list {
				if strings.EqualFold(d, l) {
					return true
				}
			}
		}
		return false
	}
	if len(only) > 0 && !matches(only) {
		return false
	}
	return !matches(not)
}

// tryExecOK reports whether the TryExec program, if any, is installed.
func tryExecOK(prog string) bool {
	if prog == "" {
		return true
	}
	_, err := exec.LookPath(prog)
	return err == nil
}

// localizedKey returns key for the user's locale, trying lang_COUNTRY@MODIFIER,
// lang_COUNTRY, lang@MODIFIER and lang before the plain key.
func localizedKey(keys map[string]string, key string) string {
	for _, loc := range localeVariants() {
		if v, ok := keys[key+"["+loc+"]"]; ok {
			return v
		}
	}
	return keys[key]
}

func localeVariants() []string {
	var loc string
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if loc = os.Getenv(v); loc != "" {
			break
		}
	}
	if loc == "" || loc == "C" || loc == "POSIX" || strings.HasPrefix(loc, "C.") {
		return nil
	}
	loc, mod, _ := strings.Cut(loc, "@")
	loc, _, _ = strings.Cut(loc, ".")
	lang, country, _ := strings.Cut(loc, "_")
	var out []string
	if country != "" && mod != "" {
		out = append(out, lang+"_"+country+"@"+mod)
	}
	if country != "" {
		out = append(out, lang+"_"+country)
	}
	if mod != "" {
		out = append(out, lang+"@"+mod)
	}
	return append(out, lang)
}

// unescapeString undoes the escapes of the Desktop Entry string type.
func unescapeString(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// splitList splits a ";"-separated value, dropping empty items.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ";") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// --- Exec ---

var errBadExec = errors.New("invalid Exec line")

// execArgs turns the entry's Exec line into the command for opening path, per
// the Desktop Entry spec's quoting rules and field codes. Field codes inside a
// quoted argument are left alone: the spec leaves them undefined, and
// expanding them there would put the file name into strings such as
// `sh -c "…"`. When the line takes no file, the path is appended, as GLib does.
func execArgs(e *desktopEntry, path string) ([]string, error) {
	tokens, err := splitExec(e.Exec)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.ID, err)
	}
	var args []string
	usedFile := false
	for _, tok := range tokens {
		if tok.quoted {
			args = append(args, tok.text)
			continue
		}
		switch tok.text {
		case "%f", "%F":
			args = append(args, path)
			usedFile = true
			continue
		case "%u", "%U":
			args = append(args, fileURI(path))
			usedFile = true
			continue
		case "%i":
			if e.Icon != "" {
				args = append(args, "--icon", e.Icon)
			}
			continue
		}
		var b strings.Builder
		hadCode := false
		for i := 0; i < len(tok.text); i++ {
			c := tok.text[i]
			if c != '%' || i+1 == len(tok.text) {
				b.WriteByte(c)
				continue
			}
			i++
			hadCode = true
			switch tok.text[i] {
			case '%':
				b.WriteByte('%')
			case 'f', 'F':
				b.WriteString(path)
				usedFile = true
			case 'u', 'U':
				b.WriteString(fileURI(path))
				usedFile = true
			case 'c':
				b.WriteString(e.Name)
			case 'k':
				b.WriteString(e.Path)
			case 'i':
				b.WriteString(e.Icon)
			}
			// %d %D %n %N %v %m are deprecated and, like unknown codes, removed.
		}
		if b.Len() == 0 && hadCode {
			continue
		}
		args = append(args, b.String())
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("%s: %w: empty", e.ID, errBadExec)
	}
	if !usedFile {
		args = append(args, path)
	}
	return args, nil
}

type execToken struct {
	text   string
	quoted bool
}

// splitExec splits an Exec value into arguments. Arguments are separated by
// spaces; a double-quoted one may contain spaces, and within quotes a
// backslash escapes `"`, "`", "$" and "\".
func splitExec(s string) ([]execToken, error) {
	var tokens []execToken
	var cur strings.Builder
	inToken, quoted, inQuote := false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote && c == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\", s[i+1]) >= 0:
			i++
			cur.WriteByte(s[i])
		case c == '"':
			inQuote = !inQuote
			inToken, quoted = true, true
		case !inQuote && (c == ' ' || c == '\t'):
			if inToken {
				tokens = append(tokens, execToken{cur.String(), quoted})
				cur.Reset()
				inToken, quoted = false, false
			}
		default:
			cur.WriteByte(c)
			inToken = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("%w: unterminated quote", errBadExec)
	}
	if inToken {
		tokens = append(tokens, execToken{cur.String(), quoted})
	}
	return tokens, nil
}
