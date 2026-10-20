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
	"path/filepath"
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

func TestMacFFmpegSourcesArePinnedAndArchitectureSpecific(t *testing.T) {
	intelURL, intelSHA := ffmpegSourceForPlatform("darwin", "amd64")
	armURL, armSHA := ffmpegSourceForPlatform("darwin", "arm64")
	if !strings.Contains(intelURL, "/macos/amd64/") || !strings.Contains(armURL, "/macos/arm64/") || strings.Contains(armURL, "latest") {
		t.Fatal("incorrect or floating Mac sources")
	}
	if len(intelSHA) != 64 || len(armSHA) != 64 || intelSHA == armSHA {
		t.Fatal("missing independent archive checksums")
	}
	if url, _ := ffmpegSourceForPlatform("darwin", "386"); url != "" {
		t.Fatal("unsupported architecture would download the wrong executable")
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
	if err := os.WriteFile(filepath.Join(dir, managedFFmpegMarker), []byte(managedFFmpegSHA256+"\n"+sum+"\n"), 0o644); err != nil {
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

func TestEnsureFFmpegStopsAfterThreeTransientFailures(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "temporary", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	setManagedFFmpegSource(t, server.URL, managedFFmpegSHA256)

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
			sum := managedFFmpegSHA256
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

func setManagedFFmpegSource(t *testing.T, url, sum string) {
	t.Helper()
	oldURL, oldSum := managedFFmpegURL, managedFFmpegSHA256
	managedFFmpegURL, managedFFmpegSHA256 = url, sum
	t.Cleanup(func() {
		managedFFmpegURL, managedFFmpegSHA256 = oldURL, oldSum
	})
}
