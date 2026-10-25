package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wx_channel/internal/config"
	"wx_channel/internal/minidownload"
)

func TestConfiguredFFmpegPathKeepsExplicitConfig(t *testing.T) {
	got := configuredFFmpegPath(`D:\tools\ffmpeg.exe`)
	if got != `D:\tools\ffmpeg.exe` {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestConfiguredFFmpegPathLeavesEmptyConfigUnmanaged(t *testing.T) {
	got := configuredFFmpegPath("")
	if got != "" {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestConfiguredFFmpegPathTreatsLegacyDefaultAsUnmanaged(t *testing.T) {
	if got := configuredFFmpegPath("ffmpeg"); got != "" {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestLocateFFmpegPriority(t *testing.T) {
	root := t.TempDir()
	configured := writeFFmpegFile(t, filepath.Join(root, "configured", "ffmpeg.exe"))
	bundled := writeFFmpegFile(t, filepath.Join(root, "package", "ffmpeg", "ffmpeg.exe"))
	managed := writeFFmpegFile(t, filepath.Join(root, "downloads", "ffmpeg.exe"))
	cases := []struct {
		name       string
		configured string
		bundled    string
		managed    string
		wantPath   string
		wantSource string
	}{
		{"explicit config wins", configured, bundled, managed, configured, ffmpegSourceConfigured},
		{"bundled before managed", "", bundled, managed, bundled, ffmpegSourceBundled},
		{"managed fallback", "", "", managed, managed, ffmpegSourceManaged},
		{"nothing available", "", "", "", "", ""},
		{"broken explicit config hides fallbacks", filepath.Join(root, "missing", "ffmpeg.exe"), bundled, managed, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubFFmpegLocations(t, tc.bundled, tc.managed)
			r := newTestRuntime(t, config.AppSettings{DownloadDir: filepath.Join(root, "downloads"), FFmpegPath: tc.configured})
			path, source := r.locateFFmpeg()
			if path != tc.wantPath || source != tc.wantSource {
				t.Fatalf("locateFFmpeg() = (%q, %q), want (%q, %q)", path, source, tc.wantPath, tc.wantSource)
			}
		})
	}
}

func TestHasFFmpegSelectsPathForDownloader(t *testing.T) {
	root := t.TempDir()
	bundled := writeFFmpegFile(t, filepath.Join(root, "package", "ffmpeg", "ffmpeg.exe"))
	managed := writeFFmpegFile(t, filepath.Join(root, "downloads", "ffmpeg.exe"))
	for _, tc := range []struct {
		name     string
		bundled  string
		managed  string
		want     string
		wantPath bool
	}{
		{"bundled", bundled, managed, bundled, true},
		{"managed", "", managed, managed, true},
		{"none", "", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubFFmpegLocations(t, tc.bundled, tc.managed)
			r := newTestRuntime(t, config.AppSettings{DownloadDir: filepath.Join(root, "downloads")})
			if got := r.HasFFmpeg(); got != tc.wantPath {
				t.Fatalf("HasFFmpeg() = %v, want %v", got, tc.wantPath)
			}
			if r.Downloader.FFmpegPath != tc.want {
				t.Fatalf("Downloader.FFmpegPath = %q, want %q", r.Downloader.FFmpegPath, tc.want)
			}
		})
	}
}

func TestEnsureFFmpegPrefersBundledWithoutDownloading(t *testing.T) {
	root := t.TempDir()
	bundled := writeFFmpegFile(t, filepath.Join(root, "package", "ffmpeg", "ffmpeg.exe"))
	managed := writeFFmpegFile(t, filepath.Join(root, "downloads", "ffmpeg.exe"))
	stubFFmpegLocations(t, bundled, managed)
	r := newTestRuntime(t, config.AppSettings{DownloadDir: filepath.Join(root, "downloads")})

	var states []minidownload.FFmpegState
	err := r.EnsureFFmpeg(context.Background(), func(state minidownload.FFmpegState) {
		states = append(states, state)
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Downloader.FFmpegPath != bundled {
		t.Fatalf("Downloader.FFmpegPath = %q, want bundled %q", r.Downloader.FFmpegPath, bundled)
	}
	if len(states) != 1 || states[0].State != "found" || states[0].Path != bundled {
		t.Fatalf("progress states = %+v, want single found state", states)
	}
}

func TestEnsureFFmpegRejectsBrokenConfiguredPath(t *testing.T) {
	root := t.TempDir()
	bundled := writeFFmpegFile(t, filepath.Join(root, "package", "ffmpeg", "ffmpeg.exe"))
	stubFFmpegLocations(t, bundled, "")
	r := newTestRuntime(t, config.AppSettings{
		DownloadDir: filepath.Join(root, "downloads"),
		FFmpegPath:  filepath.Join(root, "missing", "ffmpeg.exe"),
	})

	err := r.EnsureFFmpeg(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "download.ffmpeg") {
		t.Fatalf("EnsureFFmpeg() error = %v, want actionable configuration error", err)
	}
	if r.Downloader.FFmpegPath != "" {
		t.Fatalf("Downloader.FFmpegPath = %q, want no fallback after broken explicit config", r.Downloader.FFmpegPath)
	}
}

func TestPreflightChecksDescribeFFmpegSource(t *testing.T) {
	root := t.TempDir()
	bundled := writeFFmpegFile(t, filepath.Join(root, "package", "ffmpeg", "ffmpeg.exe"))
	for _, tc := range []struct {
		name       string
		bundled    string
		configured string
		want       string
	}{
		{"bundled", bundled, "", "ffmpeg 可用（随包）"},
		{"download later", "", "", "随包无 ffmpeg"},
		{"broken config", bundled, filepath.Join(root, "missing", "ffmpeg.exe"), "配置的 download.ffmpeg 不可用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubFFmpegLocations(t, tc.bundled, "")
			r := newTestRuntime(t, config.AppSettings{
				DownloadDir:   filepath.Join(root, strings.ReplaceAll(tc.name, " ", "-")),
				FFmpegPath:    tc.configured,
				ProxyHostname: "127.0.0.1",
				ProxyPort:     0,
			})
			checks := strings.Join(r.PreflightChecks(), "\n")
			if !strings.Contains(checks, tc.want) {
				t.Fatalf("PreflightChecks() = %q, want %q", checks, tc.want)
			}
		})
	}
}

func TestDownloadDirForTargetPrefersExistingTargetDir(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "wx123")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := newTestRuntime(t, config.AppSettings{DownloadDir: root})

	if got := r.downloadDirForTarget("wx123"); got != targetDir {
		t.Fatalf("downloadDirForTarget() = %q, want %q", got, targetDir)
	}
	if got := r.downloadDirForTarget("  "); got != root {
		t.Fatalf("empty target dir = %q, want download root", got)
	}
	if got := r.downloadDirForTarget("wx-missing"); got != root {
		t.Fatalf("missing target dir = %q, want download root", got)
	}
	if got, want := r.downloadDirForTarget("CON"), root; got != want {
		t.Fatalf("reserved target dir = %q, want %q", got, want)
	}
	if err := os.MkdirAll(filepath.Join(root, "_CON"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := r.downloadDirForTarget("CON"), filepath.Join(root, "_CON"); got != want {
		t.Fatalf("sanitized target dir = %q, want %q", got, want)
	}
}

func newTestRuntime(t *testing.T, settings config.AppSettings) *Runtime {
	t.Helper()
	return &Runtime{
		Settings:   settings,
		Downloader: minidownload.New(settings.DownloadDir),
	}
}

func writeFFmpegFile(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubFFmpegLocations(t *testing.T, bundled, managed string) {
	t.Helper()
	oldBundled, oldManaged := bundledFFmpegPath, managedFFmpegPath
	bundledFFmpegPath = func() string { return bundled }
	managedFFmpegPath = func(string) string { return managed }
	t.Cleanup(func() {
		bundledFFmpegPath, managedFFmpegPath = oldBundled, oldManaged
	})
}
