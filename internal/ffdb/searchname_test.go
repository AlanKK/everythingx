package ffdb

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// A reader that opened the database before the daemon migrated it must start
// normalizing terms once the migration lands, without restarting.
func TestSearch_ReaderPicksUpMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.db")
	const full, ascii = "Ｒｅｐｏｒｔ ２０２６.pdf", "Report 2026.pdf"
	createPreviousSchema(t, path, full, ascii)

	ro, err := OpenDBReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	assertNames(t, "Ｒｅｐｏｒｔ", full)

	// The daemon migrates through its own handle. It must not touch the
	// package's prepared statements, as a separate process would not.
	daemon, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	migrateSearchName(daemon)
	daemon.Close()

	assertNames(t, "Ｒｅｐｏｒｔ", full, ascii) // trigram path
	assertNames(t, "Ｒｅ", full, ascii)     // prefix path
	if !searchNameEnabled {
		t.Fatal("search_name not enabled after the schema changed")
	}
}

// Trigram needs 3 characters; a 2-character CJK term (6 bytes) must take the
// prefix path.
func TestSearch_TwoCharacterCJKTerm(t *testing.T) {
	db, err := CreateDB(filepath.Join(t.TempDir(), "cjk.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, n := range []string{"カタカナ.zip", "ｶﾀｶﾅ.txt", "ひらがな.zip"} {
		if err = InsertRecord(db, n, "/x/"+n, 0, shared.ItemIsFile); err != nil {
			t.Fatal(err)
		}
	}
	assertNames(t, "カタ", "カタカナ.zip", "ｶﾀｶﾅ.txt")
	assertNames(t, "ｶﾀ", "カタカナ.zip", "ｶﾀｶﾅ.txt")
}

// LIKE wildcards in a short term, including fullwidth ones that NFKC turns
// into ASCII, match literally.
func TestSearch_ShortTermEscapesWildcards(t *testing.T) {
	db, err := CreateDB(filepath.Join(t.TempDir(), "esc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, n := range []string{"_a.txt", "ba.txt", "%x.txt", "yx.txt", `\z.txt`} {
		if err = InsertRecord(db, n, "/x/"+n, 0, shared.ItemIsFile); err != nil {
			t.Fatal(err)
		}
	}
	assertNames(t, "_a", "_a.txt")
	assertNames(t, "＿a", "_a.txt")
	assertNames(t, "％", "%x.txt")
	assertNames(t, `\`, `\z.txt`)
}

// The short-term search must stay a covering scan of the filename-ordered
// index, on a new database and on a migrated one alike.
func TestSearch_PrefixPlanIsCovering(t *testing.T) {
	dir := t.TempDir()
	fresh, err := CreateDB(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	oldPath := filepath.Join(dir, "old.db")
	createPreviousSchema(t, oldPath, "a.txt")
	migrated, err := OpenDB(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()

	for name, db := range map[string]*sql.DB{"fresh": fresh, "migrated": migrated} {
		for _, col := range []string{"search_name", "filename"} {
			rows, err := db.Query("EXPLAIN QUERY PLAN "+prefixSearchSQL(col), "a%", 10)
			if err != nil {
				t.Fatal(err)
			}
			var plan string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan += detail + "; "
			}
			rows.Close()
			if !strings.Contains(plan, "COVERING INDEX idx_filename_search") {
				t.Errorf("%s db, %s: plan %q is not a covering scan of idx_filename_search", name, col, plan)
			}
		}
		var indexes []string
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'files' AND sql IS NOT NULL ORDER BY name")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n string
			rows.Scan(&n)
			indexes = append(indexes, n)
		}
		rows.Close()
		if strings.Join(indexes, ",") != "idx_filename_search" {
			t.Errorf("%s db indexes = %v, want [idx_filename_search]", name, indexes)
		}
	}
}

// oldRenameDirSQL is the directory rename of builds before search_name: it
// updates filename but leaves search_name as it was.
const oldRenameDirSQL = "UPDATE files SET fullpath = ? || SUBSTR(fullpath, LENGTH(?) + 1), filename = CASE WHEN fullpath = ? THEN ? ELSE filename END WHERE fullpath = ? OR (fullpath > ? AND fullpath < ?)"

// A directory renamed by an older binary after a rollback keeps the
// search_name of its old filename; the next start corrects it (#132).
func TestMigrateSearchName_RepairsRenameByOlderBinary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rename.db")
	db, err := CreateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	oldDir, newDir := filepath.Join(root, "old"), filepath.Join(root, "new")
	scan := []shared.EventRecord{
		{Filename: "old", Path: oldDir, ObjectType: shared.ItemIsDir, FoundOnScan: true},
		{Filename: "child.txt", Path: oldDir + "/child.txt", ObjectType: shared.ItemIsFile, FoundOnScan: true},
	}
	if err = BulkStoreEvents(db, &scan); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// The older binary renames old to new.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(oldRenameDirSQL, newDir, oldDir, oldDir, "new", oldDir, oldDir+"/", oldDir+"0"); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err = OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertNames(t, "new", "new")
	assertNames(t, "old")
	assertNames(t, "child.txt", "child.txt")
	var fullpath string
	if err = db.QueryRow("SELECT fullpath FROM files WHERE search_name = 'new'").Scan(&fullpath); err != nil || fullpath != newDir {
		t.Fatalf("renamed dir fullpath = %q (%v), want %q", fullpath, err, newDir)
	}
	assertFTSIntact(t, db)
}

// Rows whose search_name is correct but differs from filename (names NFKC
// changes) are checked on every start but not rewritten.
func TestMigrateSearchName_DoesNotRewriteCorrectRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.db")
	db, err := CreateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertRecord(db, nameHalfComma, "/x/"+nameHalfComma, 0, shared.ItemIsFile); err != nil {
		t.Fatal(err)
	}
	if err = InsertRecord(db, "plain.zip", "/x/plain.zip", 0, shared.ItemIsFile); err != nil {
		t.Fatal(err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	n, err := fillSearchName(tx, staleSearchNameWhere)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("fillSearchName rewrote %d correct rows", n)
	}
	db.Close()
}

// The stale-row filter must stay a scan of the covering index, not the table.
func TestStaleSearchNameWhere_UsesCoveringIndex(t *testing.T) {
	db, err := CreateDB(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("EXPLAIN QUERY PLAN SELECT rowid, filename, search_name FROM files " + staleSearchNameWhere)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "COVERING INDEX idx_filename_search") {
		t.Fatalf("plan does not use the covering index: %q", plan)
	}
}
