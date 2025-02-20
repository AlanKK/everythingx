package shared

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strings"
	"syscall"
)

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func GetFileInfo(path string) (string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	size := fileInfo.Size()
	lastModified := fileInfo.ModTime()

	// Get file mode and format it like "ls -alH"
	mode := fileInfo.Mode().String()
	stat := fileInfo.Sys().(*syscall.Stat_t)
	owner := fmt.Sprintf("%d", stat.Uid)
	group := fmt.Sprintf("%d", stat.Gid)
	if user, err := user.LookupId(fmt.Sprintf("%d", stat.Uid)); err == nil {
		owner = user.Username
	}
	if grp, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid)); err == nil {
		group = grp.Name
	}

	formattedSize := fmt.Sprintf("%.1fK", float64(size)/1024)
	if size >= 1024*1024 {
		formattedSize = fmt.Sprintf("%.1fM", float64(size)/(1024*1024))
	}
	if size >= 1024*1024*1024 {
		formattedSize = fmt.Sprintf("%.1fG", float64(size)/(1024*1024*1024))
	}
	headers := fmt.Sprintf("%-13s | %-12s | %-12s | %-12s | %-20s\n", "Mode", "Owner", "Group", "Size", "Last Modified")
	horizontalLine := "--------------|--------------|--------------|--------------|--------------------\n"
	lsFormat := headers + horizontalLine + fmt.Sprintf("%-13s | %-12s | %-12s | %-12s | %-20s", mode, owner, group, formattedSize, lastModified.Format("Jan 2 2006 15:04"))

	return lsFormat, nil
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
