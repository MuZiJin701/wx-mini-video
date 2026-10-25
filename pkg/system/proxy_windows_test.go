//go:build windows

package system

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func createTestProxyKey(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf(`Software\wx-mini-video-tests\%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatalf("create test registry key: %v", err)
	}
	if err := key.Close(); err != nil {
		t.Fatalf("close test registry key: %v", err)
	}
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, path) })
	return path
}

func openTestProxyKey(t *testing.T, path string) registry.Key {
	t.Helper()
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		t.Fatalf("open test registry key: %v", err)
	}
	t.Cleanup(func() { _ = key.Close() })
	return key
}

func TestCaptureProxySnapshotTreatsMissingValuesAsUnset(t *testing.T) {
	path := createTestProxyKey(t)
	snapshot, err := captureProxySnapshotAt(path)
	if err != nil {
		t.Fatalf("captureProxySnapshotAt() error = %v, want nil for missing values", err)
	}
	if snapshot.Enabled || snapshot.HasServer || snapshot.Server != "" {
		t.Fatalf("snapshot = %+v, want empty snapshot for missing values", snapshot)
	}
}

func TestCaptureProxySnapshotTreatsMissingKeyAsUnset(t *testing.T) {
	path := fmt.Sprintf(`Software\wx-mini-video-tests-missing\%d`, time.Now().UnixNano())
	snapshot, err := captureProxySnapshotAt(path)
	if err != nil {
		t.Fatalf("captureProxySnapshotAt() error = %v, want nil for missing key", err)
	}
	if snapshot.Enabled || snapshot.HasServer || snapshot.Server != "" {
		t.Fatalf("snapshot = %+v, want empty snapshot for missing key", snapshot)
	}
}

func TestCaptureProxySnapshotPreservesFullServerValue(t *testing.T) {
	path := createTestProxyKey(t)
	key := openTestProxyKey(t, path)
	if err := key.SetDWordValue("ProxyEnable", 0); err != nil {
		t.Fatal(err)
	}
	const value = "http=corp:8080; https=127.0.0.1:2023"
	if err := key.SetStringValue("ProxyServer", value); err != nil {
		t.Fatal(err)
	}

	snapshot, err := captureProxySnapshotAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Enabled || !snapshot.HasServer || snapshot.Server != value {
		t.Fatalf("snapshot = %+v, want disabled proxy with full server value", snapshot)
	}
}

func TestCaptureProxySnapshotReadsEnabledFlag(t *testing.T) {
	path := createTestProxyKey(t)
	key := openTestProxyKey(t, path)
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		t.Fatal(err)
	}
	snapshot, err := captureProxySnapshotAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Enabled || snapshot.HasServer {
		t.Fatalf("snapshot = %+v, want enabled without server", snapshot)
	}
}

func TestRestoreProxySnapshotWritesAndClearsServer(t *testing.T) {
	path := createTestProxyKey(t)
	const value = "http=corp:8080; https=127.0.0.1:2023"
	if err := restoreProxySnapshotAt(path, &ProxySnapshot{Enabled: true, HasServer: true, Server: value}); err != nil {
		t.Fatal(err)
	}
	key := openTestProxyKey(t, path)
	if got, _, err := key.GetIntegerValue("ProxyEnable"); err != nil || got != 1 {
		t.Fatalf("ProxyEnable = %d, %v; want 1", got, err)
	}
	if got, _, err := key.GetStringValue("ProxyServer"); err != nil || got != value {
		t.Fatalf("ProxyServer = %q, %v; want %q", got, err, value)
	}

	if err := restoreProxySnapshotAt(path, &ProxySnapshot{Enabled: false, HasServer: false}); err != nil {
		t.Fatal(err)
	}
	if got, _, err := key.GetIntegerValue("ProxyEnable"); err != nil || got != 0 {
		t.Fatalf("ProxyEnable = %d, %v; want 0", got, err)
	}
	if _, _, err := key.GetStringValue("ProxyServer"); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("ProxyServer still present after restore: err = %v", err)
	}
}

func TestRestoreNilSnapshotOnlyDisablesProxy(t *testing.T) {
	path := createTestProxyKey(t)
	key := openTestProxyKey(t, path)
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		t.Fatal(err)
	}
	const value = "http=corp:8080"
	if err := key.SetStringValue("ProxyServer", value); err != nil {
		t.Fatal(err)
	}

	if err := restoreProxySnapshotAt(path, nil); err != nil {
		t.Fatal(err)
	}
	if got, _, err := key.GetIntegerValue("ProxyEnable"); err != nil || got != 0 {
		t.Fatalf("ProxyEnable = %d, %v; want 0", got, err)
	}
	if got, _, err := key.GetStringValue("ProxyServer"); err != nil || got != value {
		t.Fatalf("ProxyServer changed on nil snapshot: %q, %v", got, err)
	}
}

func TestFetchCurrentProxyReadsPerSchemeValue(t *testing.T) {
	path := createTestProxyKey(t)
	key := openTestProxyKey(t, path)
	if err := key.SetDWordValue("ProxyEnable", 1); err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue("ProxyServer", "http=corp:8080; https=127.0.0.1:2023"); err != nil {
		t.Fatal(err)
	}
	proxy, err := fetchCurrentProxyAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if proxy == nil || proxy.Hostname != "corp" || proxy.Port != "8080" {
		t.Fatalf("fetchCurrentProxyAt() = %+v, want corp:8080", proxy)
	}
}

func TestFetchCurrentProxyTreatsMissingValuesAsNotConfigured(t *testing.T) {
	path := createTestProxyKey(t)
	proxy, err := fetchCurrentProxyAt(path)
	if err != nil || proxy != nil {
		t.Fatalf("fetchCurrentProxyAt() = (%+v, %v), want (nil, nil)", proxy, err)
	}
}
