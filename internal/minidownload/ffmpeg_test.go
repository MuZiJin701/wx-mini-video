package minidownload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanAutoInstallFFmpegOnlyOnWindows(t *testing.T) {
	if !canAutoInstallFFmpeg("windows") {
		t.Fatal("windows should support automatic ffmpeg install")
	}
	if canAutoInstallFFmpeg("darwin") {
		t.Fatal("darwin should not download windows ffmpeg.exe")
	}
	if canAutoInstallFFmpeg("linux") {
		t.Fatal("linux should not download windows ffmpeg.exe")
	}
}

func TestManagedFFmpegPathRequiresVerifiedMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg.exe")
	if err := os.WriteFile(path, []byte("ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != "" {
		t.Fatalf("ManagedFFmpegPath() = %q, want no unverified executable", got)
	}
	if err := os.WriteFile(filepath.Join(dir, managedFFmpegMarker), []byte(managedFFmpegSHA256+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != path {
		t.Fatalf("ManagedFFmpegPath() = %q, want %q", got, path)
	}
}

func TestNewDownloaderDoesNotDefaultToPathFFmpeg(t *testing.T) {
	if got := New(t.TempDir()).FFmpegPath; got != "" {
		t.Fatalf("FFmpegPath = %q, want no PATH default", got)
	}
}
