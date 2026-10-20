package certificate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinKeychainReadAndPrivateKeyPermissions(t *testing.T) {
	if os.Getenv("WX_MINI_VIDEO_MACOS_INTEGRATION") != "1" {
		t.Skip("设置 WX_MINI_VIDEO_MACOS_INTEGRATION=1 只读检查真实钥匙串")
	}
	certificates, err := macFetchCertificates()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "root-ca.pem"): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("%s 权限为 %o，需要 %o", path, info.Mode().Perm(), mode)
		}
	}
	t.Logf("已读取钥匙串（%d 个可信根）并验证临时私钥权限；未安装证书", len(certificates))
}
