package ffdb

import (
	"database/sql"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/mattn/go-sqlite3"
)

// TODO:  check out driver benchmarks for sqlite3 at https://github.com/cvilsmeier/go-sqlite-bench

var records []struct {
	filename string
	path     string
}

var prefixSearchStmt *sql.Stmt
var trigramSearchStmt *sql.Stmt
var globFTSStmt *sql.Stmt
var globScanStmt *sql.Stmt
var insertStmt *sql.Stmt
var deleteStmt *sql.Stmt
var deleteDirStmt *sql.Stmt
var renameDirStmt *sql.Stmt
var fullPathLikeStmt *sql.Stmt

// Creates and opens a new database at the given pathname.
func CreateDB(pathname string) (*sql.DB, error) {
	if shared.FileExists(pathname) {
		return nil, os.ErrExist
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		return nil, err
	}

	createTablesAndIndexes(db)
	configureDB(db)
	prepareStatements(db)

	return db, err
}

// Opens an existing database
func OpenDB(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if !shared.FileExists(pathname) {
		return nil, error(os.ErrNotExist)
	}

	db, err := sql.Open("sqlite3", pathname)
	if err != nil {
		return nil, err
	}

	configureDB(db)
	migrateSearchName(db)
	prepareStatements(db)

	return db, err
}

// Open read-only for the UI.  Can't use PRAGMAS in read-only mode.
func OpenDBReadOnly(pathname string) (*sql.DB, error) {
	// Check if the database file exists
	if !shared.FileExists(pathname) {
		return nil, error(os.ErrNotExist)
	}

	db, err := sql.Open("sqlite3", "file:"+pathname+"?mode=ro")
	if err != nil {
		return nil, err
	}

	prepareReadStatements(db)

	return db, err
}

// Creates the necessary tables and indexes in the database.
func createTablesAndIndexes(db *sql.DB) {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS files(filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER, search_name TEXT NOT NULL DEFAULT '')")
	if err != nil {
		log.Fatal(err)
	}

	if err = createSearchIndex(db); err != nil {
		log.Fatal(err)
	}
}

// Configures the database with necessary settings.
func configureDB(db *sql.DB) {

	// For the UI to use case sensitive search.
	// _, err := db.Exec("PRAGMA case_sensitive_like = ON")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// Enable WAL mode - multiple readers and writer
	_, err := db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Fatal(err)
	}

	// No performane impact for this
	_, err = db.Exec("PRAGMA synchronous=NORMAL;")
	if err != nil {
		log.Fatal(err)
	}
}

// Prepares the SQL statements for later use.
func prepareStatements(db *sql.DB) {
	prepareReadStatements(db)
	prepareWriteStatements(db)
}

// readMu guards the read statements and searchNameEnabled, which are
// re-prepared when the schema changes under an open reader.
var readMu sync.RWMutex

// readDB and readSchemaVersion record the database and schema the read
// statements were prepared for.
var readDB *sql.DB
var readSchemaVersion int64

// prepareReadStatements prepares only the queries that search clients use.
// A read-only client (ev, the GUI) can then open a database the daemon has
// not migrated yet, whose schema lacks columns the write statements use.
func prepareReadStatements(db *sql.DB) {
	readMu.Lock()
	defer readMu.Unlock()
	if err := prepareReadStatementsLocked(db); err != nil {
		log.Fatal(err)
	}
}

func schemaVersion(db *sql.DB) (int64, error) {
	var v int64
	err := db.QueryRow("PRAGMA schema_version").Scan(&v)
	return v, err
}

func prepareReadStatementsLocked(db *sql.DB) error {
	version, err := schemaVersion(db)
	if err != nil {
		return err
	}

	// Search the normalized names when the database has them (see
	// SearchName); otherwise fall back to the raw filename.
	enabled := hasSearchNameColumn(db) && ftsUsesSearchName(db)
	nameCol := "filename"
	if enabled {
		nameCol = "search_name"
	}

	prefix, err := db.Prepare(prefixSearchSQL(nameCol))
	if err != nil {
		return err
	}

	trigram, err := db.Prepare("SELECT f.fullpath, f.object_type FROM files_fts fts JOIN files f ON f.rowid = fts.rowid WHERE files_fts MATCH ? AND " + hiddenFilterSQL("f.fullpath") + " ORDER BY f.filename ASC LIMIT ?")
	if err != nil {
		// FTS table may not exist in older databases opened read-only
		log.Printf("Warning: could not prepare trigram search statement: %v", err)
		trigram = nil
	}

	var globFTS *sql.Stmt
	if trigram != nil {
		globFTS, err = db.Prepare(globFTSSQL(nameCol))
		if err != nil {
			log.Printf("Warning: could not prepare FTS glob search statement: %v", err)
			globFTS = nil
		}
	}

	closeAll := func(stmts ...*sql.Stmt) {
		for _, st := range stmts {
			if st != nil {
				st.Close()
			}
		}
	}

	globScan, err := db.Prepare(globScanSQL(nameCol))
	if err != nil {
		closeAll(prefix, trigram, globFTS)
		return err
	}

	fullPathLike, err := db.Prepare("SELECT fullpath FROM files where fullpath like ?")
	if err != nil {
		closeAll(prefix, trigram, globFTS, globScan)
		return err
	}

	closeAll(prefixSearchStmt, trigramSearchStmt, globFTSStmt, globScanStmt, fullPathLikeStmt)
	prefixSearchStmt, trigramSearchStmt, fullPathLikeStmt = prefix, trigram, fullPathLike
	globFTSStmt, globScanStmt = globFTS, globScan
	searchNameEnabled = enabled
	readDB, readSchemaVersion = db, version
	return nil
}

