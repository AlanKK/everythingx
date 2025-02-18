package main

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"os"
	"testing"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/shared"
	_ "github.com/mattn/go-sqlite3"
)

func TestUngzipFile(t *testing.T) {
	// Create a temporary gzip file
	sourceFile, err := ioutil.TempFile("", "source-*.gz")
	if err != nil {
		t.Fatalf("Failed to create temporary source file: %v", err)
	}
	defer os.Remove(sourceFile.Name())

	// Write compressed data to the source file
	gzipWriter := gzip.NewWriter(sourceFile)
	_, err = gzipWriter.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to write compressed data: %v", err)
	}
	gzipWriter.Close()
	sourceFile.Close()

	// Create a temporary target file
	targetFile, err := ioutil.TempFile("", "target-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temporary target file: %v", err)
	}
	defer os.Remove(targetFile.Name())
	targetFile.Close()

	// Call ungzipFile function
	err = ungzipFile(sourceFile.Name(), targetFile.Name())
	if err != nil {
		t.Fatalf("ungzipFile failed: %v", err)
	}

	// Read the target file
	targetData, err := ioutil.ReadFile(targetFile.Name())
	if err != nil {
		t.Fatalf("Failed to read target file: %v", err)
	}

	// Verify the content of the target file
	expectedData := []byte("test data")
	if !bytes.Equal(targetData, expectedData) {
		t.Errorf("Unexpected content in target file. Got %s, want %s", targetData, expectedData)
	}
}

func TestCopyData(t *testing.T) {
	// Create a temporary source database
	sourceDB, err := ffdb.CreateDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create source database: %v", err)
	}
	defer sourceDB.Close()

	// Create a temporary target database
	targetDB, err := ffdb.CreateDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create target database: %v", err)
	}
	defer targetDB.Close()

	// Insert test data into the source database
	_, err = sourceDB.Exec("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES (?, ?, ?, ?)", "testfile", "/path/to/testfile", 1, shared.ItemIsFile)
	if err != nil {
		t.Fatalf("Failed to insert test data into source database: %v", err)
	}

	// Call copyData function
	copyData(sourceDB, targetDB)

	// Verify the data in the target database
	row := targetDB.QueryRow("SELECT filename, fullpath, event_id, object_type FROM files")
	var filename, fullpath string
	var eventID int
	var objectType shared.ObjectType

	err = row.Scan(&filename, &fullpath, &eventID, &objectType)
	if err != nil {
		t.Fatalf("Failed to query target database: %v", err)
	}

	if filename != "testfile" || fullpath != "/path/to/testfile" || eventID != 1 || objectType != shared.ItemIsFile {
		t.Errorf("Unexpected data in target database. Got (%s, %s, %d, %d), want (testfile, /path/to/testfile, 1, %d)", filename, fullpath, eventID, objectType, shared.ItemIsFile)
	}
}
