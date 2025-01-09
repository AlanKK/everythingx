package main

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileTask struct {
	path string
	info os.FileInfo
}

type Result struct {
	path string
	hash string
	err  error
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

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func worker(id int, tasks <-chan FileTask, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for task := range tasks {
		if task.info.Mode()&os.ModeSymlink != 0 {
			continue // Skip symbolic links
		}

		if !task.info.Mode().IsRegular() {
			continue // Skip non-regular files
		}

		hash, err := calculateMD5(task.path)
		results <- Result{
			path: task.path,
			hash: hash,
			err:  err,
		}
	}
}

func main() {
	const numWorkers = 4
	tasks := make(chan FileTask, 100)
	results := make(chan Result, 100)

	// Create output file with buffered writer
	outFile, err := os.Create("find.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i, tasks, results, &wg)
	}

	// Start result processor
	done := make(chan bool)
	var processedFiles int
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case result := <-results:
				processedFiles++
				if result.err != nil {
					//log.Printf("Error processing %s: %v\n", result.path, result.err)
					continue
				}
				_, err := fmt.Fprintf(writer, "%s  %s\n", result.hash, result.path)
				if err != nil {
					log.Printf("Error writing to file: %v\n", err)
				}
			case <-ticker.C:
				log.Printf("Progress: processed %d files\n", processedFiles)
			case <-done:
				return
			}
		}
	}()

	// Walk filesystem
	err = filepath.Walk("/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			//log.Printf("Error accessing path %s: %v\n", path, err)
			return nil // Continue walking
		}

		if info.IsDir() {
			return nil // Skip directories
		}

		tasks <- FileTask{path: path, info: info}
		return nil
	})

	if err != nil {
		log.Printf("Error walking filesystem: %v\n", err)
	}

	close(tasks)
	wg.Wait()
	close(results)
	done <- true

	log.Printf("Finished processing %d files\n", processedFiles)
}
