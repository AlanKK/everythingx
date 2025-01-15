package main

import (
	"path/filepath"
	"testing"

	"github.com/AlanKK/findfiles/internal/models"
	"github.com/fsnotify/fsevents"
)

func TestBuildEventRecord(t *testing.T) {
	tests := []struct {
		name       string
		fsevent    fsevents.Event
		wantRecord *models.EventRecord
	}{
		{
			name: "File created",
			fsevent: fsevents.Event{
				ID:    1,
				Path:  "/testfile.txt",
				Flags: fsevents.ItemCreated | fsevents.ItemIsFile,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testfile.txt",
				Path:        "/testfile.txt",
				ObjectType:  models.ItemIsFile,
				EventID:     1,
				EventAction: models.ItemCreated,
			},
		},
		{
			name: "File removed",
			fsevent: fsevents.Event{
				ID:    2,
				Path:  "/tmp/testfile.txt",
				Flags: fsevents.ItemRemoved | fsevents.ItemIsFile,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testfile.txt",
				Path:        "/tmp/testfile.txt",
				ObjectType:  models.ItemIsFile,
				EventID:     2,
				EventAction: models.ItemDeleted,
			},
		},
		{
			name: "Directory created",
			fsevent: fsevents.Event{
				ID:    3,
				Path:  "/Users/testdir",
				Flags: fsevents.ItemCreated | fsevents.ItemIsDir,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testdir",
				Path:        filepath.Join("/Users", "testdir"),
				ObjectType:  models.ItemIsDir,
				EventID:     3,
				EventAction: models.ItemCreated,
			},
		},
		{
			name: "Symlink created",
			fsevent: fsevents.Event{
				ID:    4,
				Path:  "/path/to/testlink",
				Flags: fsevents.ItemCreated | fsevents.ItemIsSymlink,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testlink",
				Path:        "/path/to/testlink",
				ObjectType:  models.ItemIsSymlink,
				EventID:     4,
				EventAction: models.ItemCreated,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRecord := buildEventRecord(tt.fsevent)
			// Set EventTime to match since it is generated dynamically
			tt.wantRecord.EventTime = gotRecord.EventTime

			if gotRecord.Filename != tt.wantRecord.Filename ||
				gotRecord.Path != tt.wantRecord.Path ||
				gotRecord.ObjectType != tt.wantRecord.ObjectType ||
				gotRecord.EventID != tt.wantRecord.EventID ||
				gotRecord.EventAction != tt.wantRecord.EventAction {
				t.Errorf("buildEventRecord() = %v, want %v", gotRecord, tt.wantRecord)
			}
		})
	}
}

// Need to start the db thread to test this
// func TestDeleteMissing(t *testing.T) {
// 	// Create a temporary database
// 	db, err := sql.Open("sqlite3", ":memory:")
// 	if err != nil {
// 		t.Fatalf("Failed to create database: %v", err)
// 	}
// 	defer db.Close()

// 	// Create the files table
// 	_, err = db.Exec("CREATE TABLE files (filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER)")
// 	if err != nil {
// 		t.Fatalf("Failed to create table: %v", err)
// 	}

// 	// Insert test data
// 	_, err = db.Exec("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES ('file1', '/tmp/file1', 1, ?)", models.ItemIsFile)
// 	if err != nil {
// 		t.Fatalf("Failed to insert test data: %v", err)
// 	}
// 	_, err = db.Exec("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES ('file2', '/tmp/file2', 2, ?)", models.ItemIsFile)
// 	if err != nil {
// 		t.Fatalf("Failed to insert test data: %v", err)
// 	}

// 	// Create a temporary file to simulate an existing file
// 	tempFile, err := os.Create("/tmp/file1")
// 	if err != nil {
// 		t.Fatalf("Failed to create temporary file: %v", err)
// 	}
// 	tempFile.Close()
// 	defer os.Remove("/tmp/file1")

// 	// Run the deleteMissing function
// 	deleteMissing(db)

// 	// sleep for 5 seconds and drop another event in the queue to flush it
// 	time.Sleep(5 * time.Second)
// 	_, err = db.Exec("INSERT INTO files (filename, fullpath, event_id, object_type) VALUES ('file1', '/tmp/file99', 1, ?)", models.ItemIsFile)
// 	if err != nil {
// 		t.Fatalf("Failed to insert test data: %v", err)
// 	}

// 	// Check the results
// 	rows, err := db.Query("SELECT fullpath FROM files")
// 	if err != nil {
// 		t.Fatalf("Failed to query database: %v", err)
// 	}
// 	defer rows.Close()

// 	var fullpath string
// 	var foundFile1, foundFile2 bool
// 	for rows.Next() {
// 		if err := rows.Scan(&fullpath); err != nil {
// 			t.Fatalf("Failed to scan row: %v", err)
// 		}
// 		if fullpath == "/tmp/file1" {
// 			foundFile1 = true
// 		}
// 		if fullpath == "/tmp/file2" {
// 			foundFile2 = true
// 		}
// 	}

// 	if !foundFile1 {
// 		t.Fatalf("Expected file1 to be found, but it was not")
// 	}
// 	if foundFile2 {
// 		t.Fatalf("Expected file2 to be deleted, but it still exists")
// 	}
// }
