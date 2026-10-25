//go:build darwin

package ui

import (
	"os/exec"
	"strings"
)

// SystemDark 读取 macOS 全局外观；读取失败时回退深色。
func SystemDark() bool {
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "dark")
}
