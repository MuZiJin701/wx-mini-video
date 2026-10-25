package minidownload

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCanAutoInstallFFmpegOnSupportedPlatforms(t *testing.T) {
	if !canAutoInstallFFmpeg("windows") {
		t.Fatal("windows should support automatic ffmpeg install")
	}
	if !canAutoInstallFFmpeg("darwin") {
		t.Fatal("darwin should support native automatic ffmpeg install")
	}
	if canAutoInstallFFmpeg("linux") {
		t.Fatal("linux should not download windows ffmpeg.exe")
	}
}

func TestWindowsFFmpegSourcesPreferGyanThenBtbNFallback(t *testing.T) {
	sources := ffmpegSourcesForPlatform("windows", "amd64")
	if len(sources) != 2 {
		t.Fatalf("windows sources = %d, want primary + fallback", len(sources))
	}
	if !strings.Contains(sources[0].URL, "gyan.dev") || !strings.Contains(sources[0].URL, "9.0.2") {
		t.Fatalf("primary source = %q, want pinned gyan 9.0.2", sources[0].URL)
	}
	if !strings.Contains(sources[1].URL, "ffmpeg-n8.1.3-9-g29e619e767-win64-lgpl-8.1.zip") {
		t.Fatalf("fallback source = %q, want pinned BtbN month-end build", sources[1].URL)
	}
	for _, source := range sources {
		if len(source.SHA256) != 64 {
			t.Fatalf("source %q has checksum %q", source.URL, source.SHA256)
		}
	}
	if sources[0].SHA256 == sources[1].SHA256 {
		t.Fatal("candidate sources must pin independent checksums")
	}
}

func TestMacFFmpegSourcesArePinnedAndArchitectureSpecific(t *testing.T) {
	intel := ffmpegSourcesForPlatform("darwin", "amd64")
	arm := ffmpegSourcesForPlatform("darwin", "arm64")
	if len(intel) != 1 || len(arm) != 1 {
		t.Fatalf("mac sources = %d/%d, want one archive per architecture", len(intel), len(arm))
	}
	if !strings.Contains(intel[0].URL, "/macos/amd64/") || !strings.Contains(arm[0].URL, "/macos/arm64/") || strings.Contains(arm[0].URL, "latest") {
		t.Fatal("incorrect or floating Mac sources")
	}
	if len(intel[0].SHA256) != 64 || len(arm[0].SHA256) != 64 || intel[0].SHA256 == arm[0].SHA256 {
		t.Fatal("missing independent archive checksums")
	}
	if sources := ffmpegSourcesForPlatform("darwin", "386"); len(sources) != 0 {
		t.Fatal("unsupported architecture would download the wrong executable")
	}
	if sources := ffmpegSourcesForPlatform("linux", "amd64"); len(sources) != 0 {
		t.Fatal("linux has no supported automatic source")
	}
}

func TestMacFFmpegSourcesMatchPublicPlatformAccessor(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		got := FFmpegSourcesForPlatform("darwin", arch)
		want := ffmpegSourcesForPlatform("darwin", arch)
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("FFmpegSourcesForPlatform(darwin, %s) = %v, want %v", arch, got, want)
		}
	}
	if got := FFmpegSourcesForPlatform("linux", "amd64"); len(got) != 0 {
		t.Fatalf("FFmpegSourcesForPlatform(linux) = %v, want none", got)
	}
}

