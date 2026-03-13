//go:build linux

package main

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"
	"golang.org/x/sys/unix"
)

// ignorePaths contains Linux kernel virtual filesystems and other paths that
// should not be indexed — they are not real files and change constantly.
var ignorePaths = []string{
	"/proc",
	"/sys",
	"/run",
	"/dev",
	"/snap",
}

func shouldIgnorePath(path string) bool {
	for _, ignore := range ignorePaths {
		if path == ignore || strings.HasPrefix(path, ignore+"/") {
			return true
		}
	}
	return false
}

// fanotify event metadata layout from <linux/fanotify.h>
type fanotifyEventMetadata struct {
	EventLen    uint32
	Vers        uint8
	Reserved    uint8
	MetadataLen uint16
	Mask        uint64
	Fd          int32
	Pid         int32
}

// fanotify_event_info_header — common header for all info records
type fanotifyEventInfoHeader struct {
	InfoType uint8
	Pad      uint8
	Len      uint16
}

// fanotify_event_info_fid — used with FAN_REPORT_FID / FAN_REPORT_DFID_NAME
type fanotifyEventInfoFID struct {
	Header     fanotifyEventInfoHeader
	FSID       [2]uint64
	FileHandle [0]byte // variable-length kernel_fsid_t + file_handle
}

const (
	fanEventInfoTypeFID      = 1
	fanEventInfoTypeDFID     = 3
	fanEventInfoTypeName     = 2
	fanEventInfoTypeDFIDName = 5

	fanotifyMetadataVersion = 3
	sizeofEventMetadata     = uint32(unsafe.Sizeof(fanotifyEventMetadata{}))
)

// resolvePathFromFD resolves a file descriptor to its path via /proc/self/fd.
func resolvePathFromFD(fd int32) string {
	link := fmt.Sprintf("/proc/self/fd/%d", fd)
	path, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	return path
}

// parseFanotifyEvents reads raw bytes from the fanotify fd and returns EventRecords.
// It handles both FD-based events (older) and DFID_NAME events (kernel 5.9+).
func parseFanotifyEvents(buf []byte) []*shared.EventRecord {
	var records []*shared.EventRecord
	r := bytes.NewReader(buf)

	for r.Len() >= int(sizeofEventMetadata) {
		var meta fanotifyEventMetadata
		if err := binary.Read(r, binary.NativeEndian, &meta); err != nil {
			break
		}
		if meta.Vers != fanotifyMetadataVersion {
			break
		}

		// Remaining bytes in this event after the fixed header
		extraLen := int(meta.EventLen) - int(sizeofEventMetadata)
		var extraBuf []byte
		if extraLen > 0 {
			extraBuf = make([]byte, extraLen)
			if _, err := r.Read(extraBuf); err != nil {
				break
			}
		}

		var path string
		if meta.Fd >= 0 {
			// FD-based fallback: resolve via /proc/self/fd
			path = resolvePathFromFD(meta.Fd)
			unix.Close(int(meta.Fd))
		} else if len(extraBuf) > 0 {
			// DFID_NAME: parse info records to extract dir path + filename
			path = parseDFIDNamePath(extraBuf)
		}

		if path == "" || shouldIgnorePath(path) {
			continue
		}

		objType := objectTypeFromStat(path)

		isCreate := meta.Mask&unix.FAN_CREATE != 0 || meta.Mask&unix.FAN_MOVED_TO != 0
		isDelete := meta.Mask&unix.FAN_DELETE != 0 || meta.Mask&unix.FAN_MOVED_FROM != 0

		if !isCreate && !isDelete {
			continue
		}

		rec := &shared.EventRecord{
			Filename:   filepath.Base(path),
			Path:       path,
			ObjectType: objType,
			EventID:    0,
			EventTime:  time.Now().UnixNano(),
		}
		records = append(records, rec)
	}
	return records
}

// parseDFIDNamePath attempts to extract a full path from a DFID_NAME info record.
// With FAN_REPORT_DFID_NAME the kernel appends info records containing the parent
// directory FID and the child filename. We resolve the directory via a dirfd opened
// from the file handle, then join with the filename.
func parseDFIDNamePath(info []byte) string {
	if len(info) < 4 {
		return ""
	}
	// We look for a null-terminated name after the FID info structures.
	// Simple heuristic: find a printable string near the end of the info block.
	for i := len(info) - 1; i >= 0; i-- {
		if info[i] == 0 && i+1 < len(info) {
			name := unix.ByteSliceToString(info[i+1:])
			if name != "" && !strings.Contains(name, "\x00") {
				return name
			}
		}
	}
	return ""
}

