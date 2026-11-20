//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unsafe"

	"github.com/AlanKK/everythingx/internal/shared"
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

	records, movedInDirs, overflowed := parseFanotifyEvents(buf, nil)

	assert.True(t, overflowed, "expected overflowed=true for a FAN_Q_OVERFLOW event")
	assert.Empty(t, records, "an overflow marker alone should not produce file records")
	assert.Empty(t, movedInDirs, "an overflow marker alone should not report moved folders")
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

// collectWalk runs indexMovedDir against a test walkChannel and returns every
// record it sent, along with the size of each chunk.
func collectWalk(t *testing.T, root string) ([]shared.EventRecord, []int) {
	t.Helper()
	originalWalkChannel := walkChannel
	walkChannel = make(chan []shared.EventRecord, 1000)
	defer func() { walkChannel = originalWalkChannel }()

	indexMovedDir(root)
	close(walkChannel)

	var recs []shared.EventRecord
	var sizes []int
	for chunk := range walkChannel {
		recs = append(recs, chunk...)
		sizes = append(sizes, len(chunk))
	}
	return recs, sizes
}

// TestIndexMovedDirSendsContents covers the folder-rename fix: fanotify only
// reports the renamed folder itself, and the old path is deleted from the
// index together with everything under it, so every entry below the new path
// must be sent to the database writer again.
func TestIndexMovedDirSendsContents(t *testing.T) {
	root := filepath.Join(t.TempDir(), "renamed")
	assert.NoError(t, os.MkdirAll(filepath.Join(root, "sub/deeper"), 0o755))
	for _, f := range []string{"a.zip", "sub/b.zip", "sub/deeper/c.zip"} {
		assert.NoError(t, os.WriteFile(filepath.Join(root, f), nil, 0o644))
	}
	assert.NoError(t, os.Symlink("a.zip", filepath.Join(root, "link")))

	recs, _ := collectWalk(t, root)

	var got []string
	types := map[string]shared.ObjectType{}
	for _, rec := range recs {
		rel, err := filepath.Rel(root, rec.Path)
		assert.NoError(t, err)
		got = append(got, rel)
		types[rel] = rec.ObjectType
		assert.Equal(t, filepath.Base(rec.Path), rec.Filename)
		assert.False(t, rec.FoundOnScan, "records must be re-checked for existence at flush time")
	}
	sort.Strings(got)

	// The folder itself is already recorded by its FAN_MOVED_TO event.
	assert.Equal(t, []string{"a.zip", "link", "sub", "sub/b.zip", "sub/deeper", "sub/deeper/c.zip"}, got)
	assert.Equal(t, shared.ItemIsDir, types["sub/deeper"])
	assert.Equal(t, shared.ItemIsFile, types["sub/deeper/c.zip"])
	assert.Equal(t, shared.ItemIsSymlink, types["link"])
}

// TestIndexMovedDirSendsChunks checks that a large walk is sent in chunks of
// movedDirChunkSize, so the writer commits once per chunk even with --nocache.
func TestIndexMovedDirSendsChunks(t *testing.T) {
	root := t.TempDir()
	total := movedDirChunkSize*2 + 7
	for i := 0; i < total; i++ {
		assert.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("f%05d", i)), nil, 0o644))
	}

	recs, sizes := collectWalk(t, root)

	assert.Len(t, recs, total)
	assert.Equal(t, []int{movedDirChunkSize, movedDirChunkSize, 7}, sizes)
}

// TestIndexMovedDirMissingFolder checks that a folder renamed again before it
// could be walked is skipped quietly; the later rename event covers it.
func TestIndexMovedDirMissingFolder(t *testing.T) {
	recs, sizes := collectWalk(t, filepath.Join(t.TempDir(), "gone"))
	assert.Empty(t, recs)
	assert.Empty(t, sizes)
}

// TestQueueMovedDirBurstDoesNotRescan covers a burst of folder renames far
// larger than the old 4096-entry queue: nothing blocks, nothing triggers a
// rescan, duplicates collapse, and folders come out in arrival order.
func TestQueueMovedDirBurstDoesNotRescan(t *testing.T) {
	originalSet, originalRescan := movedDirs, requestRescan
	defer func() { movedDirs, requestRescan = originalSet, originalRescan }()
	movedDirs = newMovedDirSet()
	rescans := 0
	requestRescan = func(string) { rescans++ }

	const n = 20000
	for i := 0; i < n; i++ {
		queueMovedDir(fmt.Sprintf("/data/dir%d", i))
	}
	queueMovedDir("/data/dir0")

	assert.Equal(t, 0, rescans)
	assert.Equal(t, n, movedDirs.len())
	first, ok := movedDirs.next()
	assert.True(t, ok)
	assert.Equal(t, "/data/dir0", first)
	assert.Equal(t, n-1, movedDirs.len())
}

// TestQueueMovedDirOverflowRescansOnce checks that overflowing the pending set
// clears it and requests exactly one rescan, not one per dropped folder.
func TestQueueMovedDirOverflowRescansOnce(t *testing.T) {
	originalSet, originalRescan := movedDirs, requestRescan
	defer func() { movedDirs, requestRescan = originalSet, originalRescan }()
	movedDirs = newMovedDirSet()
	rescans := 0
	requestRescan = func(string) { rescans++ }

	for i := 0; i < maxPendingMovedDirs+10; i++ {
		queueMovedDir(fmt.Sprintf("/data/dir%d", i))
	}

	assert.Equal(t, 1, rescans)
	assert.Equal(t, 9, movedDirs.len(), "folders after the overflow are queued again")
}
