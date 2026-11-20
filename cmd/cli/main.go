package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
	"github.com/AlanKK/everythingx/internal/version"
	flags "github.com/jessevdk/go-flags"
)

// Options struct to hold the command-line options
type Options struct {
	Args struct {
		SearchTerm string `positional-arg-name:"searchTerm" description:"Search term: a full or partial filename, or a pattern where *, ? and [...] match the whole name (quote it: 'mypackage*.bb')"`
	} `positional-args:"yes"`
	DBPath     string `short:"d" long:"db_path" description:"Path to the database file" default:"/var/lib/everythingx/files.db"`
	Verbose    bool   `short:"v" long:"verbose" description:"Enable verbose logging"`
	Version    bool   `long:"version" description:"Show version information"`
	Highlight  bool   `short:"b" long:"highlight" description:"Highlight (bold) search term in results for readability"`
	UserHome   bool   `short:"u" long:"user" description:"Show only results in the current user's home directory"`
	Executable bool   `short:"x" long:"executable" description:"Show only executable files"`
}

// Keeps only results that are dir itself or inside it.
func filterUnderDir(results []*shared.SearchResult, dir string) []*shared.SearchResult {
	dir = strings.TrimRight(dir, "/")
	filtered := results[:0]
	for _, r := range results {
		if r.Fullpath == dir || strings.HasPrefix(r.Fullpath, dir+"/") {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Keeps only regular files (or symlinks to them) with any execute bit set.
func filterExecutable(results []*shared.SearchResult) []*shared.SearchResult {
	filtered := results[:0]
	for _, r := range results {
		if r.ObjectType != shared.ItemIsFile && r.ObjectType != shared.ItemIsSymlink {
			continue
		}
		if isExecutable(r.Fullpath) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Reports whether path (following symlinks) is a regular file with any execute bit set.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// shellExpansionHint explains extra positional arguments, which usually mean
// the shell expanded an unquoted pattern such as mypackage*.bb into the
// matching files in the current directory. It returns "" when there are none.
func shellExpansionHint(rest []string) string {
	if len(rest) == 0 {
		return ""
	}
	return fmt.Sprintf("hint: ignoring %d extra argument(s); quote wildcard patterns so the shell doesn't expand them: ev 'mypackage*.bb'", len(rest))
}

// highlightPath bolds the part of fullpath that matched term. A glob
// highlights its literal parts within the file name; a literal term
// highlights its first occurrence.
func highlightPath(fullpath, term string) string {
	const bold, reset = "\033[1m", "\033[0m"
	base := filepath.Base(fullpath)
	if spans := ffdb.HighlightSpans(base, term); spans != nil {
		dir := fullpath[:len(fullpath)-len(base)]
		var b strings.Builder
		b.WriteString(dir)
		for i, part := range ffdb.SplitBySpans(base, spans) {
			if i%2 == 1 {
				b.WriteString(bold + part + reset)
			} else {
				b.WriteString(part)
			}
		}
		return b.String()
	}
	before, match, after := shared.SplitFileName(fullpath, term)
	return before + bold + match + reset + after
}

func main() {
	var opts Options
	parser := flags.NewParser(&opts, flags.Default|flags.PrintErrors|flags.HelpFlag)

	rest, err := parser.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		parser.WriteHelp(os.Stdout)
		os.Exit(1)
	}

	if opts.Version {
		fmt.Println(version.Info())
		os.Exit(0)
	}

	if hint := shellExpansionHint(rest); hint != "" {
		fmt.Fprintln(os.Stderr, hint)
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

		if opts.UserHome {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Println("Error finding home directory: ", err)
				os.Exit(1)
			}
			results = filterUnderDir(results, home)
		}

		if opts.Executable {
			results = filterExecutable(results)
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].Fullpath < results[j].Fullpath
		})

		if opts.Highlight {
			for _, r := range results {
				fmt.Println(highlightPath(r.Fullpath, opts.Args.SearchTerm))
			}
		} else {
			for _, r := range results {
				fmt.Println(r.Fullpath)
			}
		}
		os.Exit(0)
	}
}
