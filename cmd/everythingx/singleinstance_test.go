package main

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// shortTempDir is a temp dir with a short path. t.TempDir includes the test
// name, and under macOS's /var/folders temp dir the socket path then runs past
// the 104-byte limit for Unix socket paths, so bind fails.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "evx")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestAcquireInstanceSecondLaunchSignalsPrimary(t *testing.T) {
	dir := shortTempDir(t)

	ln, primary, err := acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	defer ln.Close()

	shown := make(chan struct{}, 1)
	serveInstance(ln, func() { shown <- struct{}{} })

	ln2, primary2, err := acquireInstance(dir)
	require.NoError(t, err)
	require.False(t, primary2)
	require.Nil(t, ln2)

	select {
	case <-shown:
	case <-time.After(2 * time.Second):
		t.Fatal("primary instance did not receive show request")
	}
}

func TestAcquireInstanceReplacesStaleSocket(t *testing.T) {
	dir := shortTempDir(t)
	sock, _ := instancePaths(dir)

	stale, err := net.Listen("unix", sock)
	require.NoError(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	_, err = os.Stat(sock)
	require.NoError(t, err, "stale socket file should remain")

	ln, primary, err := acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	ln.Close()
}

func TestAcquireInstanceAfterPrimaryExits(t *testing.T) {
	dir := shortTempDir(t)

	ln, primary, err := acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	require.NoError(t, ln.Close())

	ln, primary, err = acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	ln.Close()
}
