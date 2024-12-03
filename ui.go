package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	widgetx "fyne.io/x/fyne/widget"
)

func loadUI() {
	a := app.New()
	w := a.NewWindow("Entry Completion")

	input := widgetx.NewCompletionEntry([]string{})
	input.SetPlaceHolder("Enter filename...")

	content := container.NewVBox(
		input,
		widget.NewButton("Open", func() {
			fmt.Println("Text:", input.Text)
		}),
	)
	input.OnChanged = func(s string) {
		results := getFileList(input.Text)
		fmt.Println("Results:", len(results))

		// no results
		if len(results) == 0 {
			input.HideCompletion()
			return
		}

		// then show them
		input.SetOptions(results)
		input.ShowCompletion()
	}

	w.SetContent(content)

	w.Resize(fyne.NewSize(600, 500))
	w.ShowAndRun()

	// anything below will not be executed until app is closed
	//getInput(input)
}
