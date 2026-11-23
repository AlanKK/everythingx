//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// revealLabel is the context-menu wording for platformReveal on this platform.
const revealLabel = "Reveal in Finder"

// platformOpen opens the item as a Finder double-click would: a file in its
// default app, a folder in Finder, an .app by launching it. When no app is
// registered for the file, "open" exits non-zero without offering a chooser.
func platformOpen(pathname string) error {
	return runOpen(pathname)
}

// platformReveal shows the item selected in its Finder window.
func platformReveal(pathname string) error {
	return runOpen("-R", pathname)
}

func runOpen(args ...string) error {
	out, err := exec.Command("open", args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("open %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return fmt.Errorf("open %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
