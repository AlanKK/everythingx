//go:build darwin

package main

import (
	"database/sql"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/models"

	"github.com/fsnotify/fsevents"
)

// Installing launchd service:
// sudo cp /Users/alan/Documents/git/fsevents/example/filefind.plist /Library/LaunchAgents/filefind.plist
// sudo chmod 644 /Library/LaunchDaemons/com.example.filefind.plist
// sudo chown root:wheel /Library/LaunchDaemons/com.example.filefind.plist
// sudo launchctl bootstrap system /Library/LaunchDaemons/com.example.filefind.plist
// sudo launchctl bootout system /Library/LaunchDaemons/com.example.filefind.plist

// todo:
// 	Read historical events from FSEvents
//	Create installer for launchd service

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

var delayTime time.Duration = 5 * time.Second

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func getCommandLineArgs() (*string, *string) {
	monitorPath := flag.String("monitor_path", "/", "Path to monitor for file system events")
	dbPath := flag.String("db_path", "/var/lib/filefind/files.db", "Path to the database file")
	flag.Parse()

	if *monitorPath == "" {
		log.Fatal("Monitor path is required")
	}
	if *dbPath == "" {
		log.Fatal("Database path is required")
	}
	return monitorPath, dbPath
}

func setupDatabase(databasePath string) *sql.DB {
	var err error
	var db *sql.DB

	if fileExists(databasePath) {
		db, err = ffdb.OpenDB(databasePath)
		if err != nil {
			log.Println("Database does not exist: ", databasePath)
		}
	} else {
		log.Println("Database does not exist. Creating ", databasePath)
		db, err = ffdb.CreateDB(databasePath)
		if err != nil {
			log.Fatal("Error creating database: ", err)
		}
	}

	count, err := ffdb.RecordCount(db)
	if err != nil {
		log.Fatal("Error getting record count: ", err)
	}
	log.Printf("Opened database %s. %d records", databasePath, count)

	return db
}

func gracefulShutdown(db *sql.DB) {
	log.Println("Shutting down.")
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
	os.Exit(0)
}

func main() {
	monitorPath, dbPath := getCommandLineArgs()

	db := setupDatabase(*dbPath)
	if db == nil {
		log.Fatal("Failed to open database")
	}
	defer db.Close()

	// Setup SIGTERM, SIGUSR1, SIGUSR2 listeners
	setupSignalHandlers(db)

	dev, err := fsevents.DeviceForPath(*monitorPath)
	if err != nil {
		log.Fatalf("Failed to retrieve device for path: %v", err)
	}

	log.Printf("Monitoring path: %s.  Device %d UUID %d", *monitorPath, dev, fsevents.EventIDForDeviceBeforeTime(dev, time.Now()))

	es := &fsevents.EventStream{
		Paths:   []string{*monitorPath},
		Latency: 500 * time.Millisecond,
		Device:  dev,
		Flags:   fsevents.FileEvents | fsevents.WatchRoot}
	es.Start()
	ec := es.Events

	log.Println("Device UUID for ", *monitorPath, fsevents.GetDeviceUUID(dev))

	// TODO: this needs to be fast.  Is it doing too much with the bulk db writes?
	go func() {
		startTime := time.Now()
		var lastFlushTime time.Time = time.Now()
		var eventRecordQueue = &[]models.EventRecord{}
		//log.Printf("main() - &eventRecordQueue: %p", eventRecordQueue)

		for msg := range ec {
			for _, event := range msg {
				eventRecord := buildEventRecord(event)
				if eventRecord != nil {
					addEventToQueue(db, &lastFlushTime, eventRecordQueue, eventRecord)
				}
			}
		}
		log.Println("fsevent callback time: ", time.Since(startTime))
	}()

	// Keep the program running
	select {}
}

func setupSignalHandlers(db *sql.DB) {
	// TODO: does this need a new db connection or is reuse OK?
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGTERM)

	// Handle SIGINT (Ctrl+C) to shutdown gracefully
	intChan := make(chan os.Signal, 1)
	signal.Notify(intChan, syscall.SIGINT)

	// Handle SIGUSR1 to trigger disk scan
	usr1Chan := make(chan os.Signal, 1)
	signal.Notify(usr1Chan, syscall.SIGUSR1)

	// Handle SIGUSR2 to trigger disk scan
	usr2Chan := make(chan os.Signal, 1)
	signal.Notify(usr2Chan, syscall.SIGUSR2)

	go func() {
		log.Println("Signal handler thread started.")
		for {
			select {
			case <-termChan:
				log.Println("Received SIGTERM, shutting down gracefully.")
				gracefulShutdown(db)
			case <-intChan:
				log.Println("Received SIGINT (Ctrl+C), shutting down gracefully.")
				gracefulShutdown(db)
			case <-usr1Chan:
				log.Println("Received SIGUSR1. Starting scan...")
				scanDisk(db)
			case <-usr2Chan:
				log.Println("Received SIGUSR2. Starting db audit...")
				deleteMissing(db)
			}
		}
	}()
}

