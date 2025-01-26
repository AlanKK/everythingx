package ffdb

import (
	"fmt"
	"os"
	"testing"

	"github.com/AlanKK/findfiles/internal/models"
)

func TestCreateDB(t *testing.T) {
	testDBPath := "test.db"

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()

	// Check if the table exists
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='files'")
	var tableName string
	err = row.Scan(&tableName)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if tableName != "files" {
		t.Fatalf("Expected table name to be 'files', got %s", tableName)
	}

	// Check if the filename index exists
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_filename'")
	var indexName string
	err = row.Scan(&indexName)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if indexName != "idx_filename" {
		t.Fatalf("Expected index name to be 'idx_filename', got %s", indexName)
	}

	// Check if the fullpath index exists
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_fullpath'")
	err = row.Scan(&indexName)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if indexName != "idx_fullpath" {
		t.Fatalf("Expected index name to be 'idx_fullpath', got %s", indexName)
	}
}

func TestGetRecord(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert test data
	for i := 1; i < 100; i++ {
		_, err = db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Test prefixSearch function
	numResults := 5
	results, err := PrefixSearch("testfile", numResults)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedResults := []string{"/path/to/testfile01.txt", "/path/to/testfile02.txt", "/path/to/testfile03.txt", "/path/to/testfile04.txt", "/path/to/testfile05.txt"}
	if len(results) != len(expectedResults) {
		t.Fatalf("Expected %d results, got %d", len(expectedResults), len(results))
	}

	for i, result := range results {
		if result != expectedResults[i] {
			t.Fatalf("Expected result %s, got %s", expectedResults[i], result)
		}
	}
}

// func TestGetCaseSensitiveRecord(t *testing.T) {
// 	testDBPath := "test.db"
// 	os.Remove(testDBPath)

// 	db, err := CreateDB(testDBPath)
// 	if err != nil {
// 		t.Fatalf("Expected no error, got %v", err)
// 	}
// 	defer db.Close()
// 	defer os.Remove(testDBPath)

// 	// Insert test data
// 	for i := 1; i < 100; i++ {
// 		_, err = db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
// 		if err != nil {
// 			t.Fatalf("Expected no error, got %v", err)
// 		}
// 	}
// 	_, err = db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", "Testfile01.txt", "/path/to/Testfile01.txt")
// 	if err != nil {
// 		t.Fatalf("Expected no error, got %v", err)
// 	}

// 	// Test prefixSearch function
// 	results, err := PrefixSearch("Test", 5)
// 	if err != nil {
// 		t.Fatalf("Expected no error, got %v", err)
// 	}

// 	if len(results) != 1 {
// 		t.Log(results)
// 		t.Fatalf("Expected 1 result, got %d", len(results))
// 	}
// }

func TestDeleteRecord(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert test data
	_, err = db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", "testfile1.txt", "/path/to/testfile1.txt")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Delete the record
	err = DeleteRecord(db, "/path/to/testfile1.txt")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the record was deleted
	row := db.QueryRow("SELECT COUNT(*) FROM files WHERE fullpath = ?", "/path/to/testfile1.txt")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}
}
func TestInsertRecord(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert a record
	err = InsertRecord(db, "testfile1.txt", "/path/to/testfile1.txt", 0, models.ItemIsFile)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the record was inserted
	row := db.QueryRow("SELECT filename, fullpath FROM files WHERE filename = ?", "testfile1.txt")
	var filename, fullpath string
	err = row.Scan(&filename, &fullpath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if filename != "testfile1.txt" || fullpath != "/path/to/testfile1.txt" {
		t.Fatalf("Expected filename 'testfile1.txt' and fullpath '/path/to/testfile1.txt', got filename '%s' and fullpath '%s'", filename, fullpath)
	}
}

func TestBulkInsertRecords(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert less than 100 records to test intermediate state
	for i := 1; i <= 50; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify records are not yet committed
	row := db.QueryRow("SELECT COUNT(*) FROM files")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}

	// Insert more records to trigger bulk insert
	for i := 51; i <= 100; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	err = CommitRecords(db)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify all records are committed
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 100 {
		t.Fatalf("Expected 100 records, got %d", count)
	}

	// Insert additional records to test subsequent bulk insert
	for i := 101; i <= 150; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify intermediate state
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 100 {
		t.Fatalf("Expected 100 records, got %d", count)
	}

	// Commit remaining records
	err = CommitRecords(db)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify all records are committed
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 150 {
		t.Fatalf("Expected 150 records, got %d", count)
	}
}

