//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// revealLabel is the context-menu wording for handleOpenFile on this platform.
const revealLabel = "Open Containing Folder"

// handleOpenFile opens the directory containing the given path using xdg-open,
// which launches the system's default file manager.
func handleOpenFile(pathname string) {
	if pathname == "" {
		return
	}
	dir := filepath.Dir(pathname)
	cmd := exec.Command("xdg-open", dir)
	if err := cmd.Run(); err != nil {
		fmt.Println("Error:", err)
	}
}
