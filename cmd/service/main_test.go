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
				Path:  "testfile.txt",
				Flags: fsevents.ItemCreated | fsevents.ItemIsFile,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testfile.txt",
				Path:        filepath.Join("/", "testfile.txt"),
				ObjectType:  models.ItemIsFile,
				EventID:     1,
				EventAction: models.ItemCreated,
			},
		},
		{
			name: "File removed",
			fsevent: fsevents.Event{
				ID:    2,
				Path:  "testfile.txt",
				Flags: fsevents.ItemRemoved | fsevents.ItemIsFile,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testfile.txt",
				Path:        filepath.Join("/", "testfile.txt"),
				ObjectType:  models.ItemIsFile,
				EventID:     2,
				EventAction: models.ItemDeleted,
			},
		},
		{
			name: "Directory created",
			fsevent: fsevents.Event{
				ID:    3,
				Path:  "testdir",
				Flags: fsevents.ItemCreated | fsevents.ItemIsDir,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testdir",
				Path:        filepath.Join("/", "testdir"),
				ObjectType:  models.ItemIsDir,
				EventID:     3,
				EventAction: models.ItemCreated,
			},
		},
		{
			name: "Symlink created",
			fsevent: fsevents.Event{
				ID:    4,
				Path:  "testlink",
				Flags: fsevents.ItemCreated | fsevents.ItemIsSymlink,
			},
			wantRecord: &models.EventRecord{
				Filename:    "testlink",
				Path:        filepath.Join("/", "testlink"),
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
