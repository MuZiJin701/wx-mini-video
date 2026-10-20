package config

import (
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"testing"
	"wx_channel/pkg/certificate"
)

func TestExplicitCertificatesNeverFallBackAndTakePriority(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	cert, err := certificate.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "custom.pem"), filepath.Join(dir, "custom.key")
	if err := os.WriteFile(certPath, cert.Cert, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, cert.PrivateKey, 0o600); err != nil {
		t.Fatal(err)
	}
	viper.Set("cert.file", certPath)
	if _, err := LoadCertFiles(); err == nil {
		t.Fatal("incomplete configuration silently fell back")
	}
	viper.Set("cert.key", filepath.Join(dir, "missing.key"))
	if _, err := LoadCertFiles(); err == nil {
		t.Fatal("missing explicit private key silently fell back")
	}
	viper.Set("cert.key", keyPath)
	got, err := LoadCertFiles()
	if err != nil || string(got.Cert) != string(cert.Cert) {
		t.Fatalf("explicit root not selected: %v", err)
	}
}

func TestSettingsReadsTargetFromConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wx-mini-video.yaml")
	if err := os.WriteFile(configPath, []byte("target:\n  appID: \"wx123\"\n  name: \"测试小程序\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigPath, configPath)

	cfg := New("test", "debug")
	if err := cfg.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	settings := cfg.Settings()
	if settings.Target.AppID != "wx123" {
		t.Fatalf("AppID = %q", settings.Target.AppID)
	}
	if settings.Target.Name != "测试小程序" {
		t.Fatalf("Name = %q", settings.Target.Name)
	}
}

func TestSettingsAllowsEmptyTargetName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wx-mini-video.yaml")
	if err := os.WriteFile(configPath, []byte("target:\n  appID: \"wx123\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigPath, configPath)

	cfg := New("test", "debug")
	if err := cfg.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	settings := cfg.Settings()
	if settings.Target.AppID != "wx123" {
		t.Fatalf("AppID = %q", settings.Target.AppID)
	}
	if settings.Target.Name != "" {
		t.Fatalf("Name = %q", settings.Target.Name)
	}
}

func TestSettingsUsesPortableDirectoryForDefaultDownloads(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wx-mini-video.yaml")
	if err := os.WriteFile(configPath, []byte("download:\n  dir: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigPath, configPath)

	cfg := New("test", "debug")
	if err := cfg.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Settings().DownloadDir, filepath.Join(dir, "downloads"); got != want {
		t.Fatalf("DownloadDir = %q, want portable directory %q", got, want)
	}
}

func TestSettingsKeepsExplicitDownloadDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "wx-mini-video.yaml")
	if err := os.WriteFile(configPath, []byte("download:\n  dir: D:/media\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfigPath, configPath)

	cfg := New("test", "debug")
	if err := cfg.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Settings().DownloadDir; got != "D:/media" {
		t.Fatalf("DownloadDir = %q, want explicit directory", got)
	}
}

func TestMacBundleKeepsConfigAndDownloadsOutsideSignedApplication(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "wx-mini-video.app", "Contents", "MacOS")
	if got := portableConfigDirectory(bin); got != root {
		t.Fatalf("portable root = %q, want %q", got, root)
	}
	if got := resolveDownloadDir("", portableConfigDirectory(bin)); got != filepath.Join(root, "downloads") {
		t.Fatal("downloads would modify the signed application")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := certificateBaseDir("darwin"); err != nil || got != filepath.Join(home, "Library", "Application Support") {
		t.Fatalf("CA directory = %q, %v", got, err)
	}
}
