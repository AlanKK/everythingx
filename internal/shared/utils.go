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

// GetFileInfo returns a compact, aligned key/value summary of a file's
// metadata, suitable for display in a tooltip. Labels are left-aligned in a
// fixed column so values line up under a monospace font.
func GetFileInfo(path string) (string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	owner, group := fileOwnerGroup(fileInfo)

	rows := [][2]string{
		{"Size", HumanizeSize(fileInfo.Size())},
		{"Last Modified", fileInfo.ModTime().Format("Jan 2 2006 15:04")},
		{"Mode", fileInfo.Mode().String()},
		{"Owner", owner},
		{"Group", group},
	}

	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		// Pad the label to line the values up; "Last Modified" (13) is the
		// widest, so a 15-wide column leaves a two-space gutter.
		fmt.Fprintf(&b, "%-15s%s", r[0], r[1])
	}

	return b.String(), nil
}

// fileOwnerGroup resolves the owner and group names for a file, falling back to
// numeric ids when a name lookup fails.
func fileOwnerGroup(fileInfo os.FileInfo) (owner, group string) {
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

// HumanizeSize formats a byte count as a short human-readable string.
func HumanizeSize(size int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case size >= tb:
		return fmt.Sprintf("%.1fT", float64(size)/tb)
	case size >= gb:
		return fmt.Sprintf("%.1fG", float64(size)/gb)
	case size >= mb:
		return fmt.Sprintf("%.1fM", float64(size)/mb)
	case size >= kb:
		return fmt.Sprintf("%.1fK", float64(size)/kb)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

func GetFileSizeMod(path string) (string, string) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", ""
	}

	size := fileInfo.Size()
	lastModified := fileInfo.ModTime()

	formattedSize := fmt.Sprintf("%.1fK", float64(size)/1024)
	if size >= 1024*1024 {
		formattedSize = fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
	}
	if size >= 1024*1024*1024 {
		formattedSize = fmt.Sprintf("%.1fG", float64(size)/(1024*1024*1024))
	}
	if size >= 1024*1024*1024*1024 {
		formattedSize = fmt.Sprintf("%.1fT", float64(size)/(1024*1024*1024*1024))
	}

	return formattedSize, lastModified.Format("Jan 2 2006 15:04")
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
