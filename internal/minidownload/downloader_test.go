package minidownload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wx_channel/internal/miniprogram"
)

func TestDownloadDirectResumesPartialAndWritesHistory(t *testing.T) {
	body := []byte(strings.Repeat("wx-mini-video", 1024))
	half := len(body) / 2
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body[:half])
			return
		}
		if got := r.Header.Get("Range"); got != "bytes="+strconv.Itoa(half)+"-" {
			t.Fatalf("Range = %q, want resume from %d", got, half)
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", half, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[half:])
	}))
	defer ts.Close()

	d := New(t.TempDir())
	candidate := miniprogram.Candidate{
		ID:            "resume-history",
		URL:           ts.URL + "/resume.mp4",
		Kind:          "video",
		Suffix:        ".mp4",
		ContentLength: int64(len(body)),
	}

	if _, err := d.Download(context.Background(), candidate, ""); err == nil {
		t.Fatal("first download should fail after the server closes the response early")
	}
	partPath := filepath.Join(d.Dir, "resume.mp4.part")
	if got := fileSize(partPath); got != int64(half) {
		t.Fatalf("partial size = %d, want %d", got, half)
	}

	outputPath, err := d.Download(context.Background(), candidate, "")
	if err != nil {
		t.Fatalf("resumed download failed: %v", err)
	}
	if requests != 2 {
		t.Fatalf("server requests = %d, want initial request plus resume", requests)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("downloaded body does not match original content")
	}
	if fileExists(partPath) {
		t.Fatalf("partial file %q should be renamed after completion", partPath)
	}

	historyBody, err := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(historyBody))), &record); err != nil {
		t.Fatalf("history record is not JSON: %v", err)
	}
	if record.Path != outputPath || record.SourceDomain != "127.0.0.1" || record.Size != int64(len(body)) {
		t.Fatalf("history record = %#v", record)
	}
	if record.CompletedAt.IsZero() || time.Since(record.CompletedAt) < 0 {
		t.Fatalf("history timestamp = %v, want recent UTC time", record.CompletedAt)
	}
}

func TestChooseM3U8ModeAutoUsesRemoteForShortPlaylist(t *testing.T) {
	short := []byte(`#EXTM3U
#EXTINF:10.000,
https://example.com/seg1.ts
#EXTINF:10.000,
https://example.com/seg2.ts
`)
	long := []byte(`#EXTM3U
` + strings.Repeat("#EXTINF:10.000,\nhttps://example.com/seg.ts\n", 80))

	d := New(t.TempDir())
	d.DownloadMode = "auto"
	if got := d.chooseM3U8Mode(short); got != "remote-ffmpeg" {
		t.Fatalf("short auto mode = %q, want remote-ffmpeg", got)
	}
	if got := d.chooseM3U8Mode(long); got != "prefetch" {
		t.Fatalf("long auto mode = %q, want prefetch", got)
	}
}

func TestLocalizeM3U8ReusesExistingSegments(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("fresh"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	d.Client = ts.Client()
	dir := filepath.Join(d.Dir, ".segments", "reuse")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "seg_000001.ts"), []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := []byte(`#EXTM3U
#EXTINF:10.000,
` + ts.URL + `/seg.ts
`)

	_, stats, err := d.localizeM3U8(context.Background(), "reuse", body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want cached segment reuse", requests)
	}
	if stats.Reused != 1 || stats.Completed != 1 {
		t.Fatalf("stats = %+v, want reused/completed segment", stats)
	}
}

func TestLocalizeM3U8ReportsFailedResources(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusInternalServerError)
	}))
	defer ts.Close()

	var progress []Progress
	d := New(t.TempDir())
	d.Client = ts.Client()
	d.SegmentRetries = 1
	d.OnProgress = func(p Progress) {
		progress = append(progress, p)
	}
	body := []byte(`#EXTM3U
#EXTINF:10.000,
` + ts.URL + `/bad.ts
`)

	_, stats, err := d.localizeM3U8(context.Background(), "failed-stats", body, nil)
	if err == nil {
		t.Fatal("expected failed resource")
	}
	if stats.Failed != 1 {
		t.Fatalf("failed = %d, want 1", stats.Failed)
	}
	last := progress[len(progress)-1]
	if last.Failed != 1 {
		t.Fatalf("progress failed = %d, want 1", last.Failed)
	}
}

