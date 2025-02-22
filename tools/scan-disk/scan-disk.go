package main

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
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

	outputFile, err := os.Create("/Users/alan/Documents/git/everythingx/data/myscan-md5.txt")
	if err != nil {
		log.Fatalf("Error creating output file: %v", err)
	}
	defer outputFile.Close()

	err = filepath.Walk(*root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				//log.Printf("Permission denied: %s", path)
				return nil // Continue the walk
			}
			log.Printf("Error walking dir: %s. %v", path, err)
			return nil
		}
		if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			fileCount++
			hash, err := calculateMD5(path)
			if err != nil {
				//log.Printf("Error calculating MD5 for file %s: %v", path, err)
				return nil
			}
			_, err = fmt.Fprintf(outputFile, "%s|%s\n", hash, path)
			if err != nil {
				log.Printf("Error writing to output file: %v", err)
				return nil
			}
		} else if info.IsDir() {
			dirCount++
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking the path %q: %v\n", *root, err)
	}

	log.Printf("Total files/dirs: %d/%d. Elapsed time: %s", fileCount, dirCount, time.Since(startTime).String())
}

func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
