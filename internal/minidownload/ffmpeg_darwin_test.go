package minidownload

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDarwinManagedFFmpeg(t *testing.T) {
	if os.Getenv("WX_MINI_VIDEO_MACOS_INTEGRATION") != "1" {
		t.Skip("设置 WX_MINI_VIDEO_MACOS_INTEGRATION=1 下载并运行固定版本 ffmpeg")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dir := t.TempDir()
	path, err := EnsureFFmpeg(ctx, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ManagedFFmpegPath(dir) != path {
		t.Fatal("受管 ffmpeg 校验标记无效")
	}
	output, err := exec.CommandContext(ctx, path, "-version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(output), "ffmpeg version ") {
		t.Fatalf("ffmpeg 无法运行: %v: %s", err, output)
	}
	t.Log(strings.SplitN(string(output), "\n", 2)[0])
	t.Setenv("PATH", filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Run("authenticatedHLS", TestAuthenticatedHLSWithRealFFmpeg)
}
