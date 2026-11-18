package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Single-instance support. The first GUI process listens on a per-user Unix
// socket. A later launch connects to it, asks the running instance to show its
// window, and exits, so binding the app to a "focus or launch" hotkey doesn't
// pile up windows and tray icons.

const showCommand = "show"

// instanceDir is where the socket and lock file live: $XDG_RUNTIME_DIR on
// Linux desktops (per-user, cleared at logout), otherwise the temp dir, which
// on macOS ($TMPDIR) is already per-user.
func instanceDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

func instancePaths(dir string) (sock, lock string) {
	base := fmt.Sprintf("everythingx-%d", os.Getuid())
	return filepath.Join(dir, base+".sock"), filepath.Join(dir, base+".lock")
}

// acquireInstance decides whether this process is the primary instance. When
// another instance is already running it is sent a show request and primary
// is false. Otherwise the returned listener receives requests from later
// launches. On error the caller should just start normally.
func acquireInstance(dir string) (ln net.Listener, primary bool, err error) {
	sock, lock := instancePaths(dir)

	// The lock covers the check-then-listen below, so two launches at the
	// same moment can't both become primary.
	lf, err := os.OpenFile(lock, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	defer lf.Close()
	if err := unix.Flock(int(lf.Fd()), unix.LOCK_EX); err != nil {
		return nil, false, err
	}
	defer unix.Flock(int(lf.Fd()), unix.LOCK_UN)

	if conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond); err == nil {
		defer conn.Close()
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := conn.Write([]byte(showCommand + "\n")); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}

	// Nobody answered, so any socket file left behind is from a crashed run.
	_ = os.Remove(sock)
	ln, err = net.Listen("unix", sock)
	if err != nil {
		return nil, false, err
	}
	if err := os.Chmod(sock, 0o600); err != nil {
		ln.Close()
		return nil, false, err
	}
	return ln, true, nil
}

// serveInstance calls onShow for every show request from a later launch. It
// returns once ln is closed.
func serveInstance(ln net.Listener, onShow func()) {
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				line, _ := bufio.NewReader(c).ReadString('\n')
				if strings.TrimSpace(line) == showCommand {
					onShow()
				}
			}(conn)
		}
	}()
}
