//go:build !windows

package tun

import "os"

func isAdmin() bool {
	return os.Getuid() == 0
}
