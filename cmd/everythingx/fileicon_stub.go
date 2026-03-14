//go:build !darwin

package main

func getFileIconPNG(path string, size int) []byte {
	return nil
}