func TestFileExists(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer os.Remove(tempFile.Name())

	// Test that the file exists
	if !fileExists(tempFile.Name()) {
		t.Fatalf("Expected file to exist, but it does not")
	}

	// Test that a non-existent file does not exist
	if fileExists("nonexistentfile.txt") {
		t.Fatalf("Expected file to not exist, but it does")
	}
}

func TestBulkStoreEvents(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	events := []models.EventRecord{
		{Filename: "testfile1.txt", Path: "/tmp/testfile1.txt", EventID: 1, ObjectType: 0},
		{Filename: "testfile2.txt", Path: "/tmp/testfile2.txt", EventID: 2, ObjectType: 0},
		{Filename: "testfile3.txt", Path: "/tmp/testfile3.txt", EventID: 3, ObjectType: 0},
	}

	// Create the first file only, leaving the second to mimick FSE behavior of sending create events when no
	// file is actually created (or it is quickly deleted)
	file, err := os.Create(events[0].Path)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	file.Close()
	defer os.Remove(events[0].Path)

	// Insert the event that should be deleted
	err = InsertRecord(db, "testfile3.txt", "/tmp/testfile3.txt", 0, models.ItemIsFile)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	err = BulkStoreEvents(db, &events)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the records were inserted and deleted correctly
	row := db.QueryRow("SELECT COUNT(*) FROM files WHERE filename = ?", "testfile1.txt")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 1 {
		t.Fatalf("Expected 1 record, got %d", count)
	}

	row = db.QueryRow("SELECT COUNT(*) FROM files WHERE filename = ?", "testfile2.txt")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 1 record, got %d", count)
	}

	row = db.QueryRow("SELECT COUNT(*) FROM files WHERE filename = ?", "testfile3.txt")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}
}

func TestBulkStoreDuplicates(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	events := []models.EventRecord{
		{Filename: "testfile1.txt", Path: "/tmp/testfile1.txt", EventID: 1, ObjectType: 0},
		{Filename: "testfile2.txt", Path: "/tmp/testfile2.txt", EventID: 2, ObjectType: 0},
		{Filename: "testfile3.txt", Path: "/tmp/testfile3.txt", EventID: 3, ObjectType: 0},
	}

	// Create the files so they get inserted into the db
	for i := range events {
		f, err := os.Create(events[i].Path)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		f.Close()
		defer os.Remove(events[i].Path)
	}

	// Insert one file
	err = InsertRecord(db, events[0].Filename, events[0].Path, events[0].EventID, events[0].ObjectType)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Insert all and make sure we have three in the db
	err = BulkStoreEvents(db, &events)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	row := db.QueryRow("SELECT COUNT(*) FROM files")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 3 {
		t.Fatalf("Expected 3 records, got %d", count)
	}
}

func TestOpenDB(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	// Create a new database
	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	db.Close()
	defer os.Remove(testDBPath)

	// Test opening the existing database
	db, err = OpenDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()

	// Verify PRAGMA settings
	// case_sensitive_like pragma doesn't seem to respone to the query so it is omitted
	row := db.QueryRow("PRAGMA journal_mode")
	var journalMode string
	err = row.Scan(&journalMode)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("Expected PRAGMA journal_mode to be 'wal', got %s", journalMode)
	}

	// Verify prepared statements
	if prefixSearchStmt == nil {
		t.Fatalf("Expected prefixSearchStmt to be prepared, but it is nil")
	}
	if insertStmt == nil {
		t.Fatalf("Expected insertStmt to be prepared, but it is nil")
	}
	if deleteStmt == nil {
		t.Fatalf("Expected deleteStmt to be prepared, but it is nil")
	}
}