func TestParseM3U8Duration(t *testing.T) {
	duration := parseM3U8Duration([]byte(`#EXTM3U
#EXTINF:10.500,
seg1.ts
#EXTINF:2,
seg2.ts
`))
	if duration != 12500 {
		t.Fatalf("duration = %dms, want 12500ms", duration)
	}
}

func TestParseFFmpegProgressTime(t *testing.T) {
	value, ok := parseFFmpegProgressTime("frame= 100 fps=0.0 q=-1.0 size=1024kB time=00:01:23.45 bitrate=100kbits/s")
	if !ok {
		t.Fatal("expected progress time")
	}
	if value != 83450 {
		t.Fatalf("progress = %dms, want 83450ms", value)
	}
}

func TestLocalizeM3U8DownloadsSegmentsAndResources(t *testing.T) {
	requests := map[string]int{}
	var requestsMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests[r.URL.Path]++
		count := requests[r.URL.Path]
		requestsMu.Unlock()
		if r.URL.Path == "/seg2.ts" && count == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("body-" + r.URL.Path))
	}))
	defer ts.Close()

	body := []byte(`#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="` + ts.URL + `/key.key"
#EXT-X-MAP:URI="` + ts.URL + `/init.mp4"
#EXTINF:10.000,
` + ts.URL + `/seg1.ts
#EXTINF:10.000,
` + ts.URL + `/seg2.ts
`)
	d := New(t.TempDir())
	d.Client = ts.Client()
	d.SegmentConcurrency = 2
	d.SegmentRetries = 2

	localPath, stats, err := d.localizeM3U8(context.Background(), "candidate", body, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, ts.URL) {
		t.Fatalf("expected local playlist without remote URLs, got:\n%s", text)
	}
	if !strings.Contains(text, "seg_000001.ts") || !strings.Contains(text, `URI="`) {
		t.Fatalf("expected local segment and URI references, got:\n%s", text)
	}
	if stats.Total != 4 || stats.Completed != 4 {
		t.Fatalf("stats = %+v, want 4 completed resources", stats)
	}
	requestsMu.Lock()
	seg2Requests := requests["/seg2.ts"]
	requestsMu.Unlock()
	if seg2Requests != 2 {
		t.Fatalf("seg2 requests = %d, want retry once", seg2Requests)
	}
}

func TestLocalizeM3U8KeepsSegmentsOnPartialFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.ts" {
			http.Error(w, "bad", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	body := []byte(`#EXTM3U
#EXTINF:10.000,
` + ts.URL + `/ok.ts
#EXTINF:10.000,
` + ts.URL + `/bad.ts
`)
	d := New(t.TempDir())
	d.Client = ts.Client()
	d.SegmentConcurrency = 2
	d.SegmentRetries = 1

	_, stats, err := d.localizeM3U8(context.Background(), "failed", body, nil)
	if err == nil {
		t.Fatal("expected partial failure")
	}
	if _, statErr := os.Stat(filepath.Join(stats.Dir, "seg_000001.ts")); statErr != nil {
		t.Fatalf("expected successful segment kept for retry/debugging: %v", statErr)
	}
}

