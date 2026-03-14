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
	"syscall"
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

		// Stage 2: Validate that all of the files and directories are in the database
		if err := validateInDB(db, params); err != nil {
			log.Fatalf("Error in validateInDB: %v\n", err)
		}

		// Stage 3: Rename each of the files to a new naming scheme
		if err := renameFiles(params); err != nil {
			log.Fatalf("Error in renameFiles: %v\n", err)
		}

		// Stage 4: Validate the database contains the newly named files and old ones are not in the db
		if err := validateRenamedInDB(db, params); err != nil {
			log.Fatalf("Error in validateRenamedInDB: %v\n", err)
		}

		// Stage 5: Delete all files and directories
		if err := deleteHierarchy(params); err != nil {
			log.Fatalf("Error in deleteHierarchy: %v\n", err)
		}

		// Stage 6: Validate that none of the files and directories are in the database
		if err := validateNotInDB(db, params); err != nil {
			log.Fatalf("Error in validateNotInDB: %v\n", err)
		}

		// Stage 7: Test directory rename (all children paths should update)
		dirRenameParams := &Params{
			RootDir:     rootDir,
			TestDirs:    testDirs,
			DirPrefix:   filepath.Join(testDirs, fmt.Sprintf("rename_test_%d", i)),
			Depth:       3,
			FilesPerDir: 20,
		}
		if err := testDirectoryRename(db, dirRenameParams, cmd); err != nil {
			log.Fatalf("Error in testDirectoryRename: %v\n", err)
		}

		// Stage 8: Test directory rm -rf (cascade delete)
		rmrfParams := &Params{
			RootDir:     rootDir,
			TestDirs:    testDirs,
			DirPrefix:   filepath.Join(testDirs, fmt.Sprintf("rmrf_test_%d", i)),
			Depth:       3,
			FilesPerDir: 20,
		}
		if err := testDirectoryRmRf(db, rmrfParams); err != nil {
			log.Fatalf("Error in testDirectoryRmRf: %v\n", err)
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

// waitForAllInDB polls every 2s until all paths are present in the DB or timeout elapses.
func waitForAllInDB(db *sql.DB, paths []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		missing := 0
		for _, path := range paths {
			if !inDB(db, path) {
				missing++
			}
		}
		if missing == 0 {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

// waitForNoneInDB polls every 2s until none of the paths are in the DB or timeout elapses.
func waitForNoneInDB(db *sql.DB, paths []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		found := 0
		for _, path := range paths {
			if inDB(db, path) {
				found++
			}
		}
		if found == 0 {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func validateNotInDB(db *sql.DB, p *Params) error {
	var paths []string
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			paths = append(paths, filepath.Join(dir, fmt.Sprintf("file%d.renamed", j)))
		}
	}
	if !waitForNoneInDB(db, paths, 30*time.Second) {
		found := 0
		for _, path := range paths {
			if inDB(db, path) {
				found++
			}
		}
		log.Fatalf("%d files were not deleted from the DB", found)
	}
	log.Println("All files were deleted from the DB")
	return nil
}

func validateInDB(db *sql.DB, p *Params) error {
	var paths []string
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			paths = append(paths, filepath.Join(dir, fmt.Sprintf("file%d", j)))
		}
	}
	if !waitForAllInDB(db, paths, 30*time.Second) {
		missing := 0
		for _, path := range paths {
			if !inDB(db, path) {
				log.Printf("File not found in db: %s\n", path)
				missing++
			}
		}
		log.Fatalf("%d files missing from the DB", missing)
	}
	log.Println("All files found in the DB")
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
	var oldPaths, newPaths []string
	for i := 0; i < p.Depth; i++ {
		dir := fmt.Sprintf("%s%d", p.DirPrefix, i)
		for j := 0; j < p.FilesPerDir; j++ {
			oldFile := filepath.Join(dir, fmt.Sprintf("file%d", j))
			oldPaths = append(oldPaths, oldFile)
			newPaths = append(newPaths, oldFile+".renamed")
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		old, missing := 0, 0
		for _, path := range oldPaths {
			if inDB(db, path) {
				old++
			}
		}
		for _, path := range newPaths {
			if !inDB(db, path) {
				missing++
			}
		}
		if old == 0 && missing == 0 {
			log.Println("All files renamed correctly in the DB")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	old, missing := 0, 0
	for _, path := range oldPaths {
		if inDB(db, path) {
			log.Printf("File found in db that was renamed: %s\n", path)
			old++
		}
	}
	for _, path := range newPaths {
		if !inDB(db, path) {
			log.Printf("File not found in db: %s\n", path)
			missing++
		}
	}
	log.Printf("Missing from db: %d. In db but shouldn't be: %d\n", missing, old)
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

func testDirectoryRename(db *sql.DB, p *Params, cmd *exec.Cmd) error {
	log.Println("Testing directory rename (path prefix update)")

	// Create a directory with subdirs and files
	parentDir := p.DirPrefix + "_parent"
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	var allFiles []string
	for i := 0; i < p.Depth; i++ {
		subdir := filepath.Join(parentDir, fmt.Sprintf("subdir%d", i))
		if err := os.MkdirAll(subdir, 0755); err != nil {
			return fmt.Errorf("failed to create subdir: %w", err)
		}
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(subdir, fmt.Sprintf("testfile%d", j))
			if _, err := os.Create(file); err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			allFiles = append(allFiles, file)
		}
	}

	// Poll until all files are indexed
	if !waitForAllInDB(db, allFiles, 30*time.Second) {
		return fmt.Errorf("files not indexed before rename (timeout)")
	}
	log.Printf("All %d files indexed before rename", len(allFiles))

	// Rename the parent directory
	renamedParent := p.DirPrefix + "_renamed"
	if err := os.Rename(parentDir, renamedParent); err != nil {
		return fmt.Errorf("failed to rename directory: %w", err)
	}

	// Trigger a rescan via SIGUSR1 so the service picks up the new paths and
	// removes the stale old paths (eventual consistency path).
	log.Println("Sending SIGUSR1 to trigger rescan after rename")
	if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("failed to send SIGUSR1: %w", err)
	}

	// Build the expected new paths
	var newFiles []string
	for _, oldFile := range allFiles {
		newFiles = append(newFiles, strings.Replace(oldFile, parentDir, renamedParent, 1))
	}

	// Poll until all new paths are in DB
	if !waitForAllInDB(db, newFiles, 45*time.Second) {
		missing := 0
		for _, f := range newFiles {
			if !inDB(db, f) {
				log.Printf("File not found under new path: %s\n", f)
				missing++
			}
		}
		os.RemoveAll(renamedParent)
		return fmt.Errorf("%d files not found at new path after rename + rescan", missing)
	}

	// Poll until old paths are gone
	if !waitForNoneInDB(db, allFiles, 15*time.Second) {
		oldCount := 0
		for _, f := range allFiles {
			if inDB(db, f) {
				oldCount++
			}
		}
		os.RemoveAll(renamedParent)
		return fmt.Errorf("%d files still have old paths in DB after rename + rescan", oldCount)
	}

	os.RemoveAll(renamedParent)
	log.Printf("Directory rename test passed: all %d files at new path, old paths cleaned up", len(allFiles))
	return nil
}

func testDirectoryRmRf(db *sql.DB, p *Params) error {
	log.Println("Testing directory rm -rf (cascade delete)")

	// Create a directory with subdirs and files
	parentDir := p.DirPrefix + "_parent"
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent dir: %w", err)
	}

	var allPaths []string
	allPaths = append(allPaths, parentDir)
	for i := 0; i < p.Depth; i++ {
		subdir := filepath.Join(parentDir, fmt.Sprintf("subdir%d", i))
		if err := os.MkdirAll(subdir, 0755); err != nil {
			return fmt.Errorf("failed to create subdir: %w", err)
		}
		allPaths = append(allPaths, subdir)
		for j := 0; j < p.FilesPerDir; j++ {
			file := filepath.Join(subdir, fmt.Sprintf("testfile%d", j))
			if _, err := os.Create(file); err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			allPaths = append(allPaths, file)
		}
	}

	// Poll until all files and dirs are indexed
	if !waitForAllInDB(db, allPaths, 30*time.Second) {
		return fmt.Errorf("files not indexed before rm -rf (timeout)")
	}
	log.Printf("All %d paths indexed before rm -rf", len(allPaths))

	// rm -rf the entire directory tree
	if err := os.RemoveAll(parentDir); err != nil {
		return fmt.Errorf("failed to rm -rf directory: %w", err)
	}

	// Poll until all paths are removed from DB
	if !waitForNoneInDB(db, allPaths, 30*time.Second) {
		remainingCount := 0
		for _, path := range allPaths {
			if inDB(db, path) {
				log.Printf("Path still in DB after rm -rf: %s\n", path)
				remainingCount++
			}
		}
		return fmt.Errorf("%d paths still in DB after rm -rf", remainingCount)
	}

	log.Printf("Directory rm -rf test passed: all %d paths removed from DB", len(allPaths))
	return nil
}
