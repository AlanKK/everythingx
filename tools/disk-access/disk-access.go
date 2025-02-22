package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// TODO: need to ask for full disk access on macOS to get all files under /Users/username
func main() {
	hasAccess, err := HasFullDiskAccess("/usr/local/bin/everythingxd")
	if err != nil {
		fmt.Println("Error checking Full Disk Access:", err)
		os.Exit(1)
	}
	if !hasAccess {
		fmt.Println("Full Disk Access is not granted. Please enable it in System Settings > Privacy & Security > Full Disk Access.")
		os.Exit(1)
	}
}

// HasFullDiskAccess checks if the given binary has Full Disk Access permission
// in the TCC (Transparency, Consent, and Control) database.
//
// Parameters:
//
//	binaryPath: The full path to the binary to check (e.g., "/usr/local/bin/myapp")
//
// Returns:
//
//	bool: true if Full Disk Access is granted, false otherwise
//	error: any error encountered during the check
func HasFullDiskAccess(binaryPath string) (bool, error) {
	// Open connection to TCC database
	db, err := sql.Open("sqlite3", "/Library/Application Support/com.apple.TCC/TCC.db")
	if err != nil {
		return false, err
	}
	defer db.Close()

	// Query for Full Disk Access permission
	var authValue int
	err = db.QueryRow(`
		SELECT auth_value FROM access WHERE service = 'kTCCServiceSystemPolicyAllFiles' 
		AND client = ?`,
		binaryPath).Scan(&authValue)

	// Handle case where no record is found
	if err == sql.ErrNoRows {
		return false, nil
	}

	// Handle other potential errors
	if err != nil {
		return false, err
	}

	// Return true if auth_value is 2 (granted)
	return authValue == 2, nil
}
