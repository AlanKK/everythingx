package ffdb

import (
	"database/sql"
	"log"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SearchName returns the form of a filename (or search term) used for
// matching. NFKC folds characters that look alike but differ in code point,
// such as the halfwidth ideographic comma "､" (U+FF64) and "、" (U+3001),
// halfwidth and fullwidth katakana, and fullwidth and ASCII letters, so a
// search finds every spelling. The stored filename is left as it is for
// display.
func SearchName(s string) string {
	return norm.NFKC.String(s)
}

// searchNameEnabled is true when the open database has the search_name column
// and its FTS index is built from it. Search terms are only normalized then;
// against an older database (opened read-only by ev before the daemon has
// migrated it) the raw filename is searched as before.
var searchNameEnabled bool

func hasSearchNameColumn(db interface {
	Query(string, ...any) (*sql.Rows, error)
}) bool {
	rows, err := db.Query("SELECT search_name FROM files LIMIT 0")
	if err != nil {
		return false
	}
	rows.Close()
	return true
}

func ftsUsesSearchName(db *sql.DB) bool {
	var ddl string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'files_fts'").Scan(&ddl)
	return err == nil && strings.Contains(ddl, "search_name")
}

// createSearchIndex creates the FTS index over search_name, the triggers
// that keep it in sync, and the B-tree index used for short (prefix) terms.
// The prefix search scans that index in filename order and filters on
// search_name, so the index carries both to stay covering; it replaces the
// older idx_filename, whose queries it also covers. The update trigger only
// re-tokenizes a row whose search_name changed, so a directory rename
// re-indexes the directory itself but not its children, whose search_name
// the rename assigns unchanged.
func createSearchIndex(execer interface {
	Exec(string, ...any) (sql.Result, error)
}) error {
	stmts := []string{
		"DROP INDEX IF EXISTS idx_filename",
		"CREATE INDEX IF NOT EXISTS idx_filename_search ON files(filename COLLATE BINARY, search_name, fullpath, object_type)",
		"CREATE VIRTUAL TABLE files_fts USING fts5(search_name, content='files', content_rowid='rowid', tokenize='trigram')",
		`CREATE TRIGGER files_ai AFTER INSERT ON files BEGIN
			INSERT INTO files_fts(rowid, search_name) VALUES (new.rowid, new.search_name);
		END`,
		`CREATE TRIGGER files_ad AFTER DELETE ON files BEGIN
			INSERT INTO files_fts(files_fts, rowid, search_name) VALUES('delete', old.rowid, old.search_name);
		END`,
		`CREATE TRIGGER files_au AFTER UPDATE OF search_name ON files
		WHEN old.search_name IS NOT new.search_name BEGIN
			INSERT INTO files_fts(files_fts, rowid, search_name) VALUES('delete', old.rowid, old.search_name);
			INSERT INTO files_fts(rowid, search_name) VALUES (new.rowid, new.search_name);
		END`,
	}
	for _, s := range stmts {
		if _, err := execer.Exec(s); err != nil {
			return err
		}
	}
	return createStaleTracking(execer)
}

// staleTrackingStmts record in search_name_stale the rowid of every row that
// is written with a search_name that may not match its filename. A binary
// from before search_name writes two such rows: an INSERT that leaves
// search_name empty, and a directory rename that changes
// filename but not search_name. Older binaries never drop triggers on an
// existing database, so these keep firing after a rollback, and the next
// start repairs only the recorded rows instead of scanning the whole table.
// This binary always sets search_name with filename, so it records nothing,
// except a rename between names NFKC folds together, which the repair checks
// and leaves alone.
var staleTrackingStmts = []string{
	"CREATE TABLE IF NOT EXISTS search_name_stale(id INTEGER PRIMARY KEY)",
	`CREATE TRIGGER IF NOT EXISTS search_name_stale_ai AFTER INSERT ON files
	WHEN new.search_name = '' BEGIN
		INSERT OR IGNORE INTO search_name_stale(id) VALUES (new.rowid);
	END`,
	`CREATE TRIGGER IF NOT EXISTS search_name_stale_au AFTER UPDATE OF filename ON files
	WHEN old.filename IS NOT new.filename AND old.search_name IS new.search_name BEGIN
		INSERT OR IGNORE INTO search_name_stale(id) VALUES (new.rowid);
	END`,
}

func createStaleTracking(execer interface {
	Exec(string, ...any) (sql.Result, error)
}) error {
	for _, s := range staleTrackingStmts {
		if _, err := execer.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func hasStaleTracking(db *sql.DB) bool {
	var n int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name IN ('search_name_stale_ai', 'search_name_stale_au')").Scan(&n)
	return err == nil && n == 2
}

// migrateSearchName brings a database to the search_name schema: it adds the
// column, fills it for every row, and rebuilds the FTS index over it. It runs
// in one transaction, so an interrupted migration leaves the old schema. On a
// database that is already migrated it repairs rows an older binary wrote
// after a rollback: rows it inserted have an empty search_name, and rows it
// renamed keep the search_name of their old filename. The repair reads only
// the rows the search_name_stale triggers recorded; a database migrated before
// those triggers existed gets one full scan (see staleSearchNameWhere) while
// they are installed.
func migrateSearchName(db *sql.DB) {
	if ftsUsesSearchName(db) {
		if err := repairSearchName(db); err != nil {
			log.Fatal(err)
		}
		return
	}

	log.Println("Migrating database: adding normalized search names (one-time)")
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()

	if !hasSearchNameColumn(tx) {
		if _, err = tx.Exec("ALTER TABLE files ADD COLUMN search_name TEXT NOT NULL DEFAULT ''"); err != nil {
			log.Fatal(err)
		}
	}
	for _, s := range []string{
		"DROP TRIGGER IF EXISTS files_ai",
		"DROP TRIGGER IF EXISTS files_ad",
		"DROP TRIGGER IF EXISTS files_au",
		"DROP TABLE IF EXISTS files_fts",
	} {
		if _, err = tx.Exec(s); err != nil {
			log.Fatal(err)
		}
	}
	// NFKC leaves ASCII unchanged, so copy filename for every row in SQL and
	// normalize only the names with a character outside printable ASCII in
	// Go. That keeps the migration from loading every row into memory.
	if _, err = tx.Exec("UPDATE files SET search_name = filename"); err != nil {
		log.Fatal(err)
	}
	n, err := fillSearchName(tx, "WHERE filename GLOB '*[^ -~]*'")
	if err != nil {
		log.Fatal(err)
	}
	if err = createSearchIndex(tx); err != nil {
		log.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO files_fts(files_fts) VALUES('rebuild')"); err != nil {
		log.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		log.Fatal(err)
	}
	// The migration rewrote every row, so the WAL has grown to about the
	// size of the table. The daemon keeps the database open, and a
	// checkpoint only rewinds the WAL, so truncate it now or the space
	// stays allocated until the daemon exits.
	if _, err = db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		log.Printf("Warning: could not truncate WAL after migration: %v", err)
	}
	log.Printf("Migration done: %d names normalized", n)
}

// staleSearchNameWhere selects the rows whose search_name may not match their
// filename. A correct search_name is NFKC(filename) and NFKC is idempotent, so
// a non-empty search_name equal to filename is always correct; any other row
// is a candidate that fillSearchName checks in Go. The filter reads only
// columns of idx_filename_search, so it is one covering index scan, and the
// candidates are the few names NFKC changes plus the stale rows.
const staleSearchNameWhere = "WHERE search_name = '' OR search_name <> filename"

// staleTrackedWhere selects the rows the search_name_stale triggers recorded.
const staleTrackedWhere = "WHERE rowid IN (SELECT id FROM search_name_stale)"

// repairSearchName fixes the rows an older binary wrote. With the tracking
// triggers in place it checks only the recorded rows and clears the record.
// Without them (a database migrated by an earlier 1.1 build) it scans every
// candidate row once and installs the triggers in the same transaction, so no
// write can fall between the scan and the tracking.
func repairSearchName(db *sql.DB) error {
	tracked := hasStaleTracking(db)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	where := staleTrackedWhere
	if !tracked {
		log.Println("Checking normalized search names (one-time)")
		if err = createStaleTracking(tx); err != nil {
			return err
		}
		where = staleSearchNameWhere
	}
	n, err := fillSearchName(tx, where)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM search_name_stale"); err != nil {
		return err
	}
	if n > 0 {
		log.Printf("Filled normalized search names for %d rows", n)
	}
	return tx.Commit()
}

// fillSearchName sets search_name from filename for the rows selected by
// where (all rows when empty) whose search_name differs from it, and returns
// how many it wrote. Rows that are already correct are not rewritten.
func fillSearchName(tx *sql.Tx, where string) (int, error) {
	rows, err := tx.Query("SELECT rowid, filename, search_name FROM files " + where)
	if err != nil {
		return 0, err
	}
	type row struct {
		id         int64
		searchName string
	}
	var todo []row
	for rows.Next() {
		var id int64
		var name, cur string
		if err := rows.Scan(&id, &name, &cur); err != nil {
			rows.Close()
			return 0, err
		}
		if want := SearchName(name); want != cur {
			todo = append(todo, row{id, want})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	upd, err := tx.Prepare("UPDATE files SET search_name = ? WHERE rowid = ?")
	if err != nil {
		return 0, err
	}
	defer upd.Close()
	for _, r := range todo {
		if _, err := upd.Exec(r.searchName, r.id); err != nil {
			return 0, err
		}
	}
	return len(todo), nil
}
