package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// With a dialog open over the main window, Escape dismisses the dialog and
// leaves the window, even when Escape is set to close it. The next Escape
// closes the window.
func TestEscapeDismissesDialogBeforeWindow(t *testing.T) {
	usePrefs(t, defaultPrefs())
	prefs.CloseOnEscape = true
	closed := 0
	closeMainWindow = func() { closed++ }
	defer func() { closeMainWindow = nil; openDialogs = nil }()

	w := test.NewTempWindow(t, widget.NewLabel("main"))
	w.Resize(fyne.NewSize(600, 400))
	d := dialog.NewCustom("Properties", "Close", widget.NewLabel("details"), w)
	showDialog(d)
	mainWindowEscape()
	if closed != 0 {
		t.Fatalf("Escape closed the window with a dialog open")
	}
	if len(openDialogs) != 0 || w.Canvas().Overlays().Top() != nil {
		t.Errorf("Escape left the dialog open")
	}

	mainWindowEscape()
	if closed != 1 {
		t.Errorf("with no dialog open, Escape closed the window %d times, want 1", closed)
	}
}

// The dialog's own button also drops it from the dialogs Escape dismisses.
func TestDialogButtonForgetsDialog(t *testing.T) {
	defer func() { openDialogs = nil }()
	w := test.NewTempWindow(t, widget.NewLabel("main"))
	w.Resize(fyne.NewSize(600, 400))
	d := dialog.NewCustom("Properties", "Close", widget.NewLabel("details"), w)
	showDialog(d)
	d.Hide()
	if len(openDialogs) != 0 {
		t.Errorf("closed dialog still tracked: %d", len(openDialogs))
	}
}

// Escape closes the Preferences window, even after a click on a check.
func TestEscapeClosesPreferences(t *testing.T) {
	usePrefs(t, defaultPrefs())
	showPreferences()
	w := prefsWindow
	if w == nil {
		t.Fatal("no Preferences window")
	}
	tabs := findWidget[*container.AppTabs](w.Content())
	if tabs == nil {
		t.Fatal("no tabs in Preferences")
	}
	tabs.SelectIndex(1) // Results, which has checks
	check := findWidget[*widget.Check](tabs.Selected().Content)
	if check == nil {
		t.Fatal("no check in Preferences")
	}
	test.Tap(check)
	if prefsWindow.Canvas().Focused() != nil {
		t.Error("the check kept focus after the click, so it would swallow Escape")
	}
	pressEscape(w.Canvas())
	if prefsWindow != nil {
		t.Errorf("Escape left Preferences open")
	}
}

// pressEscape delivers Escape as the desktop driver does: to the focused
// object if there is one, else to the canvas.
func pressEscape(c fyne.Canvas) {
	ev := &fyne.KeyEvent{Name: fyne.KeyEscape}
	if f := c.Focused(); f != nil {
		f.TypedKey(ev)
	} else if h := c.OnTypedKey(); h != nil {
		h(ev)
	}
}
