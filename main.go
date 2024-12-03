package main

func main() {
	load("data/all-files.txt")

	loadUI()
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