// prefixSearchSQL is the short-term search over nameCol (search_name, or
// filename on an unmigrated database). It scans idx_filename_search in
// filename order; the term is LIKE-escaped (see likeEscaper).
func prefixSearchSQL(nameCol string) string {
	return "SELECT fullpath, object_type FROM files WHERE " + nameCol + ` LIKE ? ESCAPE '\' AND ` + hiddenFilterSQL("fullpath") + " ORDER BY filename ASC LIMIT ?"
}

// hiddenFilterSQL is a condition that takes one boolean argument: true keeps
// every row, false drops hidden ones (see IsHiddenPath). Filtering in SQL
// rather than afterwards keeps LIMIT counting only the rows that are shown.
func hiddenFilterSQL(pathCol string) string {
	return "(? OR " + pathCol + " NOT LIKE '%/.%')"
}

// IsHiddenPath reports whether path is hidden: its name, or the name of any
// folder above it, starts with a dot. It matches hiddenFilterSQL.
func IsHiddenPath(path string) bool {
	return strings.Contains(path, "/.")
}

// SearchOptions changes how Search filters its results. The zero value
// matches PrefixSearch.
type SearchOptions struct {
	// HideHidden drops hidden files and folders, and everything inside a
	// hidden folder (see IsHiddenPath).
	HideHidden bool
}

// globFTSSQL selects the candidates for a glob whose LIKE pattern has a run
// of 3 or more literal characters, which the FTS5 trigram index can look up.
// It has no LIMIT: the rows are filtered with the glob's regexp afterwards.
func globFTSSQL(nameCol string) string {
	return "SELECT f.fullpath, f.object_type, f." + nameCol + " FROM files_fts fts JOIN files f ON f.rowid = fts.rowid WHERE fts." + nameCol + " LIKE ? ORDER BY f.filename ASC"
}

// globScanSQL selects the candidates for any other glob. It is a covering
// scan of idx_filename_search in filename order, so a search that stops after
// its limit reads only as far as it needs to.
//
// The plain-table LIKE folds only ASCII case, while the glob's regexp folds
// Unicode case, so a non-ASCII name whose case differs from the pattern can
// be missed here. Short literal searches (prefixSearchSQL) behave the same.
func globScanSQL(nameCol string) string {
	return "SELECT fullpath, object_type, " + nameCol + " FROM files WHERE " + nameCol + " LIKE ? ORDER BY filename ASC"
}

// refreshReadStatements re-prepares the read statements when the schema has
// changed since they were prepared. A reader that opened the database before
// the daemon migrated it would otherwise keep searching with raw terms:
// SQLite silently re-prepares the old statements against the new FTS index,
// which holds normalized names.
func refreshReadStatements() {
	readMu.RLock()
	db, prepared := readDB, readSchemaVersion
	readMu.RUnlock()
	if db == nil {
		return
	}
	version, err := schemaVersion(db)
	if err != nil || version == prepared {
		return
	}
	readMu.Lock()
	defer readMu.Unlock()
	if readDB != db || readSchemaVersion != prepared {
		return // another search already refreshed
	}
	if err := prepareReadStatementsLocked(db); err != nil {
		log.Printf("Warning: could not re-prepare search statements: %v", err)
	}
}

// likeEscaper escapes LIKE wildcards so a term is matched literally. NFKC
// turns fullwidth "％" and "＿" into "%" and "_", so this runs after
// normalization.
var likeEscaper = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)