func buildEventRecord(fsevent fsevents.Event) *models.EventRecord {
	note := ""
	var objType models.ObjectType
	var eventAction models.EventAction

	if fsevent.Flags&fsevents.ItemIsFile == fsevents.ItemIsFile {
		objType = models.ItemIsFile
	} else if fsevent.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir {
		objType = models.ItemIsDir
	} else if fsevent.Flags&fsevents.ItemIsSymlink == fsevents.ItemIsSymlink {
		objType = models.ItemIsSymlink
	}

	path := filepath.Join("/", fsevent.Path)

	for bit, description := range noteDescription {
		if fsevent.Flags&bit == bit {
			note += description + " "
		}
	}
	//log.Printf("event_id: %d Path: %s Flags: %s", fsevent.ID, fsevent.Path, note)

	switch {
	case fsevent.Flags&fsevents.ItemCreated == fsevents.ItemCreated:
		eventAction = models.ItemCreated
	case fsevent.Flags&fsevents.ItemRemoved == fsevents.ItemRemoved:
		eventAction = models.ItemDeleted
	case fsevent.Flags&fsevents.ItemRenamed == fsevents.ItemRenamed:
		// We can't tell if the path in the event is the "before" or "after" path of
		// a rename so checking if the path exists will tell us.
		if fileExists(fsevent.Path) {
			eventAction = models.ItemCreated
		} else {
			eventAction = models.ItemDeleted
		}
	default:
		return nil
	}

	eventRecord := &models.EventRecord{
		Filename:    filepath.Base(path),
		Path:        path,
		ObjectType:  objType,
		EventID:     fsevent.ID,
		EventTime:   time.Now().UnixNano(), // int64
		EventAction: eventAction,
	}

	return eventRecord
}

// Create an delay before writing to the db.  Apple's File System Events seems
// to send file create events but the files are quickly deleted or don't exist.
func addEventToQueue(db *sql.DB, lastFlushTime *time.Time, eventRecordQueue *([]models.EventRecord), event *models.EventRecord) {

	*eventRecordQueue = append(*eventRecordQueue, *event)

	tt := time.Since(*lastFlushTime)
	if tt >= delayTime {
		err := ffdb.BulkStoreEvents(db, eventRecordQueue)
		if err != nil {
			log.Println("Error writing to db: ", err)
		}
		*eventRecordQueue = (*eventRecordQueue)[:0] // clear the queue
		*lastFlushTime = time.Now()
	}
}

func deleteMissing(db *sql.DB) {
	startTime := time.Now()
	var lastFlushTime time.Time = time.Now()
	var eventRecordQueue = &[]models.EventRecord{}

	rows, err := db.Query("SELECT fullpath FROM files")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var totalFiles, filesExist, filesMissing int

	for rows.Next() {
		var fullpath string
		if err := rows.Scan(&fullpath); err != nil {
			log.Fatal(err)
		}

		if !fileExists(fullpath) {
			eventRecord := &models.EventRecord{
				Filename:    "",
				Path:        fullpath,
				ObjectType:  0,
				EventID:     0,
				EventTime:   0,
				EventAction: models.ItemDeleted,
			}

			addEventToQueue(db, &lastFlushTime, eventRecordQueue, eventRecord)
			filesMissing++
		} else {
			filesExist++
		}
		totalFiles++
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	log.Printf("DB Audit complete. Total files %d, found %d, missing %d, elapsed time %s", totalFiles, filesExist, filesMissing, time.Since(startTime).String())
}

func scanDisk(db *sql.DB) {
	startTime := time.Now()

	var fileCount int
	var dirCount int
	var objType models.ObjectType = models.ItemIsFile
	var lastFlushTime time.Time = time.Now()
	var eventRecordQueue = &[]models.EventRecord{}

	filepath.WalkDir("/", func(path string, file fs.DirEntry, err error) error {
		if !file.IsDir() {
			fileCount++
			objType = models.ItemIsFile
		} else {
			dirCount++
			objType = models.ItemIsDir
		}

		eventRecord := &models.EventRecord{
			Filename:    file.Name(),
			Path:        path,
			ObjectType:  objType,
			EventID:     0,
			EventTime:   0, // int64
			EventAction: models.ItemCreated,
		}

		addEventToQueue(db, &lastFlushTime, eventRecordQueue, eventRecord)

		return nil
	})

	// Force a flush to the db
	eventRecord := &models.EventRecord{
		Filename:    "/",
		Path:        "/",
		ObjectType:  models.ItemIsDir,
		EventID:     0,
		EventTime:   0, // int64
		EventAction: models.ItemCreated,
	}
	lastFlushTime = time.Time{} // force a flush
	addEventToQueue(db, &lastFlushTime, eventRecordQueue, eventRecord)

	log.Printf("Scan complete. Files/dirs: %d/%d. Elapsed time: %s", fileCount, dirCount, time.Since(startTime).String())
}
