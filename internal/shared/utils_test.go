package shared

import (
	"compress/gzip"
	"os"
	"strings"
	"testing"
)

func TestSplitFileName(t *testing.T) {
	tests := []struct {
		filename   string
		searchTerm string
		beforeTerm string
		term       string
		afterTerm  string
	}{
		{"exAMPle.txt", "example", "", "exAMPle", ".txt"},
		{"example.txt", "example", "", "example", ".txt"},
		{"example.txt", "txt", "example.", "txt", ""},
		{"example.txt", "e", "", "e", "xample.txt"},
		{"example.txt", "x", "e", "x", "ample.txt"},
		{"example.txt", "", "", "", "example.txt"},
		{"example.txt", "Example", "", "example", ".txt"}, // Case sensitive test
		{"example.txt", "TXT", "example.", "txt", ""},     // Case sensitive test
		{"example.txt", "notfound", "example.txt", "", ""},
	}

	for _, tt := range tests {
		before, term, after := SplitFileName(tt.filename, tt.searchTerm)
		if before != tt.beforeTerm || term != tt.term || after != tt.afterTerm {
			t.Errorf("splitFileName(%q, %q) = (%q, %q, %q); want (%q, %q, %q)",
				tt.filename, tt.searchTerm, before, term, after, tt.beforeTerm, tt.term, tt.afterTerm)
		}
	}
}

func TestGetFileInfo(t *testing.T) {
	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some data to the file
	if _, err := tmpFile.Write([]byte("Hello, World!")); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Get file info
	info, err := GetFileInfo(tmpFile.Name())
	if err != nil {
		t.Errorf("getFileInfo(%q) returned error: %v", tmpFile.Name(), err)
	}

	// Check if the output contains expected substrings
	expectedSubstrings := []string{"Mode", "Owner", "Group", "Size", "Last Modified"}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(info, substr) {
			t.Errorf("getFileInfo(%q) = %q; want it to contain %q", tmpFile.Name(), info, substr)
		}
	}
}

func TestFileExists(t *testing.T) {
	// Test that a non-existent file does not exist
	if FileExists("/tmp/nonexistentfile.txt") {
		t.Fatalf("Expected file to not exist, but it does")
	}

	// Create a temporary file
	tempFile, err := os.CreateTemp("/tmp", "testfile")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	defer os.Remove(tempFile.Name())

	// Test that the file exists
	if !FileExists(tempFile.Name()) {
		t.Fatalf("Expected file to exist, but it does not")
	}
}
func TestGUnZipFile(t *testing.T) {
	// Create a temporary gzip file for testing
	sourceFile, err := os.CreateTemp("", "testfile.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(sourceFile.Name())

	// Write some gzipped data to the file
	gzipWriter := gzip.NewWriter(sourceFile)
	if _, err := gzipWriter.Write([]byte("Hello, World!")); err != nil {
		t.Fatal(err)
	}
	gzipWriter.Close()
	sourceFile.Close()

	// Create a temporary target file
	targetFile, err := os.CreateTemp("", "testfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(targetFile.Name())
	targetFile.Close()

	// Test GUnZipFile function
	err = GUnZipFile(sourceFile.Name(), targetFile.Name())
	if err != nil {
		t.Errorf("GUnZipFile(%q, %q) returned error: %v", sourceFile.Name(), targetFile.Name(), err)
	}

	// Verify the contents of the target file
	content, err := os.ReadFile(targetFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	expectedContent := "Hello, World!"
	if string(content) != expectedContent {
		t.Errorf("GUnZipFile(%q, %q) = %q; want %q", sourceFile.Name(), targetFile.Name(), string(content), expectedContent)
	}
}
