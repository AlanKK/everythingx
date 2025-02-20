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
	currentDir, _ := os.Getwd()
	rootDir := filepath.Join(currentDir, "findfiles_test", randomString(10))
	testDirs := filepath.Join(rootDir, "testdirs")
	dbPath := filepath.Join(rootDir, "files.db")
	findfilesd := "./bin/findfilesd"

	if err := os.MkdirAll(testDirs, 0755); err != nil {
		log.Printf("failed to create directory: %s. %v", rootDir, err)
		return
	}

	log.Printf("Running program with root directory: %s\n", rootDir)

	resultChan := make(chan *exec.Cmd)
	log.Println("Starting service")

	go startService(findfilesd, rootDir, testDirs, dbPath, resultChan)
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
		log.Printf("Failed to open database: %v\n", err)
		return
	}
	defer db.Close()

	iterations := 5
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
			log.Printf("Error in createHierarchy: %v\n", err)
			return
		}
		time.Sleep(5 * time.Second)

		// Stage 2: Validate that all of the files and directories are in the database
		if err := validateInDB(db, params); err != nil {
			log.Printf("Error in validateInDB: %v\n", err)
			return
		}

		// Stage 3: Rename each of the files to a new naming scheme
		if err := renameFiles(params); err != nil {
			log.Printf("Error in renameFiles: %v\n", err)
			return
		}
		time.Sleep(5 * time.Second)

		// Stage 4: Validate the database contains the newly named files and old ones are not in the db
		if err := validateRenamedInDB(db, params); err != nil {
			log.Printf("Error in validateRenamedInDB: %v\n", err)
			return
		}

		// Stage 5: Delete all files and directories
		if err := deleteHierarchy(params); err != nil {
			log.Printf("Error in deleteHierarchy: %v\n", err)
			return
		}
		time.Sleep(5 * time.Second)

		// Stage 6: Validate that none of the files and directories are in the database
		if err := validateNotInDB(db, params); err != nil {
			log.Printf("Error in validateNotInDB: %v\n", err)
			return
		}
	}
}

func startService(findfilesd string, rootDir string, testDirs string, dbPath string, resultChan chan *exec.Cmd) {
	args := []string{"--monitor_path=" + testDirs, "--db_path=" + dbPath, "--nocache"}
	log.Printf("Starting service with args: %v\n", args)

	cmd := exec.Command(findfilesd, args...)

	logFile := filepath.Join(rootDir, "service.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open log file: %v\n", err)
		return
	}
	cmd.Stdout = file
	cmd.Stderr = file

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start executable: %v\n", err)
		return
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
	time.Sleep(5 * time.Second) // some extra time to make sure the service is ready

	file, err := os.Open(logFile)
	if err != nil {
		log.Printf("Failed to open log file: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "Event listener ready") {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading stdout: %v\n", err)
		}
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
		log.Printf("%d files were not deleted from the DB", found)
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
		log.Printf("%d files missing from the DB", missing)
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
