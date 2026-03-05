package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/mmindex"
	_ "github.com/mattn/go-sqlite3"
)

const mmIndexPath = "./benchmark.idx"

var benchPrefixes = []string{"a", "al", "ala", "alan"}

func TestMain(m *testing.M) {
	if _, err := os.Stat(targetDBPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "benchmark.db not found — run the benchmark tool first to create it:\n")
		fmt.Fprintf(os.Stderr, "  cd tools/benchmark-db && go run . -n 100\n")
		os.Exit(1)
	}

	// Build the mmap index from the benchmark DB so BenchmarkPrefixSearch_Mmap
	// uses the same data as the other benchmarks.
	db, err := ffdb.OpenDBReadOnly(targetDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open benchmark.db: %v\n", err)
		os.Exit(1)
	}
	if err := mmindex.BuildAt(db, mmIndexPath); err != nil {
		fmt.Fprintf(os.Stderr, "build mmap index: %v\n", err)
		db.Close()
		os.Exit(1)
	}
	db.Close()

	code := m.Run()
	os.Remove(mmIndexPath)
	os.Exit(code)
}

func BenchmarkPrefixSearch_Mattn(b *testing.B) {
	db, err := ffdb.OpenDBReadOnly(targetDBPath)
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix := benchPrefixes[i%len(benchPrefixes)]
		results, err := ffdb.PrefixSearch(prefix, maxSearchResults)
		if err != nil {
			b.Fatalf("search error: %v", err)
		}
		_ = results
	}
}

func BenchmarkPrefixSearch_Mmap(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix := benchPrefixes[i%len(benchPrefixes)]
		idx, err := mmindex.OpenAt(mmIndexPath)
		if err != nil {
			b.Fatalf("open index: %v", err)
		}
		results := idx.Search(prefix, maxSearchResults)
		idx.Close()
		_ = results
	}
}
