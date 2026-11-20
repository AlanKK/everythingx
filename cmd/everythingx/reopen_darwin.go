//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void registerReopenHandler(void);
*/
import "C"

var onReopen func()

// watchReopen calls show whenever macOS asks the running app to reopen. When
// EverythingX.app is opened again (Dock, Finder, Spotlight, `open -a`) while
// it is already running, LaunchServices doesn't start a second process; it
// sends the running one a "reopen" Apple event instead. Neither Fyne nor GLFW
// handles that event, so without this the hidden window would stay hidden.
// Call it once the app is running, so NSApplication's own handler, which is
// installed while it finishes launching, doesn't replace ours.
func watchReopen(show func()) {
	onReopen = show
	C.registerReopenHandler()
}

//export goReopenApplication
func goReopenApplication() {
	if onReopen != nil {
		onReopen()
	}
}
