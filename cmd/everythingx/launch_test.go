package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileURI(t *testing.T) {
	tests := []struct {
		name, path, want string
	}{
		{"plain", "/home/user/video.mkv", "file:///home/user/video.mkv"},
		{"unreserved kept", "/a-b_c.d~e/F9", "file:///a-b_c.d~e/F9"},
		{"space", "/tmp/my file.txt", "file:///tmp/my%20file.txt"},
		{"hash", "/tmp/track #1.mp3", "file:///tmp/track%20%231.mp3"},
		{"question mark", "/tmp/why?.txt", "file:///tmp/why%3F.txt"},
		{"literal percent", "/tmp/100%.txt", "file:///tmp/100%25.txt"},
		{"utf-8", "/tmp/café", "file:///tmp/caf%C3%A9"},
		{"invalid utf-8", "/tmp/bad\xff\xfe", "file:///tmp/bad%FF%FE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fileURI(tc.path)
			if got != tc.want {
				t.Errorf("fileURI(%q) = %q, want %q", tc.path, got, tc.want)
			}
			for i := 0; i < len(got); i++ {
				if got[i] >= 0x80 {
					t.Errorf("fileURI(%q) has non-ASCII byte %#x at %d", tc.path, got[i], i)
				}
			}
		})
	}
}

// launchCall records which stubbed launcher ran, and on what.
type launchCall struct {
	action, path string
}

// stubLaunchers replaces openPath and revealPath with stubs that report on the
// returned channel, restoring the real ones when the test ends.
func stubLaunchers(t *testing.T) <-chan launchCall {
	t.Helper()
	calls := make(chan launchCall, 4)
	origOpen, origReveal := openPath, revealPath
	openPath = func(p string) error { calls <- launchCall{"open", p}; return nil }
	revealPath = func(p string) error { calls <- launchCall{"reveal", p}; return nil }
	t.Cleanup(func() { openPath, revealPath = origOpen, origReveal })
	return calls
}

// expectLaunch waits for the next launcher call; launch runs it on a goroutine.
func expectLaunch(t *testing.T, calls <-chan launchCall, action, path string) {
	t.Helper()
	select {
	case got := <-calls:
		if got.action != action || got.path != path {
			t.Errorf("launched %s %q, want %s %q", got.action, got.path, action, path)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no launch, want %s %q", action, path)
	}
}

// expectNoLaunch checks that no launcher runs within a short wait.
func expectNoLaunch(t *testing.T, calls <-chan launchCall) {
	t.Helper()
	select {
	case got := <-calls:
		t.Errorf("unexpected launch: %s %q", got.action, got.path)
	case <-time.After(200 * time.Millisecond):
	}
}

// tempFile creates an empty file, since launch skips items that no longer exist.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The index can lag the disk. A result deleted since it was indexed must not be
// handed to the launcher.
func TestLaunchSkipsMissingFile(t *testing.T) {
	calls := stubLaunchers(t)
	openFile(filepath.Join(t.TempDir(), "gone.mkv"))
	expectNoLaunch(t, calls)
}

func TestLaunchIgnoresEmptyPath(t *testing.T) {
	calls := stubLaunchers(t)
	openFile("")
	revealFile("")
	expectNoLaunch(t, calls)
}
