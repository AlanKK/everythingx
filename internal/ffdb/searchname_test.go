package ffdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/AlanKK/everythingx/internal/shared"
)

// Names that differ only in characters NFKC folds together.
const (
	nameFullComma = "春の庭、小さな約束 v01.zip" // U+3001
	nameHalfComma = "春の庭､小さな約束 v02.zip" // U+FF64
)

func searchNames(t *testing.T, term string) []string {
	t.Helper()
	results, err := PrefixSearch(term, 100)
	if err != nil {
		t.Fatalf("search %q: %v", term, err)
	}
	var names []string
	for _, r := range results {
		names = append(names, filepath.Base(r.Fullpath))
	}
	sort.Strings(names)
	return names
}

func assertNames(t *testing.T, term string, want ...string) {
	t.Helper()
	got := searchNames(t, term)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("search %q: got %q, want %q", term, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("search %q: got %q, want %q", term, got, want)
		}
	}
}

// assertFTSIntact checks the FTS index against the content table; it fails
// if a trigger deleted different tokens from the ones it inserted.
func assertFTSIntact(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO files_fts(files_fts, rank) VALUES('integrity-check', 1)"); err != nil {
		t.Fatalf("FTS integrity check failed: %v", err)
	}
}

func TestSearchName(t *testing.T) {
	cases := map[string]string{
		"春の庭､小さな":     "春の庭、小さな",
		"ｶﾀｶﾅ":        "カタカナ",
		"Ｒｅｐｏｒｔ ２０２６": "Report 2026",
		"cafe\u0301":  "caf\u00e9", // decomposed (NFD) -> composed
		"plain.zip":   "plain.zip",
	}
	for in, want := range cases {
		if got := SearchName(in); got != want {
			t.Errorf("SearchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearch_MatchesAcrossWidthVariants(t *testing.T) {
	db, err := CreateDB(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	events := []shared.EventRecord{
		{Filename: nameFullComma, Path: "/x/" + nameFullComma, ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: nameHalfComma, Path: "/x/" + nameHalfComma, ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: "ｶﾀｶﾅ.zip", Path: "/x/ｶﾀｶﾅ.zip", ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: "Report 2026.pdf", Path: "/x/Report 2026.pdf", ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: "Ｒｅｐｏｒｔ ２０２６.pdf", Path: "/x/Ｒｅｐｏｒｔ ２０２６.pdf", ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: "caf\u00e9 menu.txt", Path: "/x/caf\u00e9 menu.txt", ObjectType: shared.ItemIsFile, FoundOnScan: true},
		{Filename: "cafe\u0301 menu.txt", Path: "/x/cafe\u0301 menu.txt", ObjectType: shared.ItemIsFile, FoundOnScan: true},
	}
	if err = BulkStoreEvents(db, &events); err != nil {
		t.Fatal(err)
	}

	assertNames(t, "春の庭、小さな", nameFullComma, nameHalfComma)
	assertNames(t, "春の庭､小さな", nameFullComma, nameHalfComma)
	assertNames(t, "カタカナ", "ｶﾀｶﾅ.zip")
	assertNames(t, "Report 2026", "Report 2026.pdf", "Ｒｅｐｏｒｔ ２０２６.pdf")
	assertNames(t, "report", "Report 2026.pdf", "Ｒｅｐｏｒｔ ２０２６.pdf")
	assertNames(t, "Re", "Report 2026.pdf", "Ｒｅｐｏｒｔ ２０２６.pdf") // short term: prefix search
	assertNames(t, "caf\u00e9", "caf\u00e9 menu.txt", "cafe\u0301 menu.txt")
	assertNames(t, "cafe\u0301", "caf\u00e9 menu.txt", "cafe\u0301 menu.txt")
	// The stored filename keeps its original spelling for display.
	var stored string
	if err = db.QueryRow("SELECT filename FROM files WHERE fullpath = ?", "/x/"+nameHalfComma).Scan(&stored); err != nil || stored != nameHalfComma {
		t.Fatalf("stored filename = %q (%v), want %q", stored, err, nameHalfComma)
	}
	assertFTSIntact(t, db)
}

func TestSearch_DirRenameUpdatesSearchName(t *testing.T) {
	root := t.TempDir()
	db, err := CreateDB(filepath.Join(root, "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	oldDir := filepath.Join(root, "old_ｶﾞ")
	newDir := filepath.Join(root, "春の庭､小さな約束")
	scan := []shared.EventRecord{
		{Filename: filepath.Base(oldDir), Path: oldDir, ObjectType: shared.ItemIsDir, FoundOnScan: true},
		{Filename: "v01.zip", Path: oldDir + "/v01.zip", ObjectType: shared.ItemIsFile, FoundOnScan: true},
	}
	if err = BulkStoreEvents(db, &scan); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(newDir, 0o755); err != nil { // the rename has happened on disk
		t.Fatal(err)
	}
	rename := []shared.EventRecord{
		{Filename: filepath.Base(oldDir), Path: oldDir, ObjectType: shared.ItemIsDir, IsRename: true},
		{Filename: filepath.Base(newDir), Path: newDir, ObjectType: shared.ItemIsDir, IsRename: true},
	}
	if err = BulkStoreEvents(db, &rename); err != nil {
		t.Fatal(err)
	}

	assertNames(t, "春の庭、小さな", filepath.Base(newDir))
	assertNames(t, "old_ガ")
	assertNames(t, "v01.zip", "v01.zip")
	assertFTSIntact(t, db)
}

// createPreviousSchema builds the schema written by builds before search_name.
func createPreviousSchema(t *testing.T, path string, names ...string) {
	t.Helper()
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, s := range []string{
		"CREATE TABLE files(filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER)",
		"CREATE INDEX idx_filename ON files(filename COLLATE BINARY, fullpath, object_type)",
		"CREATE VIRTUAL TABLE files_fts USING fts5(filename, content='files', content_rowid='rowid', tokenize='trigram')",
		"CREATE TRIGGER files_ai AFTER INSERT ON files BEGIN INSERT INTO files_fts(rowid, filename) VALUES (new.rowid, new.filename); END",
		"CREATE TRIGGER files_ad AFTER DELETE ON files BEGIN INSERT INTO files_fts(files_fts, rowid, filename) VALUES('delete', old.rowid, old.filename); END",
		"CREATE TRIGGER files_au AFTER UPDATE ON files BEGIN INSERT INTO files_fts(files_fts, rowid, filename) VALUES('delete', old.rowid, old.filename); INSERT INTO files_fts(rowid, filename) VALUES (new.rowid, new.filename); END",
	} {
		if _, err = raw.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range names {
		if _, err = raw.Exec("INSERT INTO files (filename, fullpath, object_type) VALUES (?, ?, 0)", n, "/x/"+n); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateSearchName_FromPreviousSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	createPreviousSchema(t, path, nameFullComma, nameHalfComma, "plain.zip")

	// Before the daemon migrates, a read-only client still searches as before.
	ro, err := OpenDBReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	if searchNameEnabled {
		t.Fatal("search_name enabled on an unmigrated database")
	}
	assertNames(t, "春の庭、小さな", nameFullComma)
	ro.Close()

	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if !searchNameEnabled {
		t.Fatal("search_name not enabled after migration")
	}
	assertNames(t, "春の庭、小さな", nameFullComma, nameHalfComma)
	assertNames(t, "plain", "plain.zip")
	assertFTSIntact(t, db)
	db.Close()

	// Opening again is a no-op, and a migrated database works read-only.
	db, err = OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	assertFTSIntact(t, db)
	db.Close()
	ro, err = OpenDBReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	assertNames(t, "春の庭､小さな", nameFullComma, nameHalfComma)
}

// A row written by an older binary after a rollback has an empty search_name;
// the next start fills it in.
func TestMigrateSearchName_BackfillsEmptyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.db")
	db, err := CreateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO files (filename, fullpath, object_type) VALUES (?, ?, 0)", nameHalfComma, "/x/"+nameHalfComma); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertNames(t, "春の庭、小さな", nameHalfComma)
	assertFTSIntact(t, db)
}