func TestRewriteM3U8URLs(t *testing.T) {
	base := "https://venus-live.qmxdata.com/streams/5312_v.m3u8?auth_key=test"
	input := []byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:10.000,
../../../streams/5356_h/1781701095_335.ts
#EXTINF:10.000,
../../../streams/5356_h/1781701111_336.ts
#EXT-X-ENDLIST
`)
	result := string(rewriteM3U8URLs(input, base))

	if !strings.Contains(result, "https://venus-live.qmxdata.com/streams/5356_h/1781701095_335.ts") {
		t.Errorf("expected absolute URL for first segment, got:\n%s", result)
	}
	if !strings.Contains(result, "https://venus-live.qmxdata.com/streams/5356_h/1781701111_336.ts") {
		t.Errorf("expected absolute URL for second segment, got:\n%s", result)
	}
	if strings.Contains(result, "../../../") {
		t.Errorf("expected no relative paths, got:\n%s", result)
	}
	if !strings.Contains(result, "#EXTM3U") {
		t.Errorf("expected EXT tags preserved, got:\n%s", result)
	}
}

func TestRewriteM3U8URLsAbsolute(t *testing.T) {
	base := "https://example.com/playlist.m3u8"
	input := []byte(`#EXTM3U
#EXTINF:10.000,
https://cdn.example.com/seg1.ts
#EXTINF:10.000,
/absolute/seg2.ts
`)
	result := string(rewriteM3U8URLs(input, base))

	if !strings.Contains(result, "https://cdn.example.com/seg1.ts") {
		t.Errorf("expected absolute URL preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "https://example.com/absolute/seg2.ts") {
		t.Errorf("expected root-relative path resolved, got:\n%s", result)
	}
}

func TestRewriteM3U8TagURIAttributes(t *testing.T) {
	base := "https://example.com/live/path/index.m3u8?auth_key=abc"
	input := []byte(`#EXTM3U
#EXT-X-KEY:METHOD=AES-128,URI="key.key"
#EXT-X-MAP:URI="../init.mp4"
#EXTINF:10.000,
seg1.ts
`)
	result := string(rewriteM3U8URLs(input, base))
	if !strings.Contains(result, `URI="https://example.com/live/path/key.key?auth_key=abc"`) {
		t.Fatalf("expected key URI rewritten with auth query, got:\n%s", result)
	}
	if !strings.Contains(result, `URI="https://example.com/live/init.mp4?auth_key=abc"`) {
		t.Fatalf("expected map URI rewritten with auth query, got:\n%s", result)
	}
	if !strings.Contains(result, `https://example.com/live/path/seg1.ts?auth_key=abc`) {
		t.Fatalf("expected segment URI rewritten with auth query, got:\n%s", result)
	}
}

func TestBuildFFmpegArgsAllowsHTTPSegmentsFromLocalPlaylist(t *testing.T) {
	args := BuildFFmpegArgs(`D:\downloads\playlist.m3u8`, `D:\downloads\out.mp4`, nil, true)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "-protocol_whitelist file,http,https,tcp,tls,crypto,data") {
		t.Fatalf("expected protocol whitelist for local HLS playlist with remote segments, got: %v", args)
	}
}

func TestBuildFFmpegArgsOmitsHeadersForLocalPlaylist(t *testing.T) {
	args := BuildFFmpegArgs(`D:\downloads\playlist.m3u8`, `D:\downloads\out.mp4`, map[string]string{
		"User-Agent": "Mozilla/5.0",
	}, true)

	for _, arg := range args {
		if arg == "-headers" {
			t.Fatalf("local HLS playlist should not pass -headers as a file input option, got: %v", args)
		}
	}
}

func TestBuildFFmpegArgsOmitsHTTPOptionsForLocalPlaylist(t *testing.T) {
	args := BuildFFmpegArgs(`D:\downloads\playlist.m3u8`, `D:\downloads\out.mp4`, nil, true)

	for _, forbidden := range []string{"-reconnect", "-reconnect_streamed", "-reconnect_delay_max", "-http_persistent"} {
		for _, arg := range args {
			if arg == forbidden {
				t.Fatalf("local HLS playlist should not pass %s as a file input option, got: %v", forbidden, args)
			}
		}
	}
}

