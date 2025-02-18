package main

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/AlanKK/findfiles/internal/ffdb"
	"github.com/AlanKK/findfiles/internal/shared"
	flags "github.com/jessevdk/go-flags"
)

// Options struct to hold the command-line options
type Options struct {
	DBPath     string `short:"d" long:"db_path" description:"Path to the database file" default:"/var/lib/findfiles/files.db"`
	SearchTerm string `short:"n" long:"name" description:"Search term, full or partial filename"`
	Verbose    bool   `short:"v" long:"verbose" description:"Enable verbose logging"`
	Highlight  bool   `short:"b" long:"highlight" description:"Highlight (bold) search term in results for readability"`
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)

	_, err := parser.Parse()
	if err != nil {
		fmt.Println("Error parsing flags: ", err)
		os.Exit(1)
	}

	if opts.SearchTerm == "" {
		fmt.Println("Search term is required.")
		os.Exit(1)
	}

	if !shared.FileExists(opts.DBPath) {
		fmt.Println("Database does not exist ", opts.DBPath)
		os.Exit(1)
	}

	if opts.Verbose {
		fmt.Println("Opening database ", opts.DBPath)
	}
	db, err := ffdb.OpenDBReadOnly(opts.DBPath)
	if err != nil {
		fmt.Println("Error opening database: ", opts.DBPath, err)
		os.Exit(1)
	}
	defer db.Close()

	if opts.SearchTerm != "" {
		results, err := ffdb.PrefixSearch(opts.SearchTerm, math.MaxInt)
		if err != nil {
			fmt.Println("Error searching for ", opts.SearchTerm, err)
			os.Exit(1)
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Fullpath < results[j].Fullpath
		})

		if opts.Highlight {
			for _, r := range results {
				before, term, after := shared.SplitFileName(r.Fullpath, opts.SearchTerm)
				fmt.Printf("%s\033[1m%s\033[0m%s\n", before, term, after)
			}
		} else {
			for _, r := range results {
				fmt.Println(r.Fullpath)
			}
		}
		os.Exit(0)
	}
}
