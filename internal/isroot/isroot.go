//go:build !windows

package isroot

import "os/user"

func IsRoot() bool {
	curUser, err := user.Current()
	if err != nil {
		return false
	}

	return curUser.Uid == "0" || curUser.Username == "root"
}
