//go:build linux

package main

import (
	"encoding/binary"
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

// dfidNameRecord builds a synthetic DFID_NAME info record for the given handle
// size and filename, laid out the way the kernel writes one.
func dfidNameRecord(fsid [2]int32, handleType int32, handleBytes int, name string) []byte {
	// header(4) + fsid(8) + handle_bytes(4) + handle_type(4) + handle + name + NUL
	info := make([]byte, 4+8+8+handleBytes+len(name)+1)
	binary.NativeEndian.PutUint32(info[4:], uint32(fsid[0]))
	binary.NativeEndian.PutUint32(info[8:], uint32(fsid[1]))
	binary.NativeEndian.PutUint32(info[12:], uint32(handleBytes))
	binary.NativeEndian.PutUint32(info[16:], uint32(handleType))
	copy(info[20+handleBytes:], name)
	return info
}

// TestParseDFIDNameInfoNameNotPaddedBeforeFilename guards against a real bug
// found in production: the kernel places the filename immediately after the
// raw file handle bytes with no padding (only the *end* of the whole info
// record is padded to FANOTIFY_EVENT_ALIGN). The previous code rounded up to an
// 8-byte boundary before the filename too, which silently ate the first N
// bytes of every filename whenever handle_bytes was not itself a multiple of
// 8 (observed on ZFS, handle_bytes=12: exactly 4 leading characters of every
// filename were lost, corrupting every FAN_CREATE/DELETE/MOVE path).
func TestParseDFIDNameInfoNameNotPaddedBeforeFilename(t *testing.T) {
	t.Parallel()

	fsid := [2]int32{111, 222}
	// 12 is not a multiple of 8, which is what exposes the bug; 8 and 16 are
	// the sizes that hid it, so parse correctly either way.
	for _, handleBytes := range []int{8, 12, 16, 20} {
		info := dfidNameRecord(fsid, 1, handleBytes, "abc.txt")

		parsed, ok := parseDFIDNameInfo(info)

		assert.True(t, ok, "handle_bytes=%d", handleBytes)
		assert.Equal(t, "abc.txt", parsed.name,
			"handle_bytes=%d: name must not lose its leading bytes", handleBytes)
		assert.Equal(t, fsid, parsed.fsid, "handle_bytes=%d", handleBytes)
		assert.Len(t, parsed.handleData, handleBytes)
	}
}

func TestParseDFIDNameInfoRejectsTruncatedRecord(t *testing.T) {
	t.Parallel()

	// handle_bytes claims 32 but the record only carries 12.
	info := dfidNameRecord([2]int32{1, 2}, 1, 12, "abc.txt")
	binary.NativeEndian.PutUint32(info[12:], 32)

	_, ok := parseDFIDNameInfo(info)

	assert.False(t, ok, "a record whose handle runs past its end must be rejected")
}
