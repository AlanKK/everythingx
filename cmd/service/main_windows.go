//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"
	"golang.org/x/sys/windows"
)

func shouldIgnorePath(path string) bool {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if strings.EqualFold(path, config.DBPath+suffix) {
			return true
		}
	}
	return false
}

func reportPermissionError(path string) {
	log.Printf("Permission denied scanning %s", path)
}

type fileChange struct {
	path   string
	action uint32
}

func parseFileChanges(root string, data []byte) ([]fileChange, error) {
	var changes []fileChange
	for offset := 0; offset < len(data); {
		if len(data)-offset < 12 {
			return nil, fmt.Errorf("short file change record")
		}
		next := binary.LittleEndian.Uint32(data[offset:])
		action := binary.LittleEndian.Uint32(data[offset+4:])
		nameBytes := binary.LittleEndian.Uint32(data[offset+8:])
		if nameBytes%2 != 0 || nameBytes > uint32(len(data)-offset-12) {
			return nil, fmt.Errorf("invalid file change name length")
		}
		name := make([]uint16, nameBytes/2)
		for i := range name {
			name[i] = binary.LittleEndian.Uint16(data[offset+12+i*2:])
		}
		changes = append(changes, fileChange{
			path:   filepath.Join(root, string(utf16.Decode(name))),
			action: action,
		})
		if next == 0 {
			break
		}
		if next < 12 || next > uint32(len(data)-offset) {
			return nil, fmt.Errorf("invalid file change offset")
		}
		offset += int(next)
	}
	return changes, nil
}

func openDirectoryWatcher(root string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(name, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
}

func enqueueFileChange(change fileChange) {
	if shouldIgnorePath(change.path) {
		return
	}
	switch change.action {
	case windows.FILE_ACTION_ADDED, windows.FILE_ACTION_RENAMED_NEW_NAME:
		info, err := os.Lstat(change.path)
		if err != nil {
			return
		}
		if info.IsDir() {
			scanDisk(change.path, "")
			return
		}
		kind := shared.ItemIsFile
		if info.Mode()&fs.ModeSymlink != 0 {
			kind = shared.ItemIsSymlink
		}
		dbChannel <- &shared.EventRecord{Filename: filepath.Base(change.path), Path: change.path, ObjectType: kind, EventTime: time.Now().UnixNano()}
	case windows.FILE_ACTION_REMOVED, windows.FILE_ACTION_RENAMED_OLD_NAME:
		// A directory removal also removes every indexed descendant. The same
		// range delete is harmless when the removed path was a regular file.
		dbChannel <- &shared.EventRecord{Filename: filepath.Base(change.path), Path: change.path, ObjectType: shared.ItemIsDir, Deleted: true, EventTime: time.Now().UnixNano()}
	}
}

func watchDirectory(root string, handle windows.Handle, rescan chan<- struct{}) {
	requestRescan := func() {
		select {
		case rescan <- struct{}{}:
		default:
		}
	}
	const mask = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME
	buf := make([]byte, 64*1024)
	for {
		if handle == 0 {
			time.Sleep(time.Second)
			var err error
			handle, err = openDirectoryWatcher(root)
			if err != nil {
				log.Printf("Windows directory watcher reopen failed: %v", err)
				continue
			}
			requestRescan()
		}
		var n uint32
		err := windows.ReadDirectoryChanges(handle, &buf[0], uint32(len(buf)), true, mask, &n, nil, 0)
		if err != nil || n == 0 {
			log.Printf("Windows directory watcher lost events: %v", err)
			requestRescan()
			if err != nil {
				windows.CloseHandle(handle)
				handle = 0
			}
			continue
		}
		changes, err := parseFileChanges(root, buf[:n])
		if err != nil {
			log.Printf("Windows directory watcher invalid event: %v", err)
			requestRescan()
			continue
		}
		for _, change := range changes {
			enqueueFileChange(change)
		}
	}
}

func reconcileWindows(root string) {
	scanDisk(root, "")
	if err := flushDatabaseWriter(); err != nil {
		log.Fatalf("Index scan failed: %v", err)
	}
	deleteMissing(root)
	if err := flushDatabaseWriter(); err != nil {
		log.Fatalf("Index cleanup failed: %v", err)
	}
}

func main() {
	config := getCommandLineArgs()
	var err error
	config.DBPath, err = filepath.Abs(config.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	root, err := filepath.Abs(config.MonitorPath)
	if err != nil {
		log.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		log.Fatalf("Monitor path must be a directory: %s: %v", root, err)
	}
	log.Printf("Starting service with PID %d (%s)", os.Getpid(), version.Info())
	db, _ := setupDatabase(config.DBPath)
	defer db.Close()
	dbChannel = make(chan *shared.EventRecord, 5000)
	go databaseWriter(db, config.NoCache)

	handle, err := openDirectoryWatcher(root)
	if err != nil {
		log.Fatalf("Cannot watch %s: %v", root, err)
	}
	rescan := make(chan struct{}, 1)
	go watchDirectory(root, handle, rescan)
	reconcileWindows(root)
	log.Printf("Windows directory watcher ready for %s", root)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-signals:
			if err := flushDatabaseWriter(); err != nil {
				log.Printf("Final index flush failed: %v", err)
			}
			return
		case <-rescan:
			reconcileWindows(root)
		case <-ticker.C:
			reconcileWindows(root)
		}
	}
}
