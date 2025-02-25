package main

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	flags "github.com/jessevdk/go-flags"
)

// Options struct to hold the command-line options
type Options struct {
	Args struct {
		SearchTerm string `positional-arg-name:"searchTerm" description:"Search term, full or partial filename"`
	} `positional-args:"yes" required:"yes"`
	DBPath    string `short:"d" long:"db_path" description:"Path to the database file" default:"/var/lib/everythingx/files.db"`
	Verbose   bool   `short:"v" long:"verbose" description:"Enable verbose logging"`
	Highlight bool   `short:"b" long:"highlight" description:"Highlight (bold) search term in results for readability"`
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default|flags.PrintErrors|flags.HelpFlag)

	_, err := parser.Parse()
	if err != nil {
		parser.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	if opts.Args.SearchTerm == "" {
		fmt.Println("Search term is required.")
		parser.WriteHelp(os.Stdout)
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

	if opts.Args.SearchTerm != "" {
		results, err := ffdb.PrefixSearch(opts.Args.SearchTerm, math.MaxInt)
		if err != nil {
			fmt.Println("Error searching for ", opts.Args.SearchTerm, err)
			os.Exit(1)
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Fullpath < results[j].Fullpath
		})

		if opts.Highlight {
			for _, r := range results {
				before, term, after := shared.SplitFileName(r.Fullpath, opts.Args.SearchTerm)
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
