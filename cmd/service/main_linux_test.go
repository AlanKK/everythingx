//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestShouldIgnorePath(t *testing.T) {
	tests := []struct {
		path   string
		ignore bool
	}{
		{"/proc/1/status", true},
		{"/proc", true},
		{"/sys/kernel/debug", true},
		{"/run/systemd", true},
		{"/dev/null", true},
		{"/snap/firefox/current", true},
		{"/home/alan/file.txt", false},
		{"/usr/local/bin/ev", false},
		{"/var/lib/everythingx/files.db", false},
		{"/procedure/readme.txt", false}, // must not match /proc prefix loosely
		{"/system/file", false},          // must not match /sys prefix loosely
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := shouldIgnorePath(tt.path)
			assert.Equal(t, tt.ignore, result, "shouldIgnorePath(%q)", tt.path)
		})
	}
}

// TestParseFanotifyEventsOverflow verifies that a FAN_Q_OVERFLOW event is
// reported via the overflowed return value instead of being silently
// dropped, since the kernel emits it whenever it had to discard events
// because our fanotify queue fell behind.
func TestParseFanotifyEventsOverflow(t *testing.T) {
	event := fanotifyEventMetadata{
		EventLen:    sizeofEventMetadata,
		Vers:        fanotifyMetadataVersion,
		MetadataLen: uint16(sizeofEventMetadata),
		Mask:        unix.FAN_Q_OVERFLOW,
		Fd:          -1,
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&event)), int(unsafe.Sizeof(event)))

	records, overflowed := parseFanotifyEvents(buf, nil)

	assert.True(t, overflowed, "expected overflowed=true for a FAN_Q_OVERFLOW event")
	assert.Empty(t, records, "an overflow marker alone should not produce file records")
}

// TestParseDFIDNamePathNameNotPaddedBeforeFilename guards against a real bug
// found in production: the kernel places the filename immediately after the
// raw file handle bytes with no padding (only the *end* of the whole info
// record is padded to 8-byte alignment). The previous code rounded up to an
// 8-byte boundary before the filename too, which silently ate the first N
// bytes of every filename whenever handle_bytes was not itself a multiple of
// 8 (observed on ZFS, handle_bytes=12: exactly 4 leading characters of every
// filename were lost, corrupting every FAN_CREATE/DELETE/MOVE path).
func TestParseDFIDNamePathNameNotPaddedBeforeFilename(t *testing.T) {
	const handleBytes = 12 // not a multiple of 8 — reproduces the observed bug
	const wantName = "abc.txt"
	fsid := [2]int32{111, 222}

	// header(4) + fsid(8) + handle_bytes(4) + handle_type(4) + handle(handleBytes) + name + NUL
	info := make([]byte, 4+8+8+handleBytes+len(wantName)+1)
	binary.NativeEndian.PutUint32(info[4:], uint32(fsid[0]))
	binary.NativeEndian.PutUint32(info[8:], uint32(fsid[1]))
	binary.NativeEndian.PutUint32(info[12:], handleBytes)
	binary.NativeEndian.PutUint32(info[16:], 0x7fffffff) // bogus handle_type, OpenByHandleAt must fail
	copy(info[20+handleBytes:], wantName)

	dirFD, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY, 0)
	if err != nil {
		t.Fatalf("could not open / for test fixture: %v", err)
	}
	defer unix.Close(dirFD)
	mountFDMap := map[[2]int32]int{fsid: dirFD}

	var logBuf bytes.Buffer
	oldOutput := log.Writer()
	oldVerbose := verbose
	log.SetOutput(&logBuf)
	verbose = true
	defer func() {
		log.SetOutput(oldOutput)
		verbose = oldVerbose
	}()

	got := parseDFIDNamePath(info, mountFDMap)

	// OpenByHandleAt is expected to fail (the handle is bogus), so the
	// resolved path is always "" — what we're really checking is the *name*
	// it logged along the way, which must be the full, untruncated filename.
	assert.Empty(t, got)
	assert.Contains(t, logBuf.String(), `name="`+wantName+`"`,
		"logged name must be the untruncated filename, not missing its leading bytes")
}