func TestBundledFFmpegCandidateLayouts(t *testing.T) {
	root := t.TempDir()
	appMacOS := filepath.Join(root, "wx-mini-video.app", "Contents", "MacOS")
	cases := []struct {
		name   string
		exeDir string
		goos   string
		arch   string
		want   string
	}{
		{"windows", filepath.Join(root, "win"), "windows", "amd64", filepath.Join(root, "win", "ffmpeg", "ffmpeg.exe")},
		{"darwin app amd64", appMacOS, "darwin", "amd64", filepath.Join(root, "ffmpeg", "ffmpeg-amd64")},
		{"darwin app arm64", appMacOS, "darwin", "arm64", filepath.Join(root, "ffmpeg", "ffmpeg-arm64")},
		{"darwin unpacked arm64", filepath.Join(root, "portable"), "darwin", "arm64", filepath.Join(root, "portable", "ffmpeg", "ffmpeg-arm64")},
		{"darwin unsupported arch", appMacOS, "darwin", "386", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bundledFFmpegCandidate(tc.exeDir, tc.goos, tc.arch); got != tc.want {
				t.Fatalf("bundledFFmpegCandidate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckedBundledFFmpegRejectsCorruptionAndAcceptsWorkingExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffmpeg.exe")
	if err := os.WriteFile(path, []byte("corrupt executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := checkedBundledFFmpeg(path); got != "" {
		t.Fatalf("corrupt bundled executable accepted: %q", got)
	}
	// 本机已有 ffmpeg 时验证成功探测及缓存；不复制可能依赖旁文件的系统启动 shim。
	installed, err := exec.LookPath("ffmpeg")
	if err != nil {
		return
	}
	if got := checkedBundledFFmpeg(installed); got != installed {
		t.Fatalf("working executable rejected: %q", got)
	}
	if got := checkedBundledFFmpeg(installed); got != installed {
		t.Fatal("cached usable executable lost")
	}
}

func TestBundledFFmpegPathRequiresExistingExecutable(t *testing.T) {
	root := t.TempDir()
	appMacOS := filepath.Join(root, "wx-mini-video.app", "Contents", "MacOS")
	if got := bundledFFmpegPath(appMacOS, "darwin", "arm64"); got != "" {
		t.Fatalf("missing bundled ffmpeg = %q, want empty", got)
	}
	path := filepath.Join(root, "ffmpeg", "ffmpeg-arm64")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := bundledFFmpegPath(appMacOS, "darwin", "arm64"); got != "" {
		t.Fatalf("bundled ffmpeg without exec bit = %q, want empty", got)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := bundledFFmpegPath(appMacOS, "darwin", "arm64"); got != path {
			t.Fatalf("bundled ffmpeg = %q, want %q", got, path)
		}
	}
	windowsDir := filepath.Join(root, "win")
	windowsExe := filepath.Join(windowsDir, "ffmpeg", "ffmpeg.exe")
	if err := os.MkdirAll(filepath.Dir(windowsExe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(windowsExe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := bundledFFmpegPath(windowsDir, "windows", "amd64"); got != windowsExe {
		t.Fatalf("windows bundled ffmpeg = %q, want %q", got, windowsExe)
	}
}

func TestBundledFFmpegPathAvoidsMissingPackageFiles(t *testing.T) {
	// 测试二进制位于 Go 临时构建目录，正常情况下旁边没有随包 ffmpeg。
	if got := BundledFFmpegPath(); got != "" {
		t.Fatalf("BundledFFmpegPath() = %q, want empty next to test binary", got)
	}
}

func TestManagedFFmpegPathRequiresVerifiedMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, managedFFmpegName())
	if err := os.WriteFile(path, []byte("ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != "" {
		t.Fatalf("ManagedFFmpegPath() = %q, want no unverified executable", got)
	}
	sum, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, managedFFmpegMarker), []byte(managedFFmpegArchiveSHA(t)+"\n"+sum+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != path {
		t.Fatalf("ManagedFFmpegPath() = %q, want %q", got, path)
	}
	if err := os.WriteFile(path, []byte("different"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != "" {
		t.Fatalf("ManagedFFmpegPath() = %q, want no modified executable", got)
	}
}

func TestManagedFFmpegPathRejectsUnpinnedArchiveMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, managedFFmpegName())
	if err := os.WriteFile(path, []byte("ffmpeg"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := strings.Repeat("0", 64) + "\n" + sum + "\n"
	if err := os.WriteFile(filepath.Join(dir, managedFFmpegMarker), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ManagedFFmpegPath(dir); got != "" {
		t.Fatalf("ManagedFFmpegPath() = %q, want marker from unknown archive rejected", got)
	}
}

func TestNewDownloaderDoesNotDefaultToPathFFmpeg(t *testing.T) {
	if got := New(t.TempDir()).FFmpegPath; got != "" {
		t.Fatalf("FFmpegPath = %q, want no PATH default", got)
	}
}

func TestEnsureFFmpegRetriesServerFailure(t *testing.T) {
	archive, sum := testFFmpegArchive(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	setManagedFFmpegSource(t, server.URL, sum)

	path, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if got := filepath.Base(path); got != managedFFmpegName() {
		t.Fatalf("ffmpeg path = %q", path)
	}
}

func TestEnsureFFmpegFallsBackToNextSource(t *testing.T) {
	archive, sum := testFFmpegArchive(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.HasSuffix(r.URL.Path, "/missing") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	setManagedFFmpegSources(t,
		FFmpegSource{URL: server.URL + "/missing", SHA256: sum},
		FFmpegSource{URL: server.URL + "/primary", SHA256: sum},
	)

	path, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want one request per candidate source", requests)
	}
	if ManagedFFmpegPath(filepath.Dir(path)) != path {
		t.Fatal("marker written for fallback source was not accepted")
	}
}

func TestEnsureFFmpegSendsDownloadUserAgent(t *testing.T) {
	archive, sum := testFFmpegArchive(t)
	agents := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.UserAgent()
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	setManagedFFmpegSource(t, server.URL, sum)

	if _, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	if got := <-agents; got != ffmpegDownloadUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, ffmpegDownloadUserAgent)
	}
}

func TestEnsureFFmpegStopsAfterThreeTransientFailures(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	setManagedFFmpegSource(t, server.URL, managedFFmpegArchiveSHA(t))

	_, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "download.ffmpeg") {
		t.Fatalf("EnsureFFmpeg() error = %v, want actionable error", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestEnsureFFmpegDoesNotRetryClientOrChecksumFailure(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			archive, _ := testFFmpegArchive(t)
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if status == http.StatusNotFound {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(archive)
			}))
			defer server.Close()
			sum := managedFFmpegArchiveSHA(t)
			if status == http.StatusOK {
				sum = "invalid-checksum"
			}
			setManagedFFmpegSource(t, server.URL, sum)

			_, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil)
			if err == nil || !strings.Contains(err.Error(), "download.ffmpeg") {
				t.Fatalf("EnsureFFmpeg() error = %v, want actionable error", err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want 1", requests)
			}
		})
	}
}

func TestEnsureFFmpegExplainsRecoveryAfterExtractionFailure(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	setManagedFFmpegSource(t, server.URL, hex.EncodeToString(sum[:]))

	_, err := EnsureFFmpeg(context.Background(), t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "download.ffmpeg") {
		t.Fatalf("EnsureFFmpeg() error = %v, want recovery guidance", err)
	}
}

func testFFmpegArchive(t *testing.T) ([]byte, string) {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	entry, err := zw.Create("bin/" + managedFFmpegName())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("test ffmpeg")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(sum[:])
}

// managedFFmpegArchiveSHA 返回当前平台首个固定源的摘要，供标记测试使用。
func managedFFmpegArchiveSHA(t *testing.T) string {
	t.Helper()
	if len(managedFFmpegSources) == 0 {
		t.Skip("当前平台没有固定 ffmpeg 源")
	}
	return managedFFmpegSources[0].SHA256
}

func setManagedFFmpegSource(t *testing.T, url, sum string) {
	t.Helper()
	setManagedFFmpegSources(t, FFmpegSource{URL: url, SHA256: sum})
}

func setManagedFFmpegSources(t *testing.T, sources ...FFmpegSource) {
	t.Helper()
	old := managedFFmpegSources
	managedFFmpegSources = sources
	t.Cleanup(func() {
		managedFFmpegSources = old
	})
}
