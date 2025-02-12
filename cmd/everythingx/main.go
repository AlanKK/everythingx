package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlanKK/findfiles/internal/ffdb"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func main() {
	dbFile := getCommandLineArgs()

	if !fileExists(dbFile) {
		fmt.Println("Database does not exist ", dbFile)
		os.Exit(1)
	}

	fmt.Println("Opening database ", dbFile)
	_, err := ffdb.OpenDBReadOnly(dbFile)
	if err != nil {
		fmt.Println("Error opening database: ", dbFile, err)
		os.Exit(1)
	}

	loadUI()
}

func getCommandLineArgs() string {
	dbFile := flag.String("db_path", "/var/lib/findfiles/files.db", "Path to the database file")
	flag.Parse()

	if *dbFile == "" {
		fmt.Println("Database path is required")
		os.Exit(1)
	}
	return *dbFile
}
