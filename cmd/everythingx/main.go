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
	dbFile, searchTerm := getCommandLineArgs()

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

	if searchTerm != "" {
		results, err := ffdb.PrefixSearch(searchTerm, 1000)
		if err != nil {
			fmt.Println("Error searching for ", searchTerm, err)
			os.Exit(1)
		}
		for _, r := range results {
			before, term, after := splitFileName(r.Fullpath, searchTerm)
			fmt.Printf("%s\033[1m%s\033[0m%s\n", before, term, after)
		}
		os.Exit(0)
	}

	loadUI()
}

func getCommandLineArgs() (string, string) {
	dbFile := flag.String("db_path", "/var/lib/findfiles/files.db", "Path to the database file")
	searchTerm := flag.String("name", "", "Search term")
	flag.Parse()

	if *dbFile == "" {
		fmt.Println("Database path is required")
		os.Exit(1)
	}
	return *dbFile, *searchTerm
}
