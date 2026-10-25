package minidownload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wx_channel/internal/miniprogram"
)

func TestDirectDownloadNeverMixesSameNamedResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"entity"`)
		w.Header().Set("Content-Length", "6")
		if strings.HasPrefix(r.URL.Path, "/a/") {
			w.Write([]byte("AAA"))
			return
		}
		if r.Header.Get("Range") != "" {
			t.Error("new resource reused another resource's partial data")
		}
		w.Write([]byte("BBBBBB"))
	}))
	defer server.Close()
	d := New(t.TempDir())
	a := miniprogram.Candidate{ID: "a", URL: server.URL + "/a/video.mp4", Kind: "video"}
	firstOutput, err := d.Download(context.Background(), a, "", "")
	if err == nil {
		t.Fatal("expected interrupted first resource")
	}
	b := miniprogram.Candidate{ID: "b", URL: server.URL + "/b/video.mp4", Kind: "video"}
	output, err := d.Download(context.Background(), b, "", "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "BBBBBB" {
		t.Fatalf("mixed media: %q, %v", data, err)
	}
	if got := fileSize(partialPath(firstOutput, a)); got != 3 {
		t.Fatal("another resource's partial file was lost")
	}
}

func TestPartialLengthIsNotProofOfCompletion(t *testing.T) {
	for _, validator := range []bool{true, false} {
		t.Run(fmt.Sprint(validator), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if validator {
					w.Header().Set("ETag", `"original"`)
				} else {
					w.Header().Set("Last-Modified", "Mon, 19 Oct 2026 00:00:00 GMT")
				}
				if requests == 1 {
					w.Header().Set("Content-Length", "6")
					w.Write([]byte("AAA"))
					return
				}
				if validator {
					if r.Header.Get("If-Range") != `"original"` || r.Header.Get("Range") != "bytes=3-" {
						t.Error("missing entity validation")
					}
					w.Header().Set("Content-Range", "bytes 3-5/6")
					w.WriteHeader(http.StatusPartialContent)
					w.Write([]byte("BBB"))
				} else {
					if r.Header.Get("Range") != "" {
						t.Error("unverifiable entity was resumed")
					}
					w.Write([]byte("AAABBB"))
				}
			}))
			defer server.Close()
			d := New(t.TempDir())
			stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
			candidate := miniprogram.Candidate{ID: "segmented", URL: server.URL + "/video.mp4", Kind: "video", ContentLength: 3}
			if _, err := d.Download(context.Background(), candidate, "", ""); err == nil {
				t.Fatal("accepted truncated response")
			}
			output, err := d.Download(context.Background(), candidate, "", "")
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil || string(data) != "AAABBB" || requests != 2 {
				t.Fatalf("premature completion: %q, requests=%d, err=%v", data, requests, err)
			}
		})
	}
}

func TestDirectResumeRestartsChangedEntity(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("ETag", `"old"`)
			w.Header().Set("Content-Length", "6")
			w.Write([]byte("AAA"))
			return
		}
		if r.Header.Get("If-Range") != `"old"` {
			t.Error("old entity was not conditional")
		}
		w.Header().Set("ETag", `"new"`)
		w.Write([]byte("BBBBBB"))
	}))
	defer server.Close()
	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{URL: server.URL + "/video.mp4", Kind: "video"}
	d.Download(context.Background(), candidate, "", "")
	output, err := d.Download(context.Background(), candidate, "", "")
	data, readErr := os.ReadFile(output)
	if err != nil || readErr != nil || string(data) != "BBBBBB" {
		t.Fatalf("changed entity was appended: %q, %v, %v", data, err, readErr)
	}
}

func TestDownloadRedirectDropsCredentialsAtOriginBoundary(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, key := range []string{"Authorization", "Cookie", "X-API-Key"} {
			if r.Header.Get(key) != "" {
				t.Errorf("redirect leaked %s", key)
			}
		}
		w.Write([]byte("media"))
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/video.mp4", http.StatusFound)
	}))
	defer source.Close()
	d := New(t.TempDir())
	_, err := d.Download(context.Background(), miniprogram.Candidate{URL: source.URL + "/video.mp4", Kind: "video", Headers: map[string]string{"Authorization": "secret", "Cookie": "secret", "X-API-Key": "secret"}}, "", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestPlaylistRelayScopesHeadersAndRewritesNestedPlaylists(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
			t.Error("playlist leaked credentials across origins")
		}
		w.Write([]byte("foreign-media"))
	}))
	defer foreign.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "secret" || r.Header.Get("Cookie") != "session" {
			http.Error(w, "missing auth", http.StatusForbidden)
			return
		}
		w.Write([]byte("#EXTM3U\n#EXTINF:1,\n" + foreign.URL + "/seg.ts\n#EXT-X-ENDLIST\n"))
	}))
	defer source.Close()
	d := New(t.TempDir())
	input, closeRelay, err := d.servePlaylist(context.Background(), miniprogram.Candidate{URL: source.URL + "/master.m3u8", Headers: map[string]string{"Authorization": "secret", "Cookie": "session", "X-API-Key": "secret"}}, []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nchild.m3u8\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeRelay()
	fetch := func(address string) string {
		t.Helper()
		resp, err := http.Get(address)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("relay failed: %d, %v", resp.StatusCode, err)
		}
		return string(data)
	}
	root := fetch(input)
	childURL := mediaLine(root)
	child := fetch(childURL)
	if got := fetch(mediaLine(child)); got != "foreign-media" {
		t.Fatalf("nested relay: %q", got)
	}
	resp, err := http.Get(input + "/unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatal("relay exposed an unregistered resource")
	}
	closeRelay()
	if resp, err := http.Get(input); err == nil {
		resp.Body.Close()
		t.Fatal("playlist listener survived cleanup")
	}
}

func TestRemotePlaylistRejectsLocalFilesAndPreservesInlineKeys(t *testing.T) {
	for _, body := range []string{"#EXTM3U\nfile:///C:/private.txt\n", "#EXTM3U\n#EXT-X-KEY:URI=\"file:///C:/private.key\"\n"} {
		if _, err := rewriteRemotePlaylist([]byte(body), "https://media.example.com/live.m3u8", nil); err == nil {
			t.Fatal("remote playlist gained local file access")
		}
	}
	body, err := rewriteRemotePlaylist([]byte("#EXTM3U\n#EXT-X-KEY:URI=\"data:text/plain;base64,AAAA\"\nseg.ts\n"), "https://media.example.com/live.m3u8", nil)
	if err != nil || !strings.Contains(string(body), "data:text/plain;base64,AAAA") {
		t.Fatal("inline key was lost")
	}
}

func TestPrefetchDoesNotReuseDifferentPlaylistSegmentsByPosition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(r.URL.Path)) }))
	defer server.Close()
	d := New(t.TempDir())
	bucket := testBucket(d)
	first := []byte("#EXTM3U\n#EXTINF:1,\n" + server.URL + "/old.ts\n")
	_, old, err := d.localizeM3U8(context.Background(), bucket, "same-stream", first, nil, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	second := []byte("#EXTM3U\n#EXTINF:1,\n" + server.URL + "/new.ts\n")
	_, fresh, err := d.localizeM3U8(context.Background(), bucket, "same-stream", second, nil, server.URL)
	if err != nil || fresh.Reused != 0 || old.Dir == fresh.Dir {
		t.Fatalf("reused stale playlist: %+v, %v", fresh, err)
	}
	data, err := os.ReadFile(filepath.Join(fresh.Dir, "seg_000001.ts"))
	if err != nil || string(data) != "/new.ts" {
		t.Fatalf("wrong media segment: %q, %v", data, err)
	}
}

func mediaLine(playlist string) string {
	for _, line := range strings.Split(playlist, "\n") {
		if strings.HasPrefix(line, "http://") {
			return line
		}
	}
	return ""
}

func writeSegmentDone(t *testing.T, path, rawURL string, size int64) {
	t.Helper()
	data, err := json.Marshal(segmentMetadata{URL: rawURL, Size: size, CompletedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(segmentDonePath(path), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLocalizeM3U8RedownloadsSegmentWithoutCompletionMetadata(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("fresh"))
	}))
	defer server.Close()
	d := New(t.TempDir())
	d.Client = server.Client()
	body := []byte("#EXTM3U\n#EXTINF:1,\n" + server.URL + "/seg.ts\n")
	bucket := testBucket(d)
	dir := filepath.Join(bucket, ".segments", "legacy", miniprogram.CandidateID("", string(body)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "seg_000001.ts")
	if err := os.WriteFile(outputPath, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stats, err := d.localizeM3U8(context.Background(), bucket, "legacy", body, nil, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want legacy file without metadata re-downloaded", requests)
	}
	if stats.Reused != 0 {
		t.Fatalf("stats.Reused = %d, want 0 for a file without metadata", stats.Reused)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil || string(data) != "fresh" {
		t.Fatalf("segment = %q, %v; want fresh download", data, err)
	}
}

func TestLocalizeM3U8IgnoresTruncatedPartialFile(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("fresh"))
	}))
	defer server.Close()
	d := New(t.TempDir())
	d.Client = server.Client()
	body := []byte("#EXTM3U\n#EXTINF:1,\n" + server.URL + "/seg.ts\n")
	bucket := testBucket(d)
	dir := filepath.Join(bucket, ".segments", "truncated", miniprogram.CandidateID("", string(body)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "seg_000001.ts")
	if err := os.WriteFile(segmentPartPath(outputPath), []byte("trunc"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stats, err := d.localizeM3U8(context.Background(), bucket, "truncated", body, nil, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || stats.Reused != 0 {
		t.Fatalf("requests = %d, reused = %d; want truncated .part ignored", requests, stats.Reused)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil || string(data) != "fresh" {
		t.Fatalf("segment = %q, %v; want fresh download", data, err)
	}
	if fileExists(segmentPartPath(outputPath)) {
		t.Fatal("truncated .part file survived the retry")
	}
}

func TestLocalizeM3U8RedownloadsWhenCompletionMetadataDoesNotMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fresh"))
	}))
	defer server.Close()
	segmentURL := server.URL + "/seg.ts"
	body := []byte("#EXTM3U\n#EXTINF:1,\n" + segmentURL + "\n")
	cases := []struct {
		name string
		meta segmentMetadata
	}{
		{name: "url", meta: segmentMetadata{URL: "https://other.example/seg.ts", Size: 5}},
		{name: "size", meta: segmentMetadata{URL: segmentURL, Size: 6}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := New(t.TempDir())
			d.Client = server.Client()
			bucket := testBucket(d)
			dir := filepath.Join(bucket, ".segments", "metadata", miniprogram.CandidateID("", string(body)))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			outputPath := filepath.Join(dir, "seg_000001.ts")
			if err := os.WriteFile(outputPath, []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(tc.meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(segmentDonePath(outputPath), data, 0o600); err != nil {
				t.Fatal(err)
			}

			_, stats, err := d.localizeM3U8(context.Background(), bucket, "metadata", body, nil, server.URL)
			if err != nil {
				t.Fatal(err)
			}
			if stats.Reused != 0 {
				t.Fatalf("stats.Reused = %d, want metadata mismatch re-downloaded", stats.Reused)
			}
			got, err := os.ReadFile(outputPath)
			if err != nil || string(got) != "fresh" {
				t.Fatalf("segment = %q, %v; want fresh download", got, err)
			}
		})
	}
}

func TestLocalizeM3U8RejectsContentLengthMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = w.Write([]byte("abc"))
	}))
	defer server.Close()
	d := New(t.TempDir())
	d.Client = server.Client()
	d.SegmentRetries = 1
	body := []byte("#EXTM3U\n#EXTINF:1,\n" + server.URL + "/short.ts\n")

	_, stats, err := d.localizeM3U8(context.Background(), testBucket(d), "short", body, nil, server.URL)
	if err == nil {
		t.Fatal("accepted a segment shorter than its Content-Length")
	}
	if !strings.Contains(err.Error(), "Content-Length") {
		t.Fatalf("error = %v, want Content-Length mismatch", err)
	}
	outputPath := filepath.Join(stats.Dir, "seg_000001.ts")
	if fileExists(outputPath) {
		t.Fatal("truncated segment was promoted to the final file")
	}
	if fileExists(segmentPartPath(outputPath)) {
		t.Fatal("truncated segment left a .part file behind")
	}
}

func TestAuthenticatedHLSWithRealFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed; relay covered independently")
	}
	fixture := filepath.Join(t.TempDir(), "sample.ts")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.3", "-c:a", "aac", "-f", "mpegts", fixture).CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v, %s", err, output)
	}
	segment, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"remote-ffmpeg", "prefetch", "master"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if (r.URL.Path != "/seg.ts" && r.URL.Path != "/child.m3u8") || r.URL.Query().Get("auth") != "playlist-signature" {
					http.Error(w, "expired playlist or missing signature", 410)
					return
				}
				if r.Header.Get("Cookie") != "local-session" || r.Header.Get("Referer") != "https://servicewechat.com/wx123/1/page-frame.html" {
					http.Error(w, "auth required", 403)
					return
				}
				requests++
				if r.URL.Path == "/child.m3u8" {
					w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
					http.ServeContent(w, r, "child.m3u8", time.Time{}, strings.NewReader("#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:0.3,\nseg.ts\n#EXT-X-ENDLIST\n"))
					return
				}
				w.Header().Set("Content-Type", "video/mp2t")
				w.Write(segment)
			}))
			defer server.Close()
			d := New(t.TempDir())
			d.FFmpegPath = ffmpeg
			d.DownloadMode = mode
			playlist := filepath.Join(d.Dir, "captured.m3u8")
			body := "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXTINF:0.3,\nseg.ts\n#EXT-X-ENDLIST\n"
			if mode == "master" {
				d.DownloadMode = "prefetch"
				body = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=64000\nchild.m3u8\n"
			}
			if err := os.WriteFile(playlist, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := d.Download(ctx, miniprogram.Candidate{ID: "authenticated", Kind: "m3u8", URL: server.URL + "/expired.m3u8?auth=playlist-signature", CachedPath: playlist, Headers: map[string]string{"Cookie": "local-session", "Referer": "https://servicewechat.com/wx123/1/page-frame.html"}}, "result", "")
			if err != nil {
				t.Fatal(err)
			}
			if requests == 0 || fileSize(output) == 0 {
				t.Fatal("no authenticated segment was merged")
			}
		})
	}
}
