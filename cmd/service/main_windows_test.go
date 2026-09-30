//go:build windows

package main

import (
	"encoding/binary"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func TestParseFileChanges(t *testing.T) {
	encode := func(action uint32, name string, next uint32) []byte {
		units := utf16.Encode([]rune(name))
		data := make([]byte, 12+len(units)*2)
		binary.LittleEndian.PutUint32(data, next)
		binary.LittleEndian.PutUint32(data[4:], action)
		binary.LittleEndian.PutUint32(data[8:], uint32(len(units)*2))
		for i, unit := range units {
			binary.LittleEndian.PutUint16(data[12+i*2:], unit)
		}
		return data
	}
	first := encode(windows.FILE_ACTION_ADDED, `sub\résumé.txt`, 0)
	binary.LittleEndian.PutUint32(first, uint32(len(first)))
	data := append(first, encode(windows.FILE_ACTION_REMOVED, `old.txt`, 0)...)
	changes, err := parseFileChanges(`C:\root`, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].path != filepath.Join(`C:\root`, `sub\résumé.txt`) || changes[0].action != windows.FILE_ACTION_ADDED || changes[1].path != filepath.Join(`C:\root`, `old.txt`) {
		t.Fatalf("unexpected changes: %+v", changes)
	}
	binary.LittleEndian.PutUint32(data[8:], 3)
	if _, err := parseFileChanges(`C:\root`, data); err == nil {
		t.Fatal("expected invalid UTF-16 byte length to fail")
	}
}

func TestWindowsWatcherIgnoresOwnDatabase(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	config.DBPath = `C:\example\EverythingX\files.db`
	for _, path := range []string{config.DBPath, config.DBPath + "-wal", config.DBPath + "-shm", config.DBPath + "-journal"} {
		if !shouldIgnorePath(path) {
			t.Fatalf("database path was not ignored: %s", path)
		}
	}
	if shouldIgnorePath(`C:\example\EverythingX\notes.txt`) {
		t.Fatal("unrelated path was ignored")
	}
}