func TestCacheM3U8FromURLRetriesWithoutConditionalHeadersOn304(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	d.Client = &http.Client{}
	_, err := d.cacheM3U8FromURL(context.Background(), miniprogram.Candidate{
		ID:   "retry304",
		URL:  ts.URL + "/playlist.m3u8",
		Kind: "m3u8",
		Headers: map[string]string{
			"If-None-Match": "etag",
			"User-Agent":    "ua",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want retry after 304", requests)
	}
}

func TestCacheM3U8FromURL(t *testing.T) {
	m3u8Content := `#EXTM3U
#EXT-X-TARGETDURATION:10
#EXTINF:10.000,
seg1.ts
#EXTINF:10.000,
seg2.ts
#EXT-X-ENDLIST
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write([]byte(m3u8Content))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	d := New(tmpDir)
	d.Client = &http.Client{}

	result := rewriteM3U8URLs([]byte(m3u8Content), ts.URL+"/playlist.m3u8")
	resultStr := string(result)
	if !strings.Contains(resultStr, ts.URL+"/seg1.ts") {
		t.Errorf("expected absolute segment URL, got:\n%s", resultStr)
	}
	if !strings.Contains(resultStr, ts.URL+"/seg2.ts") {
		t.Errorf("expected absolute segment URL, got:\n%s", resultStr)
	}
	if strings.Contains(resultStr, "seg1.ts"+`"`) {
		// Just verify no relative paths remain
	}

	// Also test full download + cache flow
	localPath, err := d.cacheM3U8FromURL(context.Background(), miniprogram.Candidate{
		ID:      "test123",
		URL:     ts.URL + "/playlist.m3u8",
		Kind:    "m3u8",
		Headers: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(localPath)

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ts.URL+"/seg1.ts") {
		t.Errorf("cached m3u8 should contain absolute URL, got:\n%s", string(data))
	}
}

func TestDownloadM3U8FailsWithoutFFmpeg(t *testing.T) {
	dir := t.TempDir()
	playlist := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := New(dir)
	d.FFmpegPath = "nonexistent_ffmpeg_xxx"
	err := d.downloadM3U8(context.Background(), miniprogram.Candidate{
		URL:        "http://example.com/test.m3u8",
		Kind:       "m3u8",
		CachedPath: playlist,
	}, filepath.Join(dir, "out.mp4"))
	if err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
	if !strings.Contains(err.Error(), "未找到 ffmpeg") {
		t.Errorf("expected ffmpeg not found error, got: %v", err)
	}
}

func TestDownloadM3U8FailsClearlyWhenPlaylistCacheIsUnavailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	d := New(t.TempDir())
	d.FFmpegPath = "nonexistent_ffmpeg_xxx"
	err := d.downloadM3U8(context.Background(), miniprogram.Candidate{
		ID:   "expired",
		URL:  ts.URL + "/expired.m3u8",
		Kind: "m3u8",
	}, filepath.Join(t.TempDir(), "out.mp4"))
	if err == nil {
		t.Fatal("expected error when m3u8 cache is unavailable")
	}
	if !strings.Contains(err.Error(), "没有拿到 m3u8 本地缓存") {
		t.Fatalf("expected cache guidance error, got: %v", err)
	}
	if strings.Contains(err.Error(), "ffmpeg 合并失败") {
		t.Fatalf("should not pass expired remote m3u8 to ffmpeg, got: %v", err)
	}
}

func TestDownloadDirectReportsProgress(t *testing.T) {
	body := strings.Repeat("a", 8192)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "8192")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	var progress []Progress
	d := New(t.TempDir())
	d.OnProgress = func(p Progress) {
		progress = append(progress, p)
	}

	_, err := d.Download(context.Background(), miniprogram.Candidate{
		ID:            "direct-progress",
		URL:           ts.URL + "/video.mp4",
		Kind:          "video",
		Suffix:        ".mp4",
		ContentLength: 8192,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	last := progress[len(progress)-1]
	if last.Status != "done" {
		t.Fatalf("last status = %q, want done", last.Status)
	}
	if last.Downloaded != 8192 || last.Total != 8192 {
		t.Fatalf("last progress = %d/%d, want 8192/8192", last.Downloaded, last.Total)
	}
}
