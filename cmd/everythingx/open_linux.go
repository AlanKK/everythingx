//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/rymdport/portal/openuri"
)

// revealLabel is the context-menu wording for platformReveal on this platform.
const revealLabel = "Open Containing Folder"

// showItemsTimeout bounds the FileManager1 call. D-Bus activation may have to
// cold-start the file manager, which can take a few seconds.
const showItemsTimeout = 10 * time.Second

// platformOpen opens the item in its default application, or a folder in the
// file manager. When xdg-open fails on a file, usually because no application
// is registered for its type, the desktop portal is asked to show its app
// chooser instead.
func platformOpen(pathname string) error {
	err := runCommand("xdg-open", pathname)
	if err == nil {
		return nil
	}
	if info, statErr := os.Stat(pathname); statErr == nil && info.IsDir() {
		return err
	}
	if portalErr := portalOpenFile(pathname); portalErr != nil {
		return fmt.Errorf("%w; portal: %v", err, portalErr)
	}
	return nil
}

// portalOpenFile hands the file to the XDG desktop portal with ask set, which
// on GNOME and KDE shows the "open with" chooser.
func portalOpenFile(pathname string) error {
	f, err := os.Open(pathname)
	if err != nil {
		return err
	}
	defer f.Close()
	return openuri.OpenFile("", f.Fd(), &openuri.Options{Ask: true})
}

// platformReveal opens the containing folder with the item selected, through
// org.freedesktop.FileManager1, which Nautilus, Dolphin, Nemo, Caja and Thunar
// implement. Without it, the parent folder is opened with nothing selected.
func platformReveal(pathname string) error {
	err := showItems(pathname)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		// The file manager is slow to start and will most likely still show
		// the item. Opening the folder as well would give a second window,
		// and reporting a failure would be wrong once it appears.
		fmt.Println("FileManager1 ShowItems timed out:", err)
		return nil
	}
	if !fileManagerUnavailable(err) {
		return err
	}
	fmt.Println("FileManager1 unavailable, opening the folder instead:", err)
	return runCommand("xdg-open", filepath.Dir(pathname))
}

// errNoSessionBus marks a failure to reach the session bus at all, which is a
// definite "no file manager service" for fallback purposes.
var errNoSessionBus = errors.New("no D-Bus session bus")

func showItems(pathname string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("%w: %v", errNoSessionBus, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), showItemsTimeout)
	defer cancel()
	obj := conn.Object("org.freedesktop.FileManager1", "/org/freedesktop/FileManager1")
	call := obj.CallWithContext(ctx, "org.freedesktop.FileManager1.ShowItems", 0,
		[]string{fileURI(pathname)}, "")
	return call.Err
}

// fileManagerUnavailable reports whether err shows that no FileManager1
// service exists, as opposed to one that is slow or failed in passing.
func fileManagerUnavailable(err error) bool {
	if errors.Is(err, errNoSessionBus) {
		return true
	}
	var name string
	var dbusErr dbus.Error
	var dbusErrPtr *dbus.Error
	switch {
	case errors.As(err, &dbusErr):
		name = dbusErr.Name
	case errors.As(err, &dbusErrPtr):
		name = dbusErrPtr.Name
	}
	switch name {
	case "org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.UnknownMethod",
		"org.freedesktop.DBus.Error.UnknownInterface",
		"org.freedesktop.DBus.Error.UnknownObject":
		return true
	}
	return false
}

// runCommand runs a launcher and reports only its exit status. Stdio is left
// at /dev/null on purpose: xdg-open may start the application as its own child,
// which inherits these descriptors. A pipe there would hold Wait until the app
// exits, and the app could take SIGPIPE writing to it after EverythingX quits.
func runCommand(name string, args ...string) error {
	if err := exec.Command(name, args...).Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
