package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Params struct {
	RootDir     string
	FilePrefix  string
	TestDirs    string
	DirPrefix   string
	Depth       int
	FilesPerDir int
}

func main() {
	if runtime.GOOS == "linux" && os.Getuid() != 0 {
		log.Fatal("e2eTest must be run as root on Linux (fanotify requires elevated privileges). Run: sudo make e2e")
	}

	currentDir, _ := os.Getwd()
	rootDir := filepath.Join(currentDir, "everythingx_test", randomString(10))
	testDirs := filepath.Join(rootDir, "testdirs")
	dbPath := filepath.Join(rootDir, "files.db")
	everythingxd := "bin/everythingxd"

	if err := os.MkdirAll(testDirs, 0755); err != nil {
		log.Fatalf("failed to create directory: %s. %v", rootDir, err)
	}

	log.Printf("Running program with root directory: %s, currentDir %s\n", rootDir, currentDir)

	resultChan := make(chan *exec.Cmd)
	log.Println("Starting service")

	go startService(everythingxd, rootDir, testDirs, dbPath, resultChan)
	waitForServiceReady(rootDir)
	log.Println("Done waiting for service to start")

	cmd := <-resultChan
	defer func() {
		log.Printf("Killing process with PID %d\n", cmd.Process.Pid)
		cmd.Process.Signal(os.Interrupt)
		cmd.Process.Wait()
	}()
	time.Sleep(10 * time.Second)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v\n", err)
	}
	defer db.Close()

	iterations := 2
	for i := 1; i <= iterations; i++ {
		// Having too many FilesPerDir too quickly can cause the Apple's FSE to hang
		params := &Params{
			RootDir:     rootDir,
			TestDirs:    testDirs,
			DirPrefix:   filepath.Join(testDirs, fmt.Sprintf("dir_%d", i)),
			Depth:       5,
			FilesPerDir: 100,
		}

		log.Printf("==========  Running iteration %d/%d  ==========", i, iterations)
		// Stage 1: Create directory hierarchy and populate with files
		if err := createHierarchy(params); err != nil {
			log.Fatalf("Error in createHierarchy: %v\n", err)
		}
		time.Sleep(15 * time.Second)

		// Stage 2: Validate that all of the files and directories are in the database
		if err := validateInDB(db, params); err != nil {
			log.Fatalf("Error in validateInDB: %v\n", err)
		}

		// Stage 3: Rename each of the files to a new naming scheme
		if err := renameFiles(params); err != nil {
			log.Fatalf("Error in renameFiles: %v\n", err)
		}
		time.Sleep(15 * time.Second)

		// Stage 4: Validate the database contains the newly named files and old ones are not in the db
		if err := validateRenamedInDB(db, params); err != nil {
			log.Fatalf("Error in validateRenamedInDB: %v\n", err)
		}

		// Stage 5: Delete all files and directories
		if err := deleteHierarchy(params); err != nil {
			log.Fatalf("Error in deleteHierarchy: %v\n", err)
		}
		time.Sleep(15 * time.Second)

		// Stage 6: Validate that none of the files and directories are in the database
		if err := validateNotInDB(db, params); err != nil {
			log.Fatalf("Error in validateNotInDB: %v\n", err)
		}
	}
	log.Println("All tests passed")
}

func startService(everythingxd string, rootDir string, testDirs string, dbPath string, resultChan chan *exec.Cmd) {
	args := []string{"--monitor_path=" + testDirs, "--db_path=" + dbPath, "--nocache"}
	log.Printf("Starting service with args: %v\n", args)

	cmd := exec.Command(everythingxd, args...)

	logFile := filepath.Join(rootDir, "service.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v\n", err)
	}
	cmd.Stdout = file
	cmd.Stderr = file

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start executable: %v\n", err)
	}
	resultChan <- cmd

	log.Printf("Started service with PID %d\n", cmd.Process.Pid)

	// Wait for the process to finish
	go func() {
		err := cmd.Wait()
		if err != nil {
			log.Printf("Service exited with error: %v\n", err)
		}
		log.Printf("Service exited successfully\n")
	}()
}

