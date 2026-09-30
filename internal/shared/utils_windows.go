//go:build windows

package shared

import "os"

func fileOwnerGroup(os.FileInfo) (string, string) {
	return "", ""
}