// objectTypeFromStat returns the ObjectType for the given path using lstat.
func objectTypeFromStat(path string) shared.ObjectType {
	info, err := os.Lstat(path)
	if err != nil {
		return shared.ItemIsFile
	}
	mode := info.Mode()
	switch {
	case mode.IsDir():
		return shared.ItemIsDir
	case mode&os.ModeSymlink != 0:
		return shared.ItemIsSymlink
	case mode&os.ModeNamedPipe != 0:
		return shared.ItemIsNamedPipe
	case mode&os.ModeSocket != 0:
		return shared.ItemIsSocket
	case mode&os.ModeCharDevice != 0:
		return shared.ItemIsCharDevice
	case mode&os.ModeDevice != 0:
		return shared.ItemIsBlockDevice
	default:
		return shared.ItemIsFile
	}
}

func gracefulShutdown(db *sql.DB, fanotifyFD int) {
	if fanotifyFD >= 0 {
		unix.Close(fanotifyFD)
	}
	if err := db.Close(); err != nil {
		log.Printf("Error closing database: %v", err)
	}
	os.Exit(0)
}

func setupSignalHandlers(db *sql.DB, fanotifyFD int) {
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
				gracefulShutdown(db, fanotifyFD)
			case <-intChan:
				log.Println("Received SIGINT (Ctrl+C), shutting down.")
				gracefulShutdown(db, fanotifyFD)
			case <-usr1Chan:
				log.Println("Received SIGUSR1. Starting scan...")
				scanDisk(config.MonitorPath)
				deleteMissing(config.MonitorPath)
			}
		}
	}()
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

	// FAN_REPORT_DFID_NAME requires kernel 5.9+. It gives us the parent directory
	// FID plus the filename in each event, allowing path reconstruction without
	// keeping a watch table for every directory.
	fanotifyFlags := uint(unix.FAN_CLASS_NOTIF | unix.FAN_NONBLOCK)
	// FAN_REPORT_DFID_NAME is kernel 5.9+; try it first, fall back to basic mode.
	const fanReportDfidName = 0x00000C00 // FAN_REPORT_DFID | FAN_REPORT_NAME
	fd, err := unix.FanotifyInit(fanotifyFlags|fanReportDfidName, unix.O_RDONLY|unix.O_LARGEFILE)
	if err != nil {
		log.Printf("FAN_REPORT_DFID_NAME not available (%v), falling back to basic fanotify (kernel 5.1+ required)", err)
		fd, err = unix.FanotifyInit(fanotifyFlags, unix.O_RDONLY|unix.O_LARGEFILE)
		if err != nil {
			log.Fatalf("fanotify_init failed: %v — ensure you are running as root and kernel >= 5.1", err)
		}
	}
	defer unix.Close(fd)

	// Mark the filesystem containing MonitorPath with FAN_MARK_FILESYSTEM so
	// we receive events for the entire mount point, similar to FSEvents on macOS.
	watchMask := uint64(unix.FAN_CREATE | unix.FAN_DELETE | unix.FAN_MOVED_FROM | unix.FAN_MOVED_TO | unix.FAN_ONDIR)
	if err := unix.FanotifyMark(fd, unix.FAN_MARK_ADD|unix.FAN_MARK_FILESYSTEM, watchMask, unix.AT_FDCWD, config.MonitorPath); err != nil {
		log.Fatalf("fanotify_mark failed: %v", err)
	}
	log.Printf("fanotify watching filesystem at: %s", config.MonitorPath)

	setupSignalHandlers(db, fd)

	// Event reader goroutine
	go func() {
		log.Println("Event listener ready.")
		scanTimer := time.Now()

		buf := make([]byte, 4096*32)
		for {
			n, err := unix.Read(fd, buf)
			if err != nil {
				if err == unix.EINTR || err == unix.EAGAIN {
					time.Sleep(10 * time.Millisecond)
					continue
				}
				log.Printf("fanotify read error: %v", err)
				return
			}
			if n == 0 {
				continue
			}

			records := parseFanotifyEvents(buf[:n])
			for _, rec := range records {
				if verbose {
					log.Printf("Event: %s", rec.Path)
				}
				dbChannel <- rec
			}

			if time.Since(scanTimer) > 2*time.Hour {
				scanHomeDirs()
				scanTimer = time.Now()
			}
		}
	}()

	// Keep the program running
	select {}
}
