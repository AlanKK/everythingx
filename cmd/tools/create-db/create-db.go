package main

import (
	"flag"
	"log"

	"github.com/AlanKK/findfiles/internal/ffdb"
)

func main() {
	pathname := flag.String("path", "files.db", "Path to the database file")
	flag.Parse()

	err := ffdb.CreateDBAndTable(*pathname)
	if err != nil {
		log.Fatalf("Expected no error, got %v", err)
	}

	log.Println("Database created: ", *pathname)
}
