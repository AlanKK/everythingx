package main

import (
	"database/sql"
	"fmt"
	"os/exec"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	widgetx "github.com/matwachich/fynex-widgets"
)

func handleOpenButtonClick(pathname string) {
	fmt.Println("handleOpenButtonClick: ", pathname)

	cmd := exec.Command("open", "-R", pathname)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error:", err)
	}
}

func handleEntryChanged(db *sql.DB, entry *widgetx.AutoComplete) {
	if len(entry.Text) == 0 {
		entry.ListHide()
		return
	}
	// Calling prefixSearch without a limit parameter, using the default limit of 20
	results, err := prefixSearch(db, entry.Text, 200)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println("Results:", len(results))

	// no results
	if len(results) == 0 {
		if entry.Text == "" {
			return
		}
	}

	// then show them
	entry.Options = results
	entry.ListShow()
	printMemUsage()
}

func loadUI2(db *sql.DB) {
	a := app.New()
	w := a.NewWindow("Entry Completion")

	entry := widgetx.NewAutoComplete(1)
	entry.SetPlaceHolder("Enter filename...")

	content := container.NewVBox(
		entry,
		widget.NewButton("Show in Finder", func() {
			handleOpenButtonClick(entry.Text)
		}),
	)
	entry.OnChanged = func(s string) {
		handleEntryChanged(db, entry)
	}

	w.SetContent(content)

	w.Resize(fyne.NewSize(1300, 800))
	w.ShowAndRun()

	// anything below will not be executed until app is closed
	//getInput(input)
}
