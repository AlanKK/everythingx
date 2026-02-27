package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlanKK/everythingx/internal/mmindex"
)

func main() {
	searchTerm := getCommandLineArgs()

	if searchTerm != "" {
		idx, err := mmindex.Open()
		if err != nil {
			fmt.Println("Error opening index:", err)
			os.Exit(1)
		}
		defer idx.Close()
		results := idx.Search(searchTerm, 1000)
		for _, r := range results {
			fmt.Println(r.Fullpath)
		}
		os.Exit(0)
	}

	loadUI()
}

func getCommandLineArgs() string {
	searchTerm := flag.String("name", "", "Search term")
	flag.Parse()
	return *searchTerm
}
