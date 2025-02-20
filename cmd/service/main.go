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

	"strings"
	"syscall"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/shared"

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

var dbChannel chan *shared.EventRecord

const ignorePath = "/System/Volumes/Data"

var verbose bool

type Config struct {
	MonitorPath string
	DBPath      string
	NoCache     bool
	Verbose     bool
}

func getCommandLineArgs() Config {
	config := Config{}
	flag.StringVar(&config.MonitorPath, "monitor_path", "/", "Path to monitor for file system events")
	flag.StringVar(&config.DBPath, "db_path", "/var/lib/findfiles/files.db", "Path to the database file")
	flag.BoolVar(&config.NoCache, "nocache", false, "Disable caching")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.Parse()

	if config.MonitorPath == "" {
		log.Fatal("Monitor path is required")
	}
	if config.DBPath == "" {
		log.Fatal("Database path is required")
	}
	if config.NoCache {
		log.Println("--nocache is enabled.")
	}
	if config.Verbose {
		log.Println("--verbose logging is enabled.")
		verbose = true
	}

	return config
}

func setupDatabase(databasePath string) (*sql.DB, bool) {
	var err error
	var db *sql.DB
	var new = false

	if shared.FileExists(databasePath) {
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
		new = true
	}

	count, err := ffdb.RecordCount(db)
	if err != nil {
		log.Fatal("Error getting record count: ", err)
	}
	log.Printf("Opened database %s. %d records", databasePath, count)

	return db, new
}

func gracefulShutdown(db *sql.DB, es *fsevents.EventStream) {
	es.Stop()

	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
	os.Exit(0)
}

func main() {
	log.Println("Starting service with PID", os.Getpid())
	config := getCommandLineArgs()

	db, dbIsNew := setupDatabase(config.DBPath)
	if db == nil {
		log.Fatal("Failed to open database")
	}
	defer func() {
		log.Println("Closing database")
		db.Close()
	}()

	// Start the database writer thread
	dbChannel = make(chan *shared.EventRecord, 5000)
	go databaseWriter(db, config.NoCache)

	if dbIsNew {
		log.Println("Database is new. Scanning disk.")
		scanDisk()
	}

	// Start the File System Event listener
	if !shared.FileExists(config.MonitorPath) {
		log.Fatalf("Monitor path does not exist: %s", config.MonitorPath)
	}

	dev, err := fsevents.DeviceForPath(config.MonitorPath)
	if err != nil {
		log.Fatalf("Failed to retrieve device for path: %v", err)
	}

	log.Printf("Monitoring path: %s.  Device %d UUID %d", config.MonitorPath, dev, fsevents.EventIDForDeviceBeforeTime(dev, time.Now()))

	es := &fsevents.EventStream{
		Events:  make(chan []fsevents.Event, 30000), // Provide our own channel to buffer events and prevent hangs
		Paths:   []string{config.MonitorPath},
		Latency: 60 * time.Second,
		Device:  0,
		Flags:   fsevents.FileEvents}
	es.Start()

	go func() {
		log.Println("Event listener ready.")
		for msg := range es.Events {
			for _, event := range msg {
				if strings.HasPrefix(event.Path, ignorePath) {
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
		}
	}()

	// Setup SIGTERM, SIGUSR1, SIGUSR2 listeners
	setupSignalHandlers(db, es)

	// Keep the program running
	select {}
}

func setupSignalHandlers(db *sql.DB, es *fsevents.EventStream) {
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
				log.Println("Received SIGTERM, shutting down.")
				gracefulShutdown(db, es)
			case <-intChan:
				log.Println("Received SIGINT (Ctrl+C), shutting down.")
				gracefulShutdown(db, es)
			case <-usr1Chan:
				log.Println("Received SIGUSR1. Starting scan...")
				scanDisk()
			case <-usr2Chan:
				log.Println("Received SIGUSR2. Starting db audit...")
				deleteMissing(db)
			}
		}
	}()
}

