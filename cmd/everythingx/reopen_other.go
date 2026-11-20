//go:build !darwin

package main

// watchReopen is a no-op outside macOS: there, a second launch is a real
// second process and goes through the single-instance socket instead.
func watchReopen(func()) {}
