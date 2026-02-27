package main

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/AlanKK/everythingx/internal/mmindex"
	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"
	flags "github.com/jessevdk/go-flags"
)

// Options struct to hold the command-line options
type Options struct {
	Args struct {
		SearchTerm string `positional-arg-name:"searchTerm" description:"Search term, full or partial filename"`
	} `positional-args:"yes"`
	Version   bool `long:"version" description:"Show version information"`
	Highlight bool `short:"b" long:"highlight" description:"Highlight (bold) search term in results for readability"`
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default|flags.PrintErrors|flags.HelpFlag)

	_, err := parser.Parse()
	if err != nil {
		parser.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	if opts.Version {
		fmt.Println(version.Info())
		os.Exit(0)
	}

	if opts.Args.SearchTerm == "" {
		fmt.Println("Search term is required.")
		parser.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	idx, err := mmindex.Open()
	if err != nil {
		fmt.Println("Error opening index:", err)
		os.Exit(1)
	}
	defer idx.Close()

	results := idx.Search(opts.Args.SearchTerm, math.MaxInt)

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
}