func waitForFileExists(rootDir string) {
	for {
		if _, err := os.Stat(filepath.Join(rootDir, "service.log")); os.IsNotExist(err) {
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}
}

func waitForServiceReady(rootDir string) {
	log.Println("Waiting for service to start")

	logFile := filepath.Join(rootDir, "service.log")
	waitForFileExists(logFile)

	file, err := os.Open(logFile)
	if err != nil {
		log.Fatalf("Failed to open log file: %v\n", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	deadline := time.Now().Add(60 * time.Second)

	for {
		if time.Now().After(deadline) {
			log.Fatalf("Timed out waiting for service to start. Check %s for errors.", logFile)
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Event listener ready") {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading stdout: %v\n", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func createHierarchy(p *Params) error {
	log.Println("Creating files and directories")
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("failed to create directory: %s. %v", dir, err)
			return err
		}
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(dir, fmt.Sprintf("file%d", j))
			if _, err := os.Create(file); err != nil {
				log.Printf("failed to create file: %s. %v", file, err)
				return err
			} else {
				//log.Printf("Created file: %s\n", file)
			}
		}
	}
	return nil
}

func validateNotInDB(db *sql.DB, p *Params) error {
	found := 0
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(dir, fmt.Sprintf("file%d.renamed", j))
			if inDB(db, file) {
				found++
			}
		}
	}
	if found > 0 {
		log.Fatalf("%d files were not deleted from the DB", found)
	} else {
		log.Printf("All files were deleted from the DB")
	}
	return nil
}

func validateInDB(db *sql.DB, p *Params) error {
	missing := 0
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(dir, fmt.Sprintf("file%d", j))
			if !inDB(db, file) {
				log.Printf("File not found in db: %s\n", file)
				missing++
			}
		}
	}
	if missing > 0 {
		log.Fatalf("%d files missing from the DB", missing)
	} else {
		log.Printf("All files found in the DB")
	}
	return nil
}

func renameFiles(p *Params) error {
	log.Println("Renaming files")
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			oldFile := filepath.Join(dir, fmt.Sprintf("file%d", j))
			renamedFile := oldFile + ".renamed"
			if err := os.Rename(oldFile, renamedFile); err != nil {
				log.Printf("failed to rename file: %s. %v", oldFile, err)
				return err
			}
		}
	}
	return nil
}

func validateRenamedInDB(db *sql.DB, p *Params) error {
	old := 0
	renamed := 0

	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			oldFile := filepath.Join(dir, fmt.Sprintf("file%d", j))
			renamedFile := oldFile + ".renamed"
			if inDB(db, oldFile) {
				log.Printf("File found in db that was renamed: %s\n", oldFile)
				old++
			}
			if !inDB(db, renamedFile) {
				log.Printf("File not found in db: %s\n", renamedFile)
				renamed++
			}
		}
	}
	if renamed > 0 || old > 0 {
		log.Printf("Missing from db: %d. In db but shouldn't be: %d\n", renamed, old)
	} else {
		log.Printf("All files renamed correctly in the DB")
	}
	return nil
}

func deleteHierarchy(p *Params) error {
	log.Println("Deleting files and directories")
	for i := p.Depth - 1; i >= 0; i-- {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(dir, fmt.Sprintf("file%d.renamed", j))
			if err := os.Remove(file); err != nil {
				log.Printf("failed to delete file: %s. %v", file, err)
				return err
			}
		}
		if err := os.Remove(dir); err != nil {
			log.Printf("failed to delete directory: %s. %v", dir, err)
			return err
		}
	}
	return nil
}

func inDB(db *sql.DB, path string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM files WHERE fullpath = ?", path).Scan(&count)
	if err != nil {
		log.Printf("Failed to query database: %v\n", err)
		return false
	}
	return count > 0
}
