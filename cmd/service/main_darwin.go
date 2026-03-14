//go:build darwin

package main

import (
	"database/sql"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"

	"github.com/fsnotify/fsevents"
)

// todo:
// 	Read historical events from FSEvents

var noteDescription = map[fsevents.EventFlags]string{
	fsevents.MustScanSubDirs: "MustScanSubdirs",
	fsevents.UserDropped:     "UserDropped",
	fsevents.KernelDropped:   "KernelDropped",
	fsevents.EventIDsWrapped: "EventIDsWrapped",
	fsevents.HistoryDone:     "HistoryDone",
	fsevents.RootChanged:     "RootChanged",
	fsevents.Mount:           "Mount",
	fsevents.Unmount:         "Unmount",

	fsevents.ItemCreated:       "Created",
	fsevents.ItemRemoved:       "Removed",
	fsevents.ItemInodeMetaMod:  "InodeMetaMod",
	fsevents.ItemRenamed:       "Renamed",
	fsevents.ItemModified:      "Modified",
	fsevents.ItemFinderInfoMod: "FinderInfoMod",
	fsevents.ItemChangeOwner:   "ChangeOwner",
	fsevents.ItemXattrMod:      "XAttrMod",
	fsevents.ItemIsFile:        "IsFile",
	fsevents.ItemIsDir:         "IsDir",
	fsevents.ItemIsSymlink:     "IsSymLink",
}

func shouldIgnorePath(path string) bool {
	return strings.HasPrefix(path, "/System/Volumes/Data")
}

var openSettingsOnce sync.Once

func reportPermissionError(path string) {
	log.Printf("Permission denied scanning %s. Grant Full Disk Access to everythingxd: System Settings → Privacy & Security → Full Disk Access", path)
	openSettingsOnce.Do(func() {
		log.Println("Opening System Settings → Privacy & Security → Full Disk Access")
		exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles").Start()
	})
}

func gracefulShutdown(db *sql.DB, es *fsevents.EventStream) {
	es.Stop()
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
	os.Exit(0)
}

func setupSignalHandlers(db *sql.DB, es *fsevents.EventStream) {
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGTERM)

	intChan := make(chan os.Signal, 1)
	signal.Notify(intChan, syscall.SIGINT)

	usr1Chan := make(chan os.Signal, 1)
	signal.Notify(usr1Chan, syscall.SIGUSR1)

	go func() {
		log.Println("Signal handler thread started.")
		for {
			select {
			case <-termChan:
				log.Println("Received SIGTERM, shutting down.")
				gracefulShutdown(db, es)
			case <-intChan:
				log.Println("Received SIGINT (Ctrl+C), shutting down.")
				gracefulShutdown(db, es)
			case <-usr1Chan:
				log.Println("Received SIGUSR1. Starting scan...")
				scanDisk(config.MonitorPath)
				deleteMissing(config.MonitorPath)
			}
		}
	}()
}

func buildEventRecord(fsevent *fsevents.Event) *shared.EventRecord {
	var objType shared.ObjectType

	isFile := fsevent.Flags&fsevents.ItemIsFile == fsevents.ItemIsFile
	isDir := fsevent.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir
	isSymlink := fsevent.Flags&fsevents.ItemIsSymlink == fsevents.ItemIsSymlink

	switch {
	case isFile:
		objType = shared.ItemIsFile
	case isDir:
		objType = shared.ItemIsDir
	case isSymlink:
		objType = shared.ItemIsSymlink
	}

	if verbose {
		note := ""
		for bit, description := range noteDescription {
			if fsevent.Flags&bit == bit {
				note += description + " "
			}
		}
		log.Printf("Event: %s created=%d removed=%d renamed=%d note=%s", fsevent.Path, fsevent.Flags&fsevents.ItemCreated, fsevent.Flags&fsevents.ItemRemoved, fsevent.Flags&fsevents.ItemRenamed, note)
	}

	isRenamed := fsevent.Flags&fsevents.ItemRenamed == fsevents.ItemRenamed

	return &shared.EventRecord{
		Filename:   filepath.Base(fsevent.Path),
		Path:       fsevent.Path,
		ObjectType: objType,
		EventID:    fsevent.ID,
		EventTime:  time.Now().UnixNano(),
		IsRename:   isRenamed,
	}
}

func main() {
	config := getCommandLineArgs()
	log.Printf("Starting service with PID %d (%s)", os.Getpid(), version.Info())

	db, dbIsNew := setupDatabase(config.DBPath)
	if db == nil {
		log.Fatal("Failed to open database")
	}
	defer func() {
		log.Println("Closing database")
		db.Close()
	}()

	dbChannel = make(chan *shared.EventRecord, 5000)
	go databaseWriter(db, config.NoCache)

	if dbIsNew {
		log.Println("Database is new. Scanning disk.")
		scanDisk(config.MonitorPath)
	}

	if !shared.FileExists(config.MonitorPath) {
		log.Fatalf("Monitor path does not exist: %s", config.MonitorPath)
	}

	dev, err := fsevents.DeviceForPath(config.MonitorPath)
	if err != nil {
		log.Fatalf("Failed to retrieve device for path: %v", err)
	}

	log.Printf("Monitoring path: %s.  Device %d UUID %d", config.MonitorPath, dev, fsevents.EventIDForDeviceBeforeTime(dev, time.Now()))

	var latency time.Duration = 10 * time.Second
	if config.NoCache {
		latency = 500 * time.Millisecond
		log.Printf("No cache is enabled. %dms fsevents and all events will be written to the database immediately.", latency.Milliseconds())
	}

	es := &fsevents.EventStream{
		Events:  make(chan []fsevents.Event, 30000),
		Paths:   []string{config.MonitorPath},
		Latency: latency,
		Device:  0,
		Flags:   fsevents.FileEvents,
	}
	es.Start()

	go func() {
		scanTimer := time.Now()
		log.Println("Event listener ready.")

		for msg := range es.Events {
			for _, event := range msg {
				if shouldIgnorePath(event.Path) {
					if verbose {
						log.Printf("Ignoring path: %s", event.Path)
					}
					continue
				}
				eventRecord := buildEventRecord(&event)
				if eventRecord != nil {
					dbChannel <- eventRecord
				}
				if event.Flags&fsevents.MustScanSubDirs == fsevents.MustScanSubDirs {
					log.Printf("Warning: MustScanSubdirs found for %s", event.Path)
				}
			}
			if time.Since(scanTimer) > 2*time.Hour {
				scanHomeDirs()
				scanTimer = time.Now()
			}
		}
	}()

	setupSignalHandlers(db, es)

	// Keep the program running
	select {}
}
