//go:build windows

package system

import "testing"

func TestProxySnapshotPreservesFullRegistryValue(t *testing.T) {
	value := "http=corp:8080; https=127.0.0.1:2023"
	output := "HKEY_CURRENT_USER\\Internet Settings\r\n    ProxyServer    REG_SZ    " + value + "\r\n"
	if got := parseRegValue(output, "ProxyServer"); got != value {
		t.Fatalf("snapshot truncated: %q", got)
	}
	if got := parseRegValue("    ProxyEnable    REG_DWORD    0x1\r\n", "ProxyEnable"); got != "0x1" {
		t.Fatalf("DWORD = %q", got)
	}
	if got := parseRegValue("    OtherProxyServer    REG_SZ    other", "ProxyServer"); got != "" {
		t.Fatalf("matched a different value: %q", got)
	}
}
