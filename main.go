package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func main() {
	var db *sql.DB = nil
	var err error

	dbFile := "data/files.db"
	if fileExists(dbFile) {
		db, err = openDB(dbFile)
		if err != nil {
			return
		}
		fmt.Println("Opened database ", dbFile)
	} else {
		fmt.Println("Database does not exist, creating it...")

		db, err := createDBAndTable("data/files.db")
		if err != nil {
			return
		}

		fmt.Println("Loading database from file...")
		err = loadDBFromFile(db, "data/all-files.txt")
		if err != nil {
			return
		}
	}
	defer db.Close()

	loadUI2(db)
}

func loadDBFromFile(db *sql.DB, filename string) error {
	var records int = 0

	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "/" {
			continue
		}
		base := filepath.Base(line)
		err = bulkInsertRecords(db, base, line)
		if err != nil {
			fmt.Println("Error inserting record:", err)
			return err
		}

		records++
	}

	err = commitRecords(db)
	if err != nil {
		fmt.Println("Error committing records:", err)
		return err
	}

	fmt.Println("Loaded", records, "records from %s.", filename)
	return nil
}

// import (
// 	"fmt"

// 	"fyne.io/fyne/v2"
// 	"fyne.io/fyne/v2/app"
// 	"fyne.io/fyne/v2/container"
// 	"fyne.io/fyne/v2/widget"
// )
// func main() {
// 	myApp := app.New()
// 	myWindow := myApp.NewWindow("Text Box Example")

// 	input := widget.NewEntry()
// 	input.SetPlaceHolder("Enter text here...")

// 	content := container.NewVBox(
// 		widget.NewLabel("Enter some text:"),
// 		input,
// 		widget.NewButton("Submit", func() {
// 			fmt.Println("Text entered:", input.Text)
// 		}),
// 	)

// 	myWindow.SetContent(content)
// 	myWindow.Resize(fyne.NewSize(300, 200))
// 	myWindow.Show()
// 	myApp.Run()
// 	fmt.Println("Done.")
// }
