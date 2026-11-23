//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// platformAppsFor lists the apps for the item's MIME type and its supertypes,
// read from the same freedesktop databases the file managers use.
func platformAppsFor(pathname string) ([]appChoice, error) {
	db := currentMimeDB()
	chain := db.typeChain(fileMimeType(db, pathname))
	entries, def := xdgAppsFor(db, chain)
	apps := make([]appChoice, 0, len(entries))
	for _, e := range entries {
		apps = append(apps, appChoice{
			Name:    e.Name,
			ID:      e.ID,
			Detail:  strings.TrimSuffix(e.ID, ".desktop"),
			Default: e == def,
		})
	}
	return apps, nil
}

// platformOpenWith runs the app's Exec line on the item. The app is started
// in its own session with stdio at /dev/null, for the reasons given at
// runCommand, and is not waited on beyond reaping it.
func platformOpenWith(app appChoice, pathname string) error {
	idx := scanApplications(applicationDirs())
	e := idx.entry(app.ID)
	if e == nil {
		return fmt.Errorf("%s: application not found", app.ID)
	}
	args, err := execArgs(e, pathname)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = e.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", app.ID, err)
	}
	go cmd.Wait()
	return nil
}

// platformChooseApp shows the desktop portal's app chooser, the GNOME or KDE
// "Open With" dialog.
func platformChooseApp(pathname string) error {
	return portalOpenFile(pathname)
}
