package main

import (
	"flag"
	"log"

	"github.com/AlanKK/everythingx/internal/ffdb"
)

func main() {
	pathname := flag.String("path", "/Users/alan/Documents/everythingx/files.db", "Path to the database file")
	flag.Parse()

	db, err := ffdb.CreateDB(*pathname)
	if err != nil {
		log.Fatalf("Expected no error, got %v", err)
	}
	db.Close()

	log.Println("Database created: ", *pathname)
}
