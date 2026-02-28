package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/mmindex"
	_ "github.com/mattn/go-sqlite3"
	zsqlite "zombiezen.com/go/sqlite"
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

func BenchmarkPrefixSearch_InMemory(b *testing.B) {
	db, err := ffdb.OpenDBReadOnly(targetDBPath)
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	defer db.Close()

	idx, err := ffdb.LoadIndex(db)
	if err != nil {
		b.Fatalf("load index: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix := benchPrefixes[i%len(benchPrefixes)]
		results := idx.Search(prefix, maxSearchResults)
		_ = results
	}
}

func BenchmarkPrefixSearch_Zombiezen(b *testing.B) {
	conn, err := zsqlite.OpenConn(targetDBPath, zsqlite.OpenReadOnly|zsqlite.OpenURI)
	if err != nil {
		b.Fatalf("open conn: %v", err)
	}
	defer conn.Close()

	const query = "SELECT fullpath, object_type FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix := "%" + benchPrefixes[i%len(benchPrefixes)] + "%"

		stmt := conn.Prep(query)
		stmt.BindText(1, prefix)
		stmt.BindInt64(2, int64(maxSearchResults))

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				b.Fatalf("step error: %v", err)
			}
			if !rowReturned {
				break
			}
			_ = stmt.ColumnText(0)
			_ = stmt.ColumnInt(1)
		}

		if err := stmt.Reset(); err != nil {
			b.Fatalf("reset error: %v", err)
		}
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