func TestOpenDB_FileNotExist(t *testing.T) {
	testDBPath := "nonexistent.db"

	// Test opening a non-existent database
	_, err := OpenDB(testDBPath)
	if err == nil {
		t.Fatalf("Expected an error, got nil")
	}
}
func TestBulkInsertRecords1000(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert less than 1000 records to test intermediate state
	for i := 1; i <= 500; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%03d.txt", i), fmt.Sprintf("/path/to/testfile%03d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify records are not yet committed
	row := db.QueryRow("SELECT COUNT(*) FROM files")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}

	// Insert more records to trigger bulk insert
	for i := 501; i <= 1000; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%03d.txt", i), fmt.Sprintf("/path/to/testfile%03d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify all records are committed
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 1000 {
		t.Fatalf("Expected 1000 records, got %d", count)
	}

	// Insert additional records to test subsequent bulk insert
	for i := 1001; i <= 1500; i++ {
		err = BulkInsertRecords(db, fmt.Sprintf("testfile%03d.txt", i), fmt.Sprintf("/path/to/testfile%03d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify intermediate state
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 1000 {
		t.Fatalf("Expected 1000 records, got %d", count)
	}

	// Commit remaining records
	err = CommitRecords(db)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify all records are committed
	row = db.QueryRow("SELECT COUNT(*) FROM files")
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 1500 {
		t.Fatalf("Expected 1500 records, got %d", count)
	}
}
func TestRecordCount(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Verify initial record count
	count, err := RecordCount(db)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}

	// Insert test data
	for i := 1; i <= 10; i++ {
		err = InsertRecord(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i), 0, models.ItemIsFile)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	// Verify record count after insertion
	count, err = RecordCount(db)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 10 {
		t.Fatalf("Expected 10 records, got %d", count)
	}
}

// Verify order given to BulkStoreEvents is maintainded in DB for creates/deletes
func TestBulkStoreOrdering(t *testing.T) {
	testDBPath := "test.db"
	os.Remove(testDBPath)

	db, err := CreateDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	numItems := 1000
	events := []models.EventRecord{}
	for i := 1; i <= numItems; i++ {
		events = append(events, models.EventRecord{
			Filename:   fmt.Sprintf("testfile%d.txt", i),
			Path:       fmt.Sprintf("/tmp/testfile%d.txt", i),
			EventID:    uint64(i),
			ObjectType: 0,
		})
	}

	for i := 1; i <= numItems; i++ {
		events = append(events, models.EventRecord{
			Filename:   fmt.Sprintf("testfile%d.txt", i),
			Path:       fmt.Sprintf("/tmp/testfile%d.txt", i),
			EventID:    uint64(i),
			ObjectType: 0,
		})
	}

	// Insert all and make sure we have three in the db
	err = BulkStoreEvents(db, &events)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	row := db.QueryRow("SELECT COUNT(*) FROM files")
	var count int
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}

	// Part 2, interleave the events
	events = events[:0]

	for i := 1; i <= numItems; i++ {
		events = append(events, models.EventRecord{
			Filename:   fmt.Sprintf("testfile%d.txt", i),
			Path:       fmt.Sprintf("/tmp/testfile%d.txt", i),
			EventID:    uint64(i),
			ObjectType: 0,
		})
		events = append(events, models.EventRecord{
			Filename:   fmt.Sprintf("testfile%d.txt", i),
			Path:       fmt.Sprintf("/tmp/testfile%d.txt", i),
			EventID:    uint64(i),
			ObjectType: 0,
		})
	}
	// Insert all and make sure we have three in the db
	err = BulkStoreEvents(db, &events)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	row = db.QueryRow("SELECT COUNT(*) FROM files")

	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 records, got %d", count)
	}
}
