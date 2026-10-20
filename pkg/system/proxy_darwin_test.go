package system

import (
	"os"
	"testing"
)

func TestDarwinNetworkSnapshot(t *testing.T) {
	if os.Getenv("WX_MINI_VIDEO_MACOS_INTEGRATION") != "1" {
		t.Skip("设置 WX_MINI_VIDEO_MACOS_INTEGRATION=1 读取真实网络服务")
	}
	snapshot, err := macCaptureProxy()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mac == nil || snapshot.Mac.Service == "" || snapshot.Mac.Interface == "" {
		t.Fatalf("未捕获网络服务: %+v", snapshot)
	}
	if err := snapshot.Mac.CheckNetwork(); err != nil {
		t.Fatal(err)
	}
	t.Logf("已读取网络服务 %q，接口 %q；未更改代理", snapshot.Mac.Service, snapshot.Mac.Interface)
}
