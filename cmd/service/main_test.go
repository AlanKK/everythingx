package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsevents"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing
var originalFileExists func(string) bool
var fullPathLikeQuery func(string) (*[]string, error)
var originalDbChannel chan *shared.EventRecord
var mockFileExists func(string) bool

func init() {
	// Store the real implementation
	mockFileExists = shared.FileExists
}

func setupTest() {
	// Save original functions
	originalFileExists = mockFileExists
	fullPathLikeQuery = ffdb.FullPathLikeQuery
	originalDbChannel = dbChannel
}

func teardownTest() {
	// Restore original functions
	mockFileExists = originalFileExists
	// Can't restore ffdb.FullPathLikeQuery directly
	dbChannel = originalDbChannel
}

func TestDeleteMissing(t *testing.T) {
	setupTest()
	defer teardownTest()

	testCases := []struct {
		name           string
		root           string
		dbPaths        []string
		existingFiles  map[string]bool
		expectedEvents int
	}{
		{
			name:           "All files exist",
			root:           "/test/path",
			dbPaths:        []string{"/test/path/file1.txt", "/test/path/file2.txt", "/test/path/subdir/file3.txt"},
			existingFiles:  map[string]bool{"/test/path/file1.txt": true, "/test/path/file2.txt": true, "/test/path/subdir/file3.txt": true},
			expectedEvents: 0,
		},
		{
			name:           "Some files missing",
			root:           "/test/path",
			dbPaths:        []string{"/test/path/file1.txt", "/test/path/file2.txt", "/test/path/subdir/file3.txt"},
			existingFiles:  map[string]bool{"/test/path/file1.txt": true, "/test/path/file2.txt": false, "/test/path/subdir/file3.txt": false},
			expectedEvents: 2,
		},
		{
			name:           "All files missing",
			root:           "/test/path",
			dbPaths:        []string{"/test/path/file1.txt", "/test/path/file2.txt", "/test/path/subdir/file3.txt"},
			existingFiles:  map[string]bool{},
			expectedEvents: 3,
		},
		{
			name:           "Empty path list",
			root:           "/test/path",
			dbPaths:        []string{},
			existingFiles:  map[string]bool{},
			expectedEvents: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// Mock FileExists
			mockFileExists = func(path string) bool {
				exists, found := tc.existingFiles[path]
				return found && exists
			}

			testChannel := make(chan *shared.EventRecord, len(tc.dbPaths))
			dbChannel = testChannel

			// Mock FullPathLikeQuery
			fullPathLikeQuery = func(root string) (*[]string, error) {
				assert.Equal(t, tc.root, root, "root path should match")
				return &tc.dbPaths, nil
			}

			// Execute function in a goroutine
			done := make(chan struct{})
			go func() {
				deleteMissing(tc.root)
				close(done)
			}()

			// Collect events from channel
			var receivedEvents []*shared.EventRecord
			timeout := time.After(1 * time.Second)

		collectLoop:
			for i := 0; i < tc.expectedEvents; i++ {
				select {
				case event := <-testChannel:
					receivedEvents = append(receivedEvents, event)
				case <-timeout:
					break collectLoop
				}
			}

			<-done

			// Verify results
			assert.Equal(t, tc.expectedEvents, len(receivedEvents), "should have received expected number of events")

			// Check individual events
			for _, event := range receivedEvents {
				assert.Equal(t, "", event.Filename, "filename should be empty")
				assert.Contains(t, tc.dbPaths, event.Path, "path should be in original list")
				assert.Equal(t, shared.ObjectType(0), event.ObjectType, "object type should be 0")
				assert.Equal(t, int64(0), event.EventID, "event ID should be 0")
				assert.Equal(t, int64(0), event.EventTime, "event time should be 0")

				// Verify this was actually a missing file
				exists, found := tc.existingFiles[event.Path]
				assert.False(t, found && exists, "event should only be generated for missing files")
			}
		})
	}
}

func TestBuildEventRecord(t *testing.T) {
	tests := []struct {
		name       string
		fsevent    fsevents.Event
		wantRecord *shared.EventRecord
	}{
		{
			name: "File created",
			fsevent: fsevents.Event{
				ID:    1,
				Path:  "/testfile.txt",
				Flags: fsevents.ItemCreated | fsevents.ItemIsFile,
			},
			wantRecord: &shared.EventRecord{
				Filename:   "testfile.txt",
				Path:       "/testfile.txt",
				ObjectType: shared.ItemIsFile,
				EventID:    1,
			},
		},
		{
			name: "File removed",
			fsevent: fsevents.Event{
				ID:    2,
				Path:  "/tmp/testfile.txt",
				Flags: fsevents.ItemRemoved | fsevents.ItemIsFile,
			},
			wantRecord: &shared.EventRecord{
				Filename:   "testfile.txt",
				Path:       "/tmp/testfile.txt",
				ObjectType: shared.ItemIsFile,
				EventID:    2,
			},
		},
		{
			name: "Directory created",
			fsevent: fsevents.Event{
				ID:    3,
				Path:  "/Users/testdir",
				Flags: fsevents.ItemCreated | fsevents.ItemIsDir,
			},
			wantRecord: &shared.EventRecord{
				Filename:   "testdir",
				Path:       filepath.Join("/Users", "testdir"),
				ObjectType: shared.ItemIsDir,
				EventID:    3,
			},
		},
		{
			name: "Symlink created",
			fsevent: fsevents.Event{
				ID:    4,
				Path:  "/path/to/testlink",
				Flags: fsevents.ItemCreated | fsevents.ItemIsSymlink,
			},
			wantRecord: &shared.EventRecord{
				Filename:   "testlink",
				Path:       "/path/to/testlink",
				ObjectType: shared.ItemIsSymlink,
				EventID:    4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRecord := buildEventRecord(&tt.fsevent)
			// Set EventTime to match since it is generated dynamically
			tt.wantRecord.EventTime = gotRecord.EventTime

			if gotRecord.Filename != tt.wantRecord.Filename ||
				gotRecord.Path != tt.wantRecord.Path ||
				gotRecord.ObjectType != tt.wantRecord.ObjectType ||
				gotRecord.EventID != tt.wantRecord.EventID {
				t.Errorf("buildEventRecord() = %v, want %v", gotRecord, tt.wantRecord)
			}
		})
	}
}