func buildEventRecord(fsevent *fsevents.Event) *shared.EventRecord {
	var objType shared.ObjectType

	isFile := fsevent.Flags&fsevents.ItemIsFile == fsevents.ItemIsFile
	isDir := fsevent.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir
	isSymlink := fsevent.Flags&fsevents.ItemIsSymlink == fsevents.ItemIsSymlink

	if isFile {
		objType = shared.ItemIsFile
	} else if isDir {
		objType = shared.ItemIsDir
	} else if isSymlink {
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

	eventRecord := &shared.EventRecord{
		Filename:   filepath.Base(fsevent.Path),
		Path:       fsevent.Path,
		ObjectType: objType,
		EventID:    fsevent.ID,
		EventTime:  time.Now().UnixNano(),
	}

	return eventRecord
}

// Create an delay before writing to the db.  Apple's File System Events seems
// to send file create events but the files are quickly deleted or don't exist.
func addEventToQueue(db *sql.DB, lastFlushTime *time.Time, eventRecordQueue *[]shared.EventRecord, event *shared.EventRecord, noCache bool) {
	maxQueueSize := 100000 // Set a queue size limit to keep memory usage in check
	if noCache {
		maxQueueSize = 1
	}
	var delayTime time.Duration = 60 * time.Second

	*eventRecordQueue = append(*eventRecordQueue, *event)

	tt := time.Since(*lastFlushTime)
	if tt >= delayTime || len(*eventRecordQueue) >= maxQueueSize {
		err := ffdb.BulkStoreEvents(db, eventRecordQueue)
		if err != nil {
			log.Println("Error writing to db: ", err)
		}
		*eventRecordQueue = (*eventRecordQueue)[:0] // clear the queue
		*lastFlushTime = time.Now()
	}
}

// Scan the database for file paths and checks if they still exist on the filesystem.
// If a file is missing, it creates an EventRecord with a delete action.
func deleteMissing(db *sql.DB) {
	startTime := time.Now()

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

		if !shared.FileExists(fullpath) {
			eventRecord := &shared.EventRecord{
				Filename:   "",
				Path:       fullpath,
				ObjectType: 0,
				EventID:    0,
				EventTime:  0,
			}

			dbChannel <- eventRecord
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

func scanDisk() {
	// TODO: figure out how to use a separate eventRecordQueue for the scan that can be deleted when the scan is done.
	//       It would save about 150MB ram after a scan
	startTime := time.Now()

	var fileCount, dirCount, linkCount, pipeCount, socketCount, charDeviceCount, blockDeviceCount int
	var objType shared.ObjectType = shared.ItemIsFile

	filepath.WalkDir("/", func(path string, file fs.DirEntry, err error) error {
		if strings.HasPrefix(path, ignorePath) {
			return nil
		}
		switch {
		case file.Type().IsRegular():
			fileCount++
			objType = shared.ItemIsFile
		case file.IsDir():
			dirCount++
			objType = shared.ItemIsDir
		case file.Type()&fs.ModeSymlink != 0:
			linkCount++
			objType = shared.ItemIsSymlink
		case file.Type()&fs.ModeNamedPipe != 0:
			pipeCount++
			objType = shared.ItemIsNamedPipe
		case file.Type()&fs.ModeSocket != 0:
			socketCount++
			objType = shared.ItemIsSocket
		case file.Type()&fs.ModeCharDevice != 0:
			charDeviceCount++
			objType = shared.ItemIsCharDevice
		case file.Type()&fs.ModeDevice != 0:
			blockDeviceCount++
			objType = shared.ItemIsBlockDevice
		default:
			log.Printf("Unknown file type: %s", path)
		}

		eventRecord := &shared.EventRecord{
			Filename:    file.Name(),
			Path:        path,
			ObjectType:  objType,
			EventID:     0,
			EventTime:   0,
			FoundOnScan: true,
		}

		dbChannel <- eventRecord

		return nil
	})

	log.Printf("Scan complete. Files/dirs: %d/%d. Pipes: %d, Sockets: %d, Char Devices: %d, Block Devices: %d. Elapsed time: %s", fileCount, dirCount, pipeCount, socketCount, charDeviceCount, blockDeviceCount, time.Since(startTime).String())
}

func databaseWriter(db *sql.DB, noCache bool) {
	log.Println("databaseWriter thread started.")

	var lastFlushTime time.Time = time.Now()
	var eventRecordQueue = []shared.EventRecord{}
	log.Printf("Queue %p", &eventRecordQueue)

	for event := range dbChannel {
		addEventToQueue(db, &lastFlushTime, &eventRecordQueue, event, noCache)
	}

	log.Println("databaseWriter thread stopped.")
}
