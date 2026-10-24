//go:build windows

package tun

import (
	"golang.org/x/sys/windows"
)

func isAdmin() bool {
	token := windows.GetCurrentProcessToken()
	isElevated := token.IsElevated()
	return isElevated
}
