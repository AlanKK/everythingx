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

	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/models"

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

var dbChannel chan *models.EventRecord

const ignorePath = "/System/Volumes/Data"

var verbose bool

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func getCommandLineArgs() (string, string, bool) {
	monitorPath := flag.String("monitor_path", "/", "Path to monitor for file system events")
	dbPath := flag.String("db_path", "/var/lib/findfiles/files.db", "Path to the database file")
	noCache := flag.Bool("nocache", false, "Disable caching")
	v := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	if *monitorPath == "" {
		log.Fatal("Monitor path is required")
	}
	if *dbPath == "" {
		log.Fatal("Database path is required")
	}
	if *noCache {
		log.Println("--nocache is enabled.")
	}
	if *v {
		log.Println("--verbose logging is enabled.")
	}
	verbose = *v
	return *monitorPath, *dbPath, *noCache
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

func gracefulShutdown(db *sql.DB, es *fsevents.EventStream) {
	pprof.StopCPUProfile()

	es.Stop()

	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
	os.Exit(0)
}

func main() {
	log.Println("Starting service with PID", os.Getpid())
	monitorPath, dbPath, noCache := getCommandLineArgs()

	db := setupDatabase(dbPath)
	if db == nil {
		log.Fatal("Failed to open database")
	}
	defer func() {
		log.Println("Closing database")
		db.Close()
	}()

	// Start the database writer thread
	dbChannel = make(chan *models.EventRecord, 5000)
	go databaseWriter(db, noCache)

	// Start the File System Event listener
	if !fileExists(monitorPath) {
		log.Fatalf("Monitor path does not exist: %s", monitorPath)
	}

	dev, err := fsevents.DeviceForPath(monitorPath)
	if err != nil {
		log.Fatalf("Failed to retrieve device for path: %v", err)
	}

	// Start profiling
	f, err := os.Create("/var/lib/findfiles/findfilesd.prof")
	if err != nil {
		log.Println(err)
		return
	}
	pprof.StartCPUProfile(f)

	log.Printf("Monitoring path: %s.  Device %d UUID %d", monitorPath, dev, fsevents.EventIDForDeviceBeforeTime(dev, time.Now()))

	es := &fsevents.EventStream{
		Events:  make(chan []fsevents.Event, 10000), // Provide our own channel to buffer events and prevent hangs
		Paths:   []string{monitorPath},
		Latency: 5 * time.Second,
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

func buildEventRecord(fsevent *fsevents.Event) *models.EventRecord {
	note := ""
	var objType models.ObjectType

	isFile := fsevent.Flags&fsevents.ItemIsFile == fsevents.ItemIsFile
	isDir := fsevent.Flags&fsevents.ItemIsDir == fsevents.ItemIsDir
	isSymlink := fsevent.Flags&fsevents.ItemIsSymlink == fsevents.ItemIsSymlink

	if isFile {
		objType = models.ItemIsFile
	} else if isDir {
		objType = models.ItemIsDir
	} else if isSymlink {
		objType = models.ItemIsSymlink
	}

	for bit, description := range noteDescription {
		if fsevent.Flags&bit == bit {
			note += description + " "
		}
	}

	if verbose {
		log.Printf("Event: %s created=%d removed=%d renamed=%d note=%s", fsevent.Path, fsevent.Flags&fsevents.ItemCreated, fsevent.Flags&fsevents.ItemRemoved, fsevent.Flags&fsevents.ItemRenamed, note)
	}

	eventRecord := &models.EventRecord{
		Filename:   filepath.Base(fsevent.Path),
		Path:       fsevent.Path,
		ObjectType: objType,
		EventID:    fsevent.ID,
		EventTime:  time.Now().UnixNano(),
	}

	if verbose {
		log.Printf("Event: %s", fsevent.Path)
	}

	return eventRecord
}

// Create an delay before writing to the db.  Apple's File System Events seems
// to send file create events but the files are quickly deleted or don't exist.
func addEventToQueue(db *sql.DB, lastFlushTime *time.Time, eventRecordQueue *[]models.EventRecord, event *models.EventRecord, noCache bool) {
	maxQueueSize := 100000 // Set a queue size limit to keep memory usage in check
	if noCache {
		maxQueueSize = 1
	}
	var delayTime time.Duration = 5 * time.Second

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

		if !fileExists(fullpath) {
			eventRecord := &models.EventRecord{
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

	var fileCount, dirCount int
	var objType models.ObjectType = models.ItemIsFile

	filepath.WalkDir("/", func(path string, file fs.DirEntry, err error) error {
		if strings.HasPrefix(path, ignorePath) {
			return nil
		}
		if file.IsDir() {
			dirCount++
			objType = models.ItemIsDir
		} else {
			fileCount++
			objType = models.ItemIsFile
		}

		eventRecord := &models.EventRecord{
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

	log.Printf("Scan complete. Files/dirs: %d/%d. Elapsed time: %s", fileCount, dirCount, time.Since(startTime).String())
}

func databaseWriter(db *sql.DB, noCache bool) {
	log.Println("databaseWriter thread started.")

	var lastFlushTime time.Time = time.Now()
	var eventRecordQueue = []models.EventRecord{}
	log.Printf("Queue %p", &eventRecordQueue)

	for event := range dbChannel {
		addEventToQueue(db, &lastFlushTime, &eventRecordQueue, event, noCache)
	}

	log.Println("databaseWriter thread stopped.")
}
