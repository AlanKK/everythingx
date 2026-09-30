package main

import (
	"flag"
	"log"

	"github.com/AlanKK/everythingx/internal/ffdb"
	"github.com/AlanKK/everythingx/internal/shared"
)

func main() {
	pathname := flag.String("path", shared.DefaultDBPath(), "Path to the database file")
	flag.Parse()

	db, err := ffdb.CreateDB(*pathname)
	if err != nil {
		log.Fatalf("Expected no error, got %v", err)
	}
	db.Close()

	log.Println("Database created: ", *pathname)
}
