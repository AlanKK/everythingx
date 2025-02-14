package main

import (
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
		{"alankeister@gmail.com", "alank", "", "alank", "eister@gmail.com"},
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
		before, term, after := splitFileName(tt.filename, tt.searchTerm)
		if before != tt.beforeTerm || term != tt.term || after != tt.afterTerm {
			t.Errorf("splitFileName(%q, %q) = (%q, %q, %q); want (%q, %q, %q)",
				tt.filename, tt.searchTerm, before, term, after, tt.beforeTerm, tt.term, tt.afterTerm)
		}
	}
}
