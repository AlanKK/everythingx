package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
)

// TODO: need to ask for full disk access on macOS to get all files under /Users/username
func main() {
	dbFile, searchTerm, verbose := getCommandLineArgs()

	if !shared.FileExists(dbFile) {
		fmt.Println("Database does not exist ", dbFile)
		os.Exit(1)
	}

	if verbose {
		fmt.Println("Opening database ", dbFile)
	}
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
		// for _, r := range results {
		// 	before, term, after := shared.splitFileName(r.Fullpath, searchTerm)
		// 	fmt.Printf("%s\033[1m%s\033[0m%s\n", before, term, after)
		// }
		for _, r := range results {
			fmt.Println(r.Fullpath)
		}
		os.Exit(0)
	}

	loadUI()
}

func getCommandLineArgs() (string, string, bool) {
	dbFile := flag.String("db_path", "/var/lib/everythingx/files.db", "Path to the database file")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	searchTerm := flag.String("name", "", "Search term")
	flag.Parse()

	if *dbFile == "" {
		fmt.Println("Database path is required")
		os.Exit(1)
	}
	return *dbFile, *searchTerm, *verbose
}
