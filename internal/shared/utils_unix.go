//go:build !windows

package shared

import (
	"fmt"
	"os"
	"os/user"
	"syscall"
)

// fileOwnerGroup resolves the owner and group names for a file.
func fileOwnerGroup(fileInfo os.FileInfo) (owner, group string) {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}
	owner = fmt.Sprintf("%d", stat.Uid)
	group = fmt.Sprintf("%d", stat.Gid)
	if u, err := user.LookupId(owner); err == nil {
		owner = u.Username
	}
	if g, err := user.LookupGroupId(group); err == nil {
		group = g.Name
	}
	return owner, group
}
