package main

import (
	"fmt"
	"os"
	"runtime"
	"time"
)

func getFileInfo(path string) (int64, time.Time, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0, time.Time{}, err
	}

	size := fileInfo.Size()
	lastModified := fileInfo.ModTime()

	return size, lastModified, nil
}

func printFileInfo(path string) {
	size, lastModified, err := getFileInfo(path)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("File size: %d bytes, Last modified %s\n", size, lastModified)
}

func printMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Alloc = %v MB", bToMb(m.Alloc))
}
func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}
