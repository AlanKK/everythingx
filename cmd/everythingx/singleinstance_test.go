package main

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAcquireInstanceSecondLaunchSignalsPrimary(t *testing.T) {
	dir := t.TempDir()

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
	dir := t.TempDir()
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
	dir := t.TempDir()

	ln, primary, err := acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	require.NoError(t, ln.Close())

	ln, primary, err = acquireInstance(dir)
	require.NoError(t, err)
	require.True(t, primary)
	ln.Close()
}
