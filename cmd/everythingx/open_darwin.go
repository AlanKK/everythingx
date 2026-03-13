//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

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