func prepareWriteStatements(db *sql.DB) {
	var err error

	insertStmt, err = db.Prepare("INSERT INTO files (filename, fullpath, event_id, object_type, search_name) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatal(err)
	}

	deleteStmt, err = db.Prepare("DELETE FROM files WHERE fullpath = ?")
	if err != nil {
		log.Fatal(err)
	}

	deleteDirStmt, err = db.Prepare("DELETE FROM files WHERE fullpath = ? OR (fullpath > ? AND fullpath < ?)")
	if err != nil {
		log.Fatal(err)
	}

	renameDirStmt, err = db.Prepare("UPDATE files SET fullpath = ? || SUBSTR(fullpath, LENGTH(?) + 1), filename = CASE WHEN fullpath = ? THEN ? ELSE filename END, search_name = CASE WHEN fullpath = ? THEN ? ELSE search_name END WHERE fullpath = ? OR (fullpath > ? AND fullpath < ?)")
	if err != nil {
		log.Fatal(err)
	}

}

// globSearch returns up to limit names that match g, in filename order (all
// of them when limit is negative). The caller holds readMu.
func globSearch(g *glob, limit int, opts SearchOptions) ([]*shared.SearchResult, error) {
	if limit == 0 {
		return nil, nil
	}
	stmt := globScanStmt
	if g.longestLiteral >= 3 && globFTSStmt != nil {
		stmt = globFTSStmt
	}
	rows, err := stmt.Query(g.like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*shared.SearchResult
	var name string
	for rows.Next() {
		result := &shared.SearchResult{}
		if err := rows.Scan(&result.Fullpath, &result.ObjectType, &name); err != nil {
			return nil, err
		}
		if !g.re.MatchString(name) || (opts.HideHidden && IsHiddenPath(result.Fullpath)) {
			continue
		}
		results = append(results, result)
		if len(results) == limit {
			break
		}
	}
	return results, rows.Err()
}

// Performs a substring search for filenames containing the given term.
// For terms of 3 or more characters, uses the FTS5 trigram index for fast
// lookup. For shorter terms, which trigram cannot match, uses a prefix
// search ordered by the filename index. A term with an unescaped "*" or "?"
// is a glob matched against the whole name instead (see parseGlob).
func PrefixSearch(prefix string, limit int) ([]*shared.SearchResult, error) {
	return Search(prefix, limit, SearchOptions{})
}

// Search is PrefixSearch with options.
func Search(prefix string, limit int, opts SearchOptions) ([]*shared.SearchResult, error) {
	var rows *sql.Rows
	var err error

	refreshReadStatements()
	readMu.RLock()
	defer readMu.RUnlock()

	if searchNameEnabled {
		prefix = SearchName(prefix)
	}
	if g, ok := parseGlob(prefix); ok {
		return globSearch(g, limit, opts)
	}
	showHidden := !opts.HideHidden
	// Trigram needs 3 characters, not bytes: a 2-character CJK term is 6 bytes.
	if utf8.RuneCountInString(prefix) >= 3 && trigramSearchStmt != nil {
		// Wrap in double quotes so FTS5 treats the term as a literal phrase,
		// preventing characters like "." from causing syntax errors.
		quoted := `"` + strings.ReplaceAll(prefix, `"`, `""`) + `"`
		rows, err = trigramSearchStmt.Query(quoted, showHidden, limit)
	} else {
		rows, err = prefixSearchStmt.Query(likeEscaper.Replace(prefix)+"%", showHidden, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*shared.SearchResult
	for rows.Next() {
		result := &shared.SearchResult{}
		if err := rows.Scan(&result.Fullpath, &result.ObjectType); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// Returns the count of records in the files table.
func RecordCount(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM files").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// Deletes a record from the database with the given fullpath.
func DeleteRecord(db *sql.DB, fullpath string) error {
	_, err := db.Exec("DELETE FROM files WHERE fullpath = ?", fullpath)
	return err
}

// Inserts a new record into the database.
func InsertRecord(db *sql.DB, filename string, path string, eventID uint64, objectType shared.ObjectType) error {
	_, err := db.Exec("INSERT OR IGNORE INTO files (filename, fullpath, event_id, object_type, search_name) VALUES (?, ?, ?, ?, ?)", filename, path, eventID, objectType, SearchName(filename))
	return err
}

func FullPathLikeQuery(path string) (*[]string, error) {
	var results []string

	rows, err := fullPathLikeStmt.Query(path + "%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var result string
		err = rows.Scan(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return &results, nil
}

// FullPathLikeQueryEach streams matching paths one at a time via the callback,
// avoiding materializing all results into a slice.
func FullPathLikeQueryEach(path string, fn func(string)) error {
	rows, err := fullPathLikeStmt.Query(path + "%")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		fn(result)
	}
	return rows.Err()
}

// Collects records and commits them to the database when enough records are collected.
func BulkInsertRecords(db *sql.DB, filename string, path string) error {

	// Collect records here
	records = append(records, struct {
		filename string
		path     string
	}{filename, path})

	// Commit records when we have enough
	if len(records) >= 1000 {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		txInsert := tx.Stmt(insertStmt)
		for _, record := range records {
			_, err = txInsert.Exec(record.filename, record.path, nil, 0, SearchName(record.filename))
			if err != nil {
				return err
			}
		}

		err = tx.Commit()
		if err != nil {
			return err
		}

		records = records[:0] // Clear the slice
	}

	return nil
}

// Commits any collected records to the database.
func CommitRecords(db *sql.DB) error {

	if len(records) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO files (filename, fullpath, search_name) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, record := range records {
		_, err = stmt.Exec(record.filename, record.path, SearchName(record.filename))
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	records = records[:0] // Clear the slice
	return nil
}

// Stores a batch of events in the database.
func BulkStoreEvents(db *sql.DB, eventRecordQueue *[]shared.EventRecord) error {
	if len(*eventRecordQueue) == 0 {
		log.Println("No events to store")
		return nil
	}
	bulkTime := time.Now()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	// Rolls back on any early return; a no-op once Commit has succeeded.
	defer tx.Rollback()

	// The package-level statements are prepared on the *sql.DB. Executing
	// them directly would take another pooled connection and autocommit each
	// row, so bind them to this transaction instead.
	txInsert := tx.Stmt(insertStmt)
	txDelete := tx.Stmt(deleteStmt)
	txDeleteDir := tx.Stmt(deleteDirStmt)
	txRenameDir := tx.Stmt(renameDirStmt)

	var num_committed, num_missing, num_duplicate, num_deleted, num_renamed int

	// Pre-process: detect directory rename pairs.
	// FSEvents/fanotify fire two events for a rename: one for the old path
	// (no longer exists) and one for the new path (now exists).
	handled := make(map[int]bool)
	for i, e := range *eventRecordQueue {
		if !e.IsRename || e.ObjectType != shared.ItemIsDir || e.FoundOnScan || shared.FileExists(e.Path) {
			continue
		}
		// Old path of a dir rename. Find the new-path partner.
		for j := i + 1; j < len(*eventRecordQueue); j++ {
			ej := (*eventRecordQueue)[j]
			if ej.IsRename && ej.ObjectType == shared.ItemIsDir && !handled[j] && shared.FileExists(ej.Path) {
				_, err = txRenameDir.Exec(ej.Path, e.Path, e.Path, ej.Filename, e.Path, SearchName(ej.Filename), e.Path, e.Path+"/", e.Path+"0")
				if err != nil {
					return err
				}
				handled[i] = true
				handled[j] = true
				num_renamed++
				break
			}
		}
	}

	for i, e := range *eventRecordQueue {
		if handled[i] {
			continue
		}
		// Existence is checked at flush time, up to 10 s after the event. A
		// removal is applied regardless, because a different object may have
		// taken the path since (mv A B; mv C A) and would otherwise keep the
		// old object's rows alive under it.
		if !e.IsDelete && (e.FoundOnScan || shared.FileExists(e.Path)) {
			_, err = txInsert.Exec(e.Filename, e.Path, e.EventID, e.ObjectType, SearchName(e.Filename))
			if err != nil {
				if isDuplicate(err) {
					num_duplicate++
				} else {
					return err
				}
			}
			num_committed++
		} else {
			if e.ObjectType == shared.ItemIsDir {
				if e.IsRename {
					// Unpaired rename: just remove the dir entry, don't cascade.
					_, err = txDelete.Exec(e.Path)
				} else {
					_, err = txDeleteDir.Exec(e.Path, e.Path+"/", e.Path+"0")
				}
			} else {
				_, err = txDelete.Exec(e.Path)
			}
			if err != nil {
				return err
			}
			num_deleted++
		}
	}

	commitTime := time.Now()

	err = tx.Commit()
	if err != nil {
		return err
	}

	commitTimeEnd := time.Since(commitTime).Round(time.Microsecond).String()
	bulkTimeEnd := time.Since(bulkTime).Round(time.Microsecond).String()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf(
		"Events: %-6d Del: %-6d New: %-6d Ren: %-6d Missing: %-6d Dups: %-6d Time fn/commit: %-9s / %-8s Queue: %p Capacity: %-7d Mem: %.1f heap %.1f MB",
		len(*eventRecordQueue),
		num_deleted,
		num_committed-num_duplicate,
		num_renamed,
		num_missing,
		num_duplicate,
		bulkTimeEnd,
		commitTimeEnd,
		eventRecordQueue,
		cap(*eventRecordQueue),
		float64(shared.BToMb(m.Sys)),
		float64(shared.BToMb(m.Alloc)),
	)

	return nil
}

// Checks if the given error is a SQLite constraint violation error.
func isDuplicate(err error) bool {
	if sqliteErr, ok := err.(sqlite3.Error); ok && sqliteErr.Code == sqlite3.ErrConstraint {
		return true
	}
	return false
}
