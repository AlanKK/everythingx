package main

import (
	"testing"
	"time"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing
var originalDbChannel chan *shared.EventRecord

func setupTest() {
	originalDbChannel = dbChannel
}

func teardownTest() {
	fileExists = shared.FileExists
	fullPathLikeQuery = ffdb.FullPathLikeQuery
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
			fileExists = func(path string) bool {
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
				assert.Equal(t, uint64(0), event.EventID, "event ID should be 0")
				assert.Equal(t, int64(0), event.EventTime, "event time should be 0")

				// Verify this was actually a missing file
				exists, found := tc.existingFiles[event.Path]
				assert.False(t, found && exists, "event should only be generated for missing files")
			}
		})
	}
}
