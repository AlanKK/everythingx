package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/derekparker/trie"
)

var T *trie.Trie

type FileList struct {
	filename string
	fullpath string
}

func addToTrie(filename string, fullpath string) {
	T.Add(filename, fullpath)
}

func load(filename string) {
	T = trie.New()

	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "/" {
			continue
		}
		base := filepath.Base(line)
		addToTrie(base, line)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
	}
}

// returns a list of files that start with the characters
func getFileList(characters string) []string {
	return T.PrefixSearch(characters)
}

// returns a list of files that start with the characters
// func getFullList(characters string) [][]string {
// 	var filesAndPaths [][]string

// 	files := T.PrefixSearch(characters)

// 	for _, r := range files {
// 		node, ok := findFile(r)
// 		if ok {
// 			fullpath := node.Meta().(string)
// 			filesAndPaths = append(filesAndPaths, []string{r, fullpath})
// 			fmt.Println("Found:", r, fullpath)
// 		}
// 	}
// 	return filesAndPaths
// }

// func findFile(r string) (*trie.Node, bool) {
// 	return T.Find(r)
// }
