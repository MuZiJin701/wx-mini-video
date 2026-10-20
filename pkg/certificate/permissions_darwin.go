package certificate

import (
	"fmt"
	"os"
	"syscall"
)

func restrictCertificatePath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("拒绝使用符号链接保存根证书: %s", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("根证书路径不属于当前用户: %s", path)
	}
	if info.IsDir() {
		return os.Chmod(path, 0o700)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("根证书路径不是普通文件: %s", path)
	}
	return os.Chmod(path, 0o600)
}
