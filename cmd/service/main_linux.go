//go:build linux

package main

import (
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

func reportPermissionError(path string) {
	log.Printf("Permission denied scanning %s", path)
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
// mountFDMap maps filesystem IDs (fsid) to O_PATH file descriptors, used by
// parseDFIDNamePath to resolve parent directory file handles via open_by_handle_at.
func parseFanotifyEvents(buf []byte, mountFDMap map[[2]int32]int) []*shared.EventRecord {
	var records []*shared.EventRecord
	n := len(buf)

	for offset := 0; offset+int(sizeofEventMetadata) <= n; {
		eventLen := binary.NativeEndian.Uint32(buf[offset:])
		vers := buf[offset+4]
		mask := binary.NativeEndian.Uint64(buf[offset+8:])
		fd := int32(binary.NativeEndian.Uint32(buf[offset+16:]))

		if vers != fanotifyMetadataVersion {
			break
		}

		eventEnd := offset + int(eventLen)
		if eventEnd > n {
			break
		}

		extraStart := offset + int(sizeofEventMetadata)
		offset = eventEnd // advance past this event

		var path string
		if fd >= 0 {
			path = resolvePathFromFD(fd)
			unix.Close(int(fd))
		} else if extraStart < eventEnd {
			// Slice directly from buf — no copy needed
			path = parseDFIDNamePath(buf[extraStart:eventEnd], mountFDMap)
		}

		if path == "" || shouldIgnorePath(path) {
			continue
		}

		isCreate := mask&unix.FAN_CREATE != 0 || mask&unix.FAN_MOVED_TO != 0
		isDelete := mask&unix.FAN_DELETE != 0 || mask&unix.FAN_MOVED_FROM != 0

		if !isCreate && !isDelete {
			continue
		}

		objType := objectTypeFromStat(path)
		// objectTypeFromStat returns ItemIsFile when the item is already gone.
		// Use the FAN_ONDIR flag to correctly identify directory delete events.
		if isDelete && mask&unix.FAN_ONDIR != 0 {
			objType = shared.ItemIsDir
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

// parseDFIDNamePath resolves a full path from a DFID_NAME info record.
// With FAN_REPORT_DFID_NAME the kernel provides a file handle for the parent
// directory plus the child filename. We look up the correct mountFD from the
// event's fsid, open the parent dir via open_by_handle_at, read its path from
// /proc/self/fd, then join with the filename.
//
// Info record layout (after fanotify_event_metadata):
//
//	header:       4 bytes  (info_type, pad, len)
//	fsid:         8 bytes  (two int32s)
//	handle_bytes: 4 bytes
//	handle_type:  4 bytes  (int32)
//	f_handle:     handle_bytes bytes
//	[padding to 8-byte alignment]
//	filename:     null-terminated string
func parseDFIDNamePath(info []byte, mountFDMap map[[2]int32]int) string {
	const headerSize = 4                            // info_type(1) + pad(1) + len(2)
	const fsidSize = 8                              // __kernel_fsid_t = {int val[2]}
	const handleHeaderSize = 8                      // handle_bytes(4) + handle_type(4)
	const headerAndFsidSize = headerSize + fsidSize // = 12
	if len(info) < headerAndFsidSize+handleHeaderSize+1 {
		return ""
	}

	// Read fsid to look up the mountFD for this filesystem.
	fsid := [2]int32{
		int32(binary.NativeEndian.Uint32(info[headerSize:])),
		int32(binary.NativeEndian.Uint32(info[headerSize+4:])),
	}
	mountFD, ok := mountFDMap[fsid]
	if !ok {
		log.Printf("parseDFIDNamePath: unknown fsid=%v (filesystem not monitored)", fsid)
		return ""
	}

	offset := headerAndFsidSize
	handleBytes := binary.NativeEndian.Uint32(info[offset:])
	handleType := int32(binary.NativeEndian.Uint32(info[offset+4:]))
	offset += handleHeaderSize

	if offset+int(handleBytes) > len(info) {
		return ""
	}
	handleData := info[offset : offset+int(handleBytes)]

	// The kernel pads sizeof(file_handle)+handleBytes to 8-byte alignment
	// before placing the filename.
	nameOffset := headerAndFsidSize + ((handleHeaderSize + int(handleBytes) + 7) &^ 7)
	if nameOffset >= len(info) {
		return ""
	}
	name := unix.ByteSliceToString(info[nameOffset:])
	if name == "" || name == "." {
		return ""
	}

	// Open the parent directory from its file handle.
	dirFD, err := unix.OpenByHandleAt(mountFD, unix.NewFileHandle(handleType, handleData), unix.O_PATH)
	if err != nil {
		// ESTALE is expected: the parent directory was deleted between the event
		// and our resolution attempt (e.g. short-lived temp dirs). Drop silently.
		if err != unix.ESTALE && verbose {
			log.Printf("OpenByHandleAt failed (fsid=%v, handle_bytes=%d, name=%q): %v", fsid, handleBytes, name, err)
		}
		return ""
	}
	defer unix.Close(dirFD)

	dirPath, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", dirFD))
	if err != nil {
		return ""
	}

	return filepath.Join(dirPath, name)
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

// skipFsTypes lists virtual and pseudo filesystem types that should not be monitored.
var skipFsTypes = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true,
	"securityfs": true, "cgroup": true, "cgroup2": true, "pstore": true,
	"bpf": true, "tracefs": true, "debugfs": true, "hugetlbfs": true,
	"mqueue": true, "fusectl": true, "nsfs": true, "autofs": true,
	"efivarfs": true, "squashfs": true, "overlay": true, "tmpfs": true,
}

// buildMountFDMap reads /proc/mounts and for each real (non-virtual, non-ignored)
// filesystem: marks it for fanotify monitoring and opens an O_PATH fd on it.
// Returns a map of fsid -> fd. Caller must close all returned fds.
func buildMountFDMap(fanotifyFD int, watchMask uint64) map[[2]int32]int {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		log.Printf("Warning: could not read /proc/mounts: %v", err)
		return nil
	}

	fdMap := make(map[[2]int32]int)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mountPath := fields[1]
		fsType := fields[2]

		if skipFsTypes[fsType] || shouldIgnorePath(mountPath) {
			continue
		}

		var stat unix.Statfs_t
		if err := unix.Statfs(mountPath, &stat); err != nil {
			continue
		}
		fsid := [2]int32{stat.Fsid.Val[0], stat.Fsid.Val[1]}

		if _, exists := fdMap[fsid]; exists {
			continue // already watching this filesystem
		}

		mountfd, err := unix.Open(mountPath, unix.O_RDONLY|unix.O_DIRECTORY, 0)
		if err != nil {
			log.Printf("Warning: could not open mount point %s: %v", mountPath, err)
			continue
		}

		if err := unix.FanotifyMark(fanotifyFD, unix.FAN_MARK_ADD|unix.FAN_MARK_FILESYSTEM, watchMask, unix.AT_FDCWD, mountPath); err != nil {
			log.Printf("Warning: fanotify_mark failed for %s: %v", mountPath, err)
			unix.Close(mountfd)
			continue
		}

		fdMap[fsid] = mountfd
		log.Printf("Monitoring filesystem: %s (type=%s)", mountPath, fsType)
	}

	return fdMap
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
	fanotifyFlags := uint(unix.FAN_CLASS_NOTIF)
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

	watchMask := uint64(unix.FAN_CREATE | unix.FAN_DELETE | unix.FAN_MOVED_FROM | unix.FAN_MOVED_TO | unix.FAN_ONDIR)
	mountFDMap := buildMountFDMap(fd, watchMask)
	if len(mountFDMap) == 0 {
		log.Fatalf("no filesystems could be monitored — ensure running as root")
	}
	defer func() {
		for _, mfd := range mountFDMap {
			unix.Close(mfd)
		}
	}()

	setupSignalHandlers(db, fd)

	// Event reader goroutine — uses blocking reads (no FAN_NONBLOCK),
	// so the goroutine parks with zero CPU until events arrive.
	go func() {
		log.Println("Event listener ready.")
		buf := make([]byte, 4096*32)
		for {
			n, err := unix.Read(fd, buf)
			if err != nil {
				if err == unix.EINTR || err == unix.EAGAIN {
					continue
				}
				log.Printf("fanotify read error: %v", err)
				return
			}
			if n == 0 {
				continue
			}
			if verbose {
				log.Printf("fanotify read: %d bytes", n)
			}

			records := parseFanotifyEvents(buf[:n], mountFDMap)
			if verbose && len(records) <= 5 {
				for _, rec := range records {
					action := "created"
					if !shared.FileExists(rec.Path) {
						action = "deleted"
					}
					log.Printf("Event: %s %s", action, rec.Path)
				}
			}
			for _, rec := range records {
				dbChannel <- rec
			}
		}
	}()

	// Periodic re-scan of home directories
	go func() {
		ticker := time.NewTicker(2 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			scanHomeDirs()
		}
	}()

	// Keep the program running
	select {}
}
