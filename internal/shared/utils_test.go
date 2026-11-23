package shared

import (
	"compress/gzip"
	"os"
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

func TestFileOwnerGroup(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner, group := FileOwnerGroup(info)
	if owner == "" || group == "" {
		t.Errorf("FileOwnerGroup = %q, %q; want both set", owner, group)
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

func TestHumanizeSizeUnits(t *testing.T) {
	tests := []struct {
		size    int64
		decimal bool
		want    string
	}{
		{0, false, "0B"},
		{1023, false, "1023B"},
		{1536, false, "1.5K"},
		{5 * 1024 * 1024, false, "5.0M"},
		{3 << 40, false, "3.0T"},
		{999, true, "999B"},
		{1500, true, "1.5kB"},
		{1536, true, "1.5kB"},
		{2_500_000_000, true, "2.5GB"},
		{7_000_000_000_000_000, true, "7000.0TB"},
	}
	for _, tt := range tests {
		if got := HumanizeSizeUnits(tt.size, tt.decimal); got != tt.want {
			t.Errorf("HumanizeSizeUnits(%d, %v) = %q, want %q", tt.size, tt.decimal, got, tt.want)
		}
	}
	if HumanizeSize(1536) != HumanizeSizeUnits(1536, false) {
		t.Error("HumanizeSize should use binary units")
	}
}
