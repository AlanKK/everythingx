package main

import (
	"path/filepath"
	"testing"

	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/fsnotify/fsevents"

	_ "github.com/mattn/go-sqlite3"
)

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
