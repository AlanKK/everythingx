//go:build linux

package main

import (
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
