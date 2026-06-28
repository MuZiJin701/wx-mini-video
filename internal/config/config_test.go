package config

import (
	"os"
	"path/filepath"
	"testing"
)

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
