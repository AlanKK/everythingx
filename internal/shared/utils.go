package shared

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func GetHomeDirPath() string {
	homeDir := "/Users"
	switch runtime.GOOS {
	case "darwin":
		homeDir = "/Users"
	case "linux", "freebsd", "openbsd", "netbsd":
		homeDir = "/home"
	default:
		log.Printf("Unsupported operating system: %s", runtime.GOOS)
	}
	return homeDir
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// FileOwnerGroup resolves the owner and group names for a file, falling back to
// numeric ids when a name lookup fails.
func FileOwnerGroup(fileInfo os.FileInfo) (owner, group string) {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}
	owner = fmt.Sprintf("%d", stat.Uid)
	group = fmt.Sprintf("%d", stat.Gid)
	if u, err := user.LookupId(fmt.Sprintf("%d", stat.Uid)); err == nil {
		owner = u.Username
	}
	if g, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid)); err == nil {
		group = g.Name
	}
	return owner, group
}

// HumanizeSize formats a byte count as a short human-readable string in
// binary units, e.g. "1.5K" for 1536 bytes.
func HumanizeSize(size int64) string {
	return HumanizeSizeUnits(size, false)
}

// HumanizeSizeUnits formats a byte count as a short human-readable string.
// Binary units count in 1024s and write "1.5K"; decimal units count in 1000s
// and write "1.5kB", so the two can't be mistaken for each other.
func HumanizeSizeUnits(size int64, decimal bool) string {
	base := float64(1024)
	suffixes := []string{"K", "M", "G", "T"}
	byteSuffix := "B"
	if decimal {
		base = 1000
		suffixes = []string{"kB", "MB", "GB", "TB"}
	}
	if float64(size) < base {
		return fmt.Sprintf("%d%s", size, byteSuffix)
	}
	value := float64(size)
	unit := -1
	for value >= base && unit < len(suffixes)-1 {
		value /= base
		unit++
	}
	return fmt.Sprintf("%.1f%s", value, suffixes[unit])
}

// GetFileSizeMod returns the size and modification time of a file both
// formatted for display and raw, so callers can sort on them without a second
// stat.
func GetFileSizeMod(path string) (string, string, int64, time.Time) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", "", 0, time.Time{}
	}

	size := fileInfo.Size()
	lastModified := fileInfo.ModTime()

	return HumanizeSize(size), lastModified.Format("Jan 2 2006 15:04"), size, lastModified
}

func PrintMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// fmt.Printf("Alloc = %v MB", bToMb(m.Alloc))
}

func BToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func SplitFileName(filename string, searchTerm string) (string, string, string) {
	beforeTerm := ""
	afterTerm := ""
	actualTerm := ""

	if idx := strings.Index(strings.ToLower(filename), strings.ToLower(searchTerm)); idx != -1 {
		beforeTerm = filename[:idx]
		actualTerm = filename[idx : idx+len(searchTerm)]
		afterTerm = filename[idx+len(searchTerm):]
	} else {
		beforeTerm = filename
	}
	return beforeTerm, actualTerm, afterTerm
}

func GUnZipFile(sourcePath, targetPath string) error {
	// Open the gzip file
	gzipFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open gzip file: %w", err)
	}
	defer gzipFile.Close()

	// Create a gzip reader
	gzipReader, err := gzip.NewReader(gzipFile)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create the target file
	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer targetFile.Close()

	// Copy the data from the gzip reader to the target file
	_, err = io.Copy(targetFile, gzipReader)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}
