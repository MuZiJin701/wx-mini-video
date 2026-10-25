//go:build !windows && !darwin

package ui

// SystemDark 在没有系统明暗查询实现的平台上回退深色。
func SystemDark() bool {
	return true
}
