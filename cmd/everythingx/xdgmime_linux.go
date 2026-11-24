//go:build linux

package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file finds a file's MIME type the way the Linux file managers do, from
// the shared-mime-info database: glob patterns on the name first, then, for
// names no glob matches, the desktop's own resolver, which also applies the
// database's magic (content) rules.

const (
	mimeDirectory   = "inode/directory"
	mimeOctetStream = "application/octet-stream"
	mimeTextPlain   = "text/plain"
)

// sniffTimeout bounds the content-type helper. It runs as the menu opens.
const sniffTimeout = 2 * time.Second

// mimeDB is the part of the shared-mime-info database used here.
type mimeDB struct {
	globs   []mimeGlob
	parents map[string][]string // type -> its direct supertypes
	aliases map[string]string   // alias -> canonical type
	// genericIcons names the generic icon for a type, as its "generic-icon"
	// element does in the database, for themes without the type's own icon.
	genericIcons map[string]string
}

type mimeGlob struct {
	weight        int
	mime          string
	pattern       string
	caseSensitive bool
}

// The database is read once per set of data directories, which in practice
// means once per process. Tests point XDG_DATA_DIRS elsewhere.
var mimeCache struct {
	sync.Mutex
	key string
	db  *mimeDB
}

func currentMimeDB() *mimeDB {
	dirs := xdgDataDirs()
	key := strings.Join(dirs, "\x00")
	mimeCache.Lock()
	defer mimeCache.Unlock()
	if mimeCache.db == nil || mimeCache.key != key {
		mimeCache.db = loadMimeDB(dirs)
		mimeCache.key = key
	}
	return mimeCache.db
}

// loadMimeDB reads globs2, subclasses and aliases from each data directory's
// mime folder, most important directory first. Missing files are skipped.
func loadMimeDB(dataDirs []string) *mimeDB {
	db := &mimeDB{parents: map[string][]string{}, aliases: map[string]string{}, genericIcons: map[string]string{}}
	for _, d := range dataDirs {
		dir := filepath.Join(d, "mime")
		eachLine(filepath.Join(dir, "globs2"), func(line string) {
			// weight:type:pattern[:flags]
			f := strings.SplitN(line, ":", 4)
			if len(f) < 3 || f[2] == "__NOGLOBS__" {
				return
			}
			w, err := strconv.Atoi(f[0])
			if err != nil {
				return
			}
			g := mimeGlob{weight: w, mime: f[1], pattern: f[2]}
			if len(f) == 4 {
				for _, flag := range strings.Split(f[3], ",") {
					if flag == "cs" {
						g.caseSensitive = true
					}
				}
			}
			if !g.caseSensitive {
				g.pattern = strings.ToLower(g.pattern)
			}
			db.globs = append(db.globs, g)
		})
		eachLine(filepath.Join(dir, "subclasses"), func(line string) {
			if child, parent, ok := strings.Cut(line, " "); ok {
				if !slices.Contains(db.parents[child], parent) {
					db.parents[child] = append(db.parents[child], parent)
				}
			}
		})
		eachLine(filepath.Join(dir, "aliases"), func(line string) {
			if alias, canonical, ok := strings.Cut(line, " "); ok {
				if _, seen := db.aliases[alias]; !seen {
					db.aliases[alias] = canonical
				}
			}
		})
		eachLine(filepath.Join(dir, "generic-icons"), func(line string) {
			// type:icon-name
			if t, icon, ok := strings.Cut(line, ":"); ok {
				if _, seen := db.genericIcons[t]; !seen {
					db.genericIcons[t] = icon
				}
			}
		})
	}
	return db
}

// eachLine calls fn on each non-empty, non-comment line of the file.
func eachLine(path string, fn func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && line[0] != '#' {
			fn(line)
		}
	}
}

// globType returns the type whose glob best matches the file name: a
// case-sensitive pattern before any other, as xdgmime does, then the highest
// weight, then the longest pattern, so "*.tar.gz" beats "*.gz". It returns ""
// when nothing matches.
func (db *mimeDB) globType(name string) string {
	lower := strings.ToLower(name)
	best := -1
	for i, g := range db.globs {
		subject := lower
		if g.caseSensitive {
			subject = name
		}
		if ok, _ := filepath.Match(g.pattern, subject); !ok {
			continue
		}
		if best < 0 || globBetter(g, db.globs[best]) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return db.globs[best].mime
}

func globBetter(a, b mimeGlob) bool {
	if a.caseSensitive != b.caseSensitive {
		return a.caseSensitive
	}
	if a.weight != b.weight {
		return a.weight > b.weight
	}
	return len(a.pattern) > len(b.pattern)
}

func (db *mimeDB) canonical(t string) string {
	if c, ok := db.aliases[t]; ok {
		return c
	}
	return t
}

// typeChain returns t followed by all of its supertypes, nearest first: those
// listed in subclasses, text/plain for any text/*, and application/octet-stream
// for anything that is not an inode/*. Apps for any type in the chain can open
// the file, which is how a Go source file gets text editors.
func (db *mimeDB) typeChain(t string) []string {
	t = db.canonical(t)
	chain := []string{t}
	for i := 0; i < len(chain); i++ {
		cur := chain[i]
		parents := db.parents[cur]
		if strings.HasPrefix(cur, "text/") && cur != mimeTextPlain {
			parents = append(parents[:len(parents):len(parents)], mimeTextPlain)
		}
		if !strings.HasPrefix(cur, "inode/") && cur != mimeOctetStream {
			parents = append(parents[:len(parents):len(parents)], mimeOctetStream)
		}
		for _, p := range parents {
			p = db.canonical(p)
			if !slices.Contains(chain, p) {
				chain = append(chain, p)
			}
		}
	}
	// application/octet-stream is the most generic type; keep it last.
	if i := slices.Index(chain, mimeOctetStream); i >= 0 && i != len(chain)-1 {
		chain = append(append(chain[:i:i], chain[i+1:]...), mimeOctetStream)
	}
	return chain
}

// sniffMimeType asks the desktop for a file's type from its content. Tests
// swap it for a stub.
var sniffMimeType = desktopSniffMimeType

// fileMimeType returns the item's MIME type.
func fileMimeType(db *mimeDB, path string) string {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return mimeDirectory
	}
	if t := db.globType(filepath.Base(path)); t != "" {
		return db.canonical(t)
	}
	if t := sniffMimeType(path); t != "" {
		return db.canonical(t)
	}
	return mimeOctetStream
}

// desktopSniffMimeType runs gio, or failing that xdg-mime, both of which use
// shared-mime-info's magic rules, and returns "" if neither gives an answer.
func desktopSniffMimeType(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), sniffTimeout)
	defer cancel()

	if out, err := sniffCommand(ctx, "gio", "info", "-a", "standard::content-type", path); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if _, v, ok := strings.Cut(line, "standard::content-type:"); ok {
				if t := strings.TrimSpace(v); strings.Contains(t, "/") {
					return t
				}
			}
		}
	}
	if out, err := sniffCommand(ctx, "xdg-mime", "query", "filetype", path); err == nil {
		if t := strings.TrimSpace(out); strings.Contains(t, "/") && !strings.ContainsAny(t, " \n") {
			return t
		}
	}
	return ""
}

func sniffCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	return string(out), err
}
