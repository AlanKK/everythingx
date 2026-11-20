package main

import (
	"fmt"
	"math"
	"os"
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
// the shell expanded an unquoted pattern such as uninstall*sh into the
// matching files in the current directory before ev ran. term is what ev
// searched for, and rest the arguments it ignored. It returns "" when there
// are none.
func shellExpansionHint(term string, rest []string) string {
	if len(rest) == 0 {
		return ""
	}
	quoted := make([]string, len(rest))
	for i, r := range rest {
		quoted[i] = fmt.Sprintf("%q", r)
	}
	ignored := strings.Join(quoted, ", ")
	if len(quoted) > 3 {
		ignored = strings.Join(quoted[:3], ", ") + fmt.Sprintf(" and %d more", len(quoted)-3)
	}
	example := "'name*.ext'"
	if guess := guessPattern(append([]string{term}, rest...)); guess != "" {
		example = "'" + guess + "'"
	}
	return fmt.Sprintf("hint: the shell expanded a wildcard pattern into %d file names before ev saw it,\n"+
		"  so ev searched for %q and ignored %s.\n"+
		"  Quote the pattern, for example: ev %s\n"+
		"  To stop the shell expanding ev's arguments, see \"Typing patterns without quotes\" in the README.",
		len(rest)+1, term, ignored, example)
}

// guessPattern reconstructs a likely pattern from the names a shell expanded
// it into: their common prefix and suffix around a "*". It returns "" when
// the names share neither.
func guessPattern(names []string) string {
	prefix, suffix := names[0], names[0]
	shortest := len(names[0])
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		for !strings.HasSuffix(n, suffix) {
			suffix = suffix[1:]
		}
		shortest = min(shortest, len(n))
	}
	if len(prefix)+len(suffix) > shortest {
		suffix = suffix[len(prefix)+len(suffix)-shortest:]
	}
	if prefix == "" && suffix == "" {
		return ""
	}
	return prefix + "*" + suffix
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

	if hint := shellExpansionHint(opts.Args.SearchTerm, rest); hint != "" {
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

		for _, r := range results {
			fmt.Println(r.Fullpath)
		}
		os.Exit(0)
	}
}
