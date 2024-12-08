package main

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
)

func TestCreateDBAndTable(t *testing.T) {
	testDBPath := "test.db"

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
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

	// Check if the index exists
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name='idx_filename'")
	var indexName string
	err = row.Scan(&indexName)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if indexName != "idx_filename" {
		t.Fatalf("Expected index name to be 'idx_filename', got %s", indexName)
	}
}

func createAndOpenNewTestDatabase(t *testing.T, testDBPath string) (*sql.DB, error) {
	os.Remove(testDBPath)

	err := createDBAndTable(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if _, err := os.Stat(testDBPath); os.IsNotExist(err) {
		t.Fatalf("Expected database file to be created, but it does not exist")
	}

	db, err := initializeDB(testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	return db, err
}

func TestGetRecord(t *testing.T) {
	testDBPath := "test.db"

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
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
	results, err := prefixSearch("testfile", numResults)
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

func TestGetCaseSensitiveRecord(t *testing.T) {
	testDBPath := "test.db"

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
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
	_, err = db.Exec("INSERT INTO files (filename, fullpath) VALUES (?, ?)", "Testfile01.txt", "/path/to/Testfile01.txt")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test prefixSearch function
	results, err := prefixSearch("Test", 5)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
}

func TestDeleteRecord(t *testing.T) {
	testDBPath := "test.db"

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
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
	err = deleteRecord(db, "testfile1.txt")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify the record was deleted
	row := db.QueryRow("SELECT COUNT(*) FROM files WHERE filename = ?", "testfile1.txt")
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

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert a record
	err = insertRecord(db, "testfile1.txt", "/path/to/testfile1.txt")
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

	db, err := createAndOpenNewTestDatabase(t, testDBPath)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer db.Close()
	defer os.Remove(testDBPath)

	// Insert less than 100 records to test intermediate state
	for i := 1; i <= 50; i++ {
		err = bulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
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
		err = bulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	}

	err = commitRecords(db)
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
		err = bulkInsertRecords(db, fmt.Sprintf("testfile%02d.txt", i), fmt.Sprintf("/path/to/testfile%02d.txt", i))
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
	err = commitRecords(db)
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
