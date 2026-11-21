package ffdb

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlanKK/everythingx/internal/shared"
)

func TestParseGlob_Literal(t *testing.T) {
	for _, term := range []string{"", "plain", "Movie [1080p].mkv", `a\*b`, `a\?`, "[abc]", "img[0-9].png", `\`} {
		if _, ok := parseGlob(term); ok {
			t.Errorf("parseGlob(%q) is a glob, want literal", term)
		}
	}
}

func TestParseGlob(t *testing.T) {
	cases := []struct {
		term, like, re string
		longest        int
	}{
		{"mypackage*.bb", "mypackage%.bb", `(?is)^(mypackage).*(\.bb)$`, 9},
		{"*.bb", "%.bb", `(?is)^.*(\.bb)$`, 3},
		{"a**b", "a%b", `(?is)^(a).*(b)$`, 1},
		{"img?.png", "img_.png", `(?is)^(img).(\.png)$`, 4},
		{"img[0-9]*.png", "img_%.png", `(?is)^(img)[0-9].*(\.png)$`, 4},
		{"[!a]*", "_%", `(?is)^[^a].*$`, 0},
		{"[^a-c]*", "_%", `(?is)^[^a-c].*$`, 0},
		{"[]x]*", "_%", `(?is)^[\]x].*$`, 0},
		{"a_*", "a_%", `(?is)^(a_).*$`, 1},
		{"100%*", "100_%", `(?is)^(100%).*$`, 3},
		{`\**`, "*%", `(?is)^(\*).*$`, 1},
		{`a\[b*`, "a[b%", `(?is)^(a\[b).*$`, 3},
		{`[ab*`, "[ab%", `(?is)^(\[ab).*$`, 3},
		{`*\\`, `%\`, `(?is)^.*(\\)$`, 1},
		{"[z-a]*", "_%", `(?is)^[^\x{0}-\x{10FFFF}].*$`, 0},
	}
	for _, c := range cases {
		g, ok := parseGlob(c.term)
		if !ok {
			t.Errorf("parseGlob(%q) is literal, want a glob", c.term)
			continue
		}
		if g.like != c.like || g.re.String() != c.re || g.longestLiteral != c.longest {
			t.Errorf("parseGlob(%q) = like %q re %q longest %d, want %q %q %d",
				c.term, g.like, g.re, g.longestLiteral, c.like, c.re, c.longest)
		}
	}
}

func TestParseGlob_Matches(t *testing.T) {
	cases := []struct {
		term string
		yes  []string
		no   []string
	}{
		{"mypackage*.bb", []string{"mypackage_1.0.bb", "MyPackage.BB"}, []string{"mypackage_1.0.bbappend", "mypackage.bb.lock", "xmypackage.bb"}},
		{"?.txt", []string{"a.txt", "日.txt"}, []string{".txt", "ab.txt"}},
		{"[!a]*", []string{"b", "B1"}, []string{"a", "Abc"}},
		{"img[0-9]*.png", []string{"img3.png", "img10.png"}, []string{"imgx.png", "img3.jpg"}},
		{`\**`, []string{"*star"}, []string{"star"}},
		{"*", []string{"anything", "line\nbreak"}, nil},
	}
	for _, c := range cases {
		g, _ := parseGlob(c.term)
		for _, n := range c.yes {
			if !g.re.MatchString(n) {
				t.Errorf("%q should match %q", c.term, n)
			}
		}
		for _, n := range c.no {
			if g.re.MatchString(n) {
				t.Errorf("%q should not match %q", c.term, n)
			}
		}
	}
}

func TestHighlightSpans(t *testing.T) {
	cases := []struct {
		name, term string
		want       [][2]int
	}{
		{"mypackage_1.0.bb", "mypackage*.bb", [][2]int{{0, 9}, {13, 16}}},
		{"MyPackage.BB", "mypackage*.bb", [][2]int{{0, 9}, {9, 12}}},
		{"photo.jpg", "*.JPG", [][2]int{{5, 9}}},
		{"img3.png", "img[0-9]*.png", [][2]int{{0, 3}, {4, 8}}},
		{"anything", "*", [][2]int{}},
		{"photo.jpg", "photo", nil},              // literal: callers use SplitFileName
		{"photo.jpg", "*.png", [][2]int{}},       // no match
		{"photo.jpg", "＊.jpg", [][2]int{{5, 9}}}, // fullwidth wildcard
	}
	for _, c := range cases {
		if got := HighlightSpans(c.name, c.term); !reflect.DeepEqual(got, c.want) {
			t.Errorf("HighlightSpans(%q, %q) = %v, want %v", c.name, c.term, got, c.want)
		}
	}
}

func TestSplitBySpans(t *testing.T) {
	got := SplitBySpans("mypackage_1.0.bb", [][2]int{{0, 9}, {13, 16}})
	want := []string{"", "mypackage", "_1.0", ".bb", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SplitBySpans = %q, want %q", got, want)
	}
	if got := SplitBySpans("abc", nil); !reflect.DeepEqual(got, []string{"abc"}) {
		t.Errorf("SplitBySpans with no spans = %q", got)
	}
}

func globDB(t *testing.T, names ...string) *sql.DB {
	t.Helper()
	db, err := CreateDB(filepath.Join(t.TempDir(), "glob.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, n := range names {
		if err = InsertRecord(db, n, "/x/"+n, 0, shared.ItemIsFile); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestSearch_Glob(t *testing.T) {
	globDB(t,
		"mypackage_1.0.bb", "mypackage_1.0.bbappend", "mypackage.bb.lock", "xmypackage.bb",
		"other.bb", "photo.jpg", "PHOTO2.JPG", "a.txt", "日.txt", "ab.txt",
		"img3.png", "img10.png", "abc", "a_1", "*star", "star", "Movie [1080p].mkv",
	)

	// Whole-name rule, FTS path.
	assertNames(t, "mypackage*.bb", "mypackage_1.0.bb")
	// FTS path: ".bb" is a 3-character run.
	assertNames(t, "*.bb", "mypackage_1.0.bb", "xmypackage.bb", "other.bb")
	assertNames(t, "?.txt", "a.txt", "日.txt")
	assertNames(t, "img[0-9]*.png", "img3.png", "img10.png")
	assertNames(t, "img[0-9].png")              // no "*" or "?": a literal search
	assertNames(t, "[!a-z]*", "日.txt", "*star") // classes ignore case too
	// Case-insensitive, on both paths, and a fullwidth wildcard.
	assertNames(t, "*.JPG", "photo.jpg", "PHOTO2.JPG")
	assertNames(t, "photo*", "photo.jpg", "PHOTO2.JPG")
	assertNames(t, "＊.jpg", "photo.jpg", "PHOTO2.JPG")
	// The loose LIKE ("_" for a literal "_") is filtered exactly.
	assertNames(t, "a_*", "a_1")
	assertNames(t, `\**`, "*star")
	// A term with no unescaped "*" or "?" is still a literal search.
	assertNames(t, "Movie [1080p].mkv", "Movie [1080p].mkv")
	assertNames(t, "[1080p]", "Movie [1080p].mkv")
}

func TestSearch_GlobLimitAndOrder(t *testing.T) {
	globDB(t, "c.z", "a.z", "e.z", "b.z", "d.z",
		"c.log", "a.log", "e.log", "b.log", "d.log", "x.txt")
	cases := []struct {
		term string
		fts  bool
		want []string
	}{
		{"*.z", false, []string{"a.z", "b.z", "c.z"}},
		{"?.z", false, []string{"a.z", "b.z", "c.z"}},
		{"*.log", true, []string{"a.log", "b.log", "c.log"}},
		{"?.log", true, []string{"a.log", "b.log", "c.log"}},
	}
	for _, c := range cases {
		if g, _ := parseGlob(c.term); (g.longestLiteral >= 3) != c.fts {
			t.Fatalf("%q: longestLiteral %d, want FTS path %v", c.term, g.longestLiteral, c.fts)
		}
		results, err := PrefixSearch(c.term, 3)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, r := range results {
			got = append(got, filepath.Base(r.Fullpath))
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("PrefixSearch(%q, 3) = %q, want %q", c.term, got, c.want)
		}
	}
	if results, _ := PrefixSearch("*.z", 0); len(results) != 0 {
		t.Errorf("limit 0 returned %d results", len(results))
	}
	if results, _ := PrefixSearch("*.z", -1); len(results) != 5 {
		t.Errorf("limit -1 returned %d results, want 5", len(results))
	}
}

// A glob with a 3-character literal run is answered through the FTS index.
func TestSearch_GlobFTSPlan(t *testing.T) {
	db := globDB(t, "a.txt")
	rows, err := db.Query("EXPLAIN QUERY PLAN "+globFTSSQL("search_name"), "mypackage%.bb")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "; "
	}
	// ":L" means the LIKE constraint is handed to FTS5, not checked row by row.
	if !strings.Contains(plan, "VIRTUAL TABLE INDEX") || !strings.Contains(plan, ":L") {
		t.Errorf("plan %q does not pass the LIKE to the FTS index", plan)
	}
}

// On a database the daemon has not migrated, globs search the filename column.
func TestSearch_GlobUnmigrated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	createPreviousSchema(t, path, "mypackage_1.0.bb", "mypackage.bb.lock", "a.c", "b.h")
	ro, err := OpenDBReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if searchNameEnabled {
		t.Fatal("search_name enabled on an unmigrated database")
	}
	assertNames(t, "mypackage*.bb", "mypackage_1.0.bb")
	assertNames(t, "*.c", "a.c")
}
