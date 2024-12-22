package main

import (
	"flag"
	"io/fs"
	"log"
	"path/filepath"
	"time"
)

func main() {
	root := flag.String("root", "/", "Root path to begin scan")
	flag.Parse()

	log.Println("Scanning from", *root)

	startTime := time.Now()

	fileCount := 0
	dirCount := 0
	filepath.WalkDir("/", func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !file.IsDir() {
			//log.Println(path)
			fileCount++
		} else {
			dirCount++
		}
		return nil
	})

	log.Printf("Total files/dirs: %d/%d. Elapsed time: %s", fileCount, dirCount, time.Since(startTime).String())
}
