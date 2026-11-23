//go:build !linux

package main

// systemFontPaths has no lookup outside Linux, so "System" draws with Fyne's
// own sans-serif there.
func systemFontPaths() [4]string { return [4]string{} }
