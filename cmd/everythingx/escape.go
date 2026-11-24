package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// openDialogs are the dialogs shown over the main window, newest last. Escape
// dismisses the newest before it does anything to the window. UI thread only.
var openDialogs []dialog.Dialog

// showDialog shows d over the main window, where Escape dismisses it.
func showDialog(d dialog.Dialog) {
	openDialogs = append(openDialogs, d)
	d.SetOnClosed(func() {
		for i, o := range openDialogs {
			if o == d {
				openDialogs = append(openDialogs[:i], openDialogs[i+1:]...)
				break
			}
		}
	})
	d.Show()
}

// dismissDialog hides the newest dialog over the main window, as its button
// would, and reports whether there was one. UI thread only.
func dismissDialog() bool {
	if len(openDialogs) == 0 {
		return false
	}
	openDialogs[len(openDialogs)-1].Hide() // its OnClosed drops it from the list
	return true
}

// mainWindowEscape is what Escape does in the main window: dismiss a dialog
// if one is open, else close the window when the user wants that. It reports
// whether it did either. UI thread only.
func mainWindowEscape() bool {
	return dismissDialog() || escapeClosesWindow()
}

// closeOnEscape makes Escape in w do close, as the window's own button does.
// The canvas gets Escape only when nothing in w has focus; see dropFocus.
func closeOnEscape(w fyne.Window, close func()) {
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == fyne.KeyEscape {
			close()
		}
	})
}

// dropFocus takes the focus from o, which a check, radio button or select
// takes when clicked and then keeps, swallowing Escape. Call it once the
// control has done its job so Escape reaches the window again.
func dropFocus(o fyne.CanvasObject) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		c.Unfocus()
	}
}
