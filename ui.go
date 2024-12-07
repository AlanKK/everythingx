package main

import (
	"database/sql"
	"fmt"
	"os/exec"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	widgetx "github.com/matwachich/fynex-widgets"
)

// TODO:
// - Add system tray icon: https://docs.fyne.io/explore/systray.html
func handleOpenButtonClick(pathname string) {
	fmt.Println("handleOpenButtonClick: ", pathname)

	cmd := exec.Command("open", "-R", pathname)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error:", err)
	}
}

func handleEntryChanged(db *sql.DB, entry *widgetx.AutoComplete) {
	start := time.Now()

	if len(entry.Text) == 0 {
		entry.ListHide()
		return
	}

	searchStart := time.Now()
	results, err := prefixSearch(db, entry.Text, 200)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	searchElapsed := time.Since(searchStart)

	if len(results) == 0 {
		if entry.Text == "" {
			return
		}
	}

	// then show them
	entry.Options = results
	entry.ListShow()

	elapsed := time.Since(start)
	fmt.Printf(
		"handleEntryChanged took %s. prefixSearch: %s\n",
		elapsed,
		searchElapsed,
	)
}

func loadUI(db *sql.DB) {
	a := app.New()
	w := a.NewWindow("Entry Completion")

	// Autocomplete entry box
	entry := widgetx.NewAutoComplete(1)
	entry.SetPlaceHolder("Enter filename...")

	// Status bar
	statusBar := widget.NewLabel("Status: Ready")

	content := container.NewBorder(
		entry,
		statusBar,
		nil,
		nil,
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
}
