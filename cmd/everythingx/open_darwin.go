//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

// revealLabel is the context-menu wording for handleOpenFile on this platform.
const revealLabel = "Reveal in Finder"

// handleOpenFile reveals the given path in Finder using "open -R".
func handleOpenFile(pathname string) {
	if pathname == "" {
		return
	}
	cmd := exec.Command("open", "-R", pathname)
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}
