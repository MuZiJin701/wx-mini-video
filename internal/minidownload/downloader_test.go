package minidownload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"wx_channel/internal/miniprogram"
)

func TestDownloadDirectAudioPreservesBytesAndSuffixWithoutFFmpeg(t *testing.T) {
	body := []byte("ID3\x00\x00\x00direct audio fixture")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://servicewechat.com/wx123/1/page-frame.html" {
			t.Error("audio download lost its replay headers")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(body)
	}))
	defer ts.Close()
	d := New(t.TempDir())
	d.FFmpegPath = filepath.Join(d.Dir, "ffmpeg-not-installed.exe")
	url := ts.URL + "/stream"
	kind, suffix := miniprogram.Classify("audio/mpeg", url)
	candidate := miniprogram.Candidate{ID: "audio-test", URL: url, Kind: kind, Suffix: suffix,
		Headers: map[string]string{"Referer": "https://servicewechat.com/wx123/1/page-frame.html"}}
	output, err := d.Download(context.Background(), candidate, "音频标题", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil || string(got) != string(body) || filepath.Base(output) != "音频标题.mp3" {
		t.Fatalf("audio output changed bytes or suffix: %s, %v", output, err)
	}
	if want := filepath.Join(d.Dir, "_unknown", miniprogram.DirAudios); filepath.Dir(output) != want {
		t.Fatalf("audio output dir = %q, want %q", filepath.Dir(output), want)
	}
}

func TestDownloadDirectResumesPartialAndWritesHistory(t *testing.T) {
	body := []byte(strings.Repeat("wx-mini-video", 1024))
	half := len(body) / 2
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("ETag", `"original"`)
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body[:half])
			return
		}
		if got := r.Header.Get("Range"); got != "bytes="+strconv.Itoa(half)+"-" {
			t.Fatalf("Range = %q, want resume from %d", got, half)
		}
		if r.Header.Get("If-Range") != `"original"` {
			t.Error("resume did not validate the original entity")
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", half, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[half:])
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{
		ID:            "resume-history",
		AppID:         "wx123",
		SourceAppID:   "wx456",
		Attribution:   miniprogram.AttributionConfirmed,
		URL:           ts.URL + "/resume.mp4",
		Kind:          "video",
		Suffix:        ".mp4",
		ContentLength: int64(len(body)),
	}

	firstOutput, err := d.Download(context.Background(), candidate, "", "wx123")
	if err == nil {
		t.Fatal("first download should fail after the server closes the response early")
	}
	partPath := partialPath(firstOutput, candidate)
	if got := fileSize(partPath); got != int64(half) {
		t.Fatalf("partial size = %d, want %d", got, half)
	}

	outputPath, err := d.Download(context.Background(), candidate, "", "wx123")
	if err != nil {
		t.Fatalf("resumed download failed: %v", err)
	}
	if outputPath != firstOutput {
		t.Fatalf("resumed output = %q, want same file %q", outputPath, firstOutput)
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
	if record.AppID != "wx123" || record.SourceAppID != "wx456" || record.Kind != "video" || record.Attribution != miniprogram.AttributionConfirmed {
		t.Fatalf("history attribution fields = %#v", record)
	}
	if record.CompletedAt.IsZero() || time.Since(record.CompletedAt) < 0 {
		t.Fatalf("history timestamp = %v, want recent UTC time", record.CompletedAt)
	}
	if wantDir := filepath.Join(d.Dir, "wx123", miniprogram.DirVideos); filepath.Dir(outputPath) != wantDir {
		t.Fatalf("output dir = %q, want %q", filepath.Dir(outputPath), wantDir)
	}
}

func TestDownloadUsesSafeDistinctWindowsOutputName(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("video"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	candidate := miniprogram.Candidate{ID: "candidate-12345678", URL: ts.URL + "/video.mp4", Kind: "video", Suffix: ".mp4"}
	bucket := filepath.Join(d.Dir, "_unknown", miniprogram.DirVideos)
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bucket, "回放_2026-08-06T19-30-00Z.mp4"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := d.Download(context.Background(), candidate, "回放_2026-08-06T19:30:00Z", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(path), "回放_2026-08-06T19-30-00Z_2.mp4"; got != want {
		t.Fatalf("output filename = %q, want %q", got, want)
	}
}

func TestOutputNameAvoidsWindowsReservedDeviceNames(t *testing.T) {
	got := OutputName(miniprogram.Candidate{Kind: "video", Suffix: ".mp4"}, "CON")
	if got != "_CON.mp4" {
		t.Fatalf("OutputName() = %q, want %q", got, "_CON.mp4")
	}
}

func TestRunFFmpegWritesRedactedCandidateLog(t *testing.T) {
	d := New(t.TempDir())
	ffmpeg := filepath.Join(d.Dir, "failing-ffmpeg.cmd")
	resourceURL := "https://cdn.example.com/live.m3u8?token=query-secret"
	script := "@echo Authorization: Bearer test-secret 1>&2\r\n@echo " + resourceURL + " 1>&2\r\n@exit /b 1\r\n"
	if runtime.GOOS != "windows" {
		ffmpeg = filepath.Join(d.Dir, "failing-ffmpeg")
		script = "#!/bin/sh\nprintf '%s\\n' 'Authorization: Bearer test-secret' '" + resourceURL + "' >&2\nexit 1\n"
	}
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	d.FFmpegPath = ffmpeg
	candidate := miniprogram.Candidate{ID: "candidate-log-123", URL: resourceURL, Headers: map[string]string{"Authorization": "Bearer test-secret"}}
	err := d.runFFmpeg(context.Background(), candidate, "playlist.m3u8", filepath.Join(d.Dir, "output.mp4"), 0)
	if err == nil {
		t.Fatal("runFFmpeg should report the process failure")
	}
	logPath := filepath.Join(d.Dir, ".ffmpeg", "candidate-log-123.log")
	if !strings.Contains(err.Error(), logPath) {
		t.Fatalf("error = %q, want log path %q", err, logPath)
	}
	logBody, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(logBody), "test-secret") || strings.Contains(string(logBody), "query-secret") || !strings.Contains(string(logBody), "<REDACTED HEADER>") {
		t.Fatalf("ffmpeg log = %q, want redacted secret", logBody)
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
	body := []byte("#EXTM3U\n#EXTINF:10.000,\n" + ts.URL + "/seg.ts\n")
	bucket := testBucket(d)
	dir := filepath.Join(bucket, ".segments", "reuse", miniprogram.CandidateID("", string(body)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cached := filepath.Join(dir, "seg_000001.ts")
	if err := os.WriteFile(cached, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSegmentDone(t, cached, ts.URL+"/seg.ts", int64(len("cached")))
	_, stats, err := d.localizeM3U8(context.Background(), bucket, "reuse", body, nil, ts.URL)
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

	_, stats, err := d.localizeM3U8(context.Background(), testBucket(d), "failed-stats", body, nil, ts.URL)
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

	localPath, stats, err := d.localizeM3U8(context.Background(), testBucket(d), "candidate", body, map[string]string{}, ts.URL)
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
	var failing atomic.Bool
	failing.Store(true)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.ts" && failing.Load() {
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

	bucket := testBucket(d)
	_, stats, err := d.localizeM3U8(context.Background(), bucket, "failed", body, nil, ts.URL)
	if err == nil {
		t.Fatal("expected partial failure")
	}
	completed := filepath.Join(stats.Dir, "seg_000001.ts")
	if _, statErr := os.Stat(completed); statErr != nil {
		t.Fatalf("expected successful segment kept for retry/debugging: %v", statErr)
	}
	if !segmentComplete(completed, ts.URL+"/ok.ts") {
		t.Fatal("completed segment was kept without reusable metadata")
	}
	failedPath := filepath.Join(stats.Dir, "seg_000002.ts")
	if fileExists(failedPath) || fileExists(segmentPartPath(failedPath)) {
		t.Fatal("failed segment left a final or partial file behind")
	}

	failing.Store(false)
	_, retried, err := d.localizeM3U8(context.Background(), bucket, "failed", body, nil, ts.URL)
	if err != nil {
		t.Fatalf("retry after partial failure: %v", err)
	}
	if retried.Reused != 1 || retried.Completed != 2 {
		t.Fatalf("retry stats = %+v, want one reused and two completed resources", retried)
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
	}, testBucket(d))
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
	bucket := testBucket(d)
	localPath, err := d.cacheM3U8FromURL(context.Background(), miniprogram.Candidate{
		ID:      "test123",
		URL:     ts.URL + "/playlist.m3u8",
		Kind:    "m3u8",
		Headers: map[string]string{},
	}, bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(localPath)
	if filepath.Dir(localPath) != bucket {
		t.Fatalf("cached playlist dir = %q, want %q", filepath.Dir(localPath), bucket)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ts.URL+"/seg1.ts") {
		t.Errorf("cached m3u8 should contain absolute URL, got:\n%s", string(data))
	}
}

func TestDownloadM3U8FallsBackToRemotePlaylistWithSessionHeaders(t *testing.T) {
	var gotSession string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("X-Wx-Session")
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	d.FFmpegPath = "nonexistent_ffmpeg_xxx"
	output, err := d.Download(context.Background(), miniprogram.Candidate{
		ID:      "remote-fallback",
		Kind:    "m3u8",
		URL:     ts.URL + "/playlist.m3u8",
		Headers: map[string]string{"X-Wx-Session": "current-session"},
	}, "", "")
	if err == nil || !strings.Contains(err.Error(), "未找到 ffmpeg") {
		t.Fatalf("Download() error = %v, want successful playlist fallback before ffmpeg check", err)
	}
	if gotSession != "current-session" {
		t.Fatalf("remote playlist request X-Wx-Session = %q, want current session header", gotSession)
	}
	cached, err := os.ReadFile(filepath.Join(filepath.Dir(output), "remote-fallback.m3u8"))
	if err != nil || !strings.Contains(string(cached), "#EXTM3U") {
		t.Fatalf("remote fallback playlist = %q, %v; want cached playlist", cached, err)
	}
	if got, want := filepath.Base(filepath.Dir(output)), miniprogram.DirSegments; got != want {
		t.Fatalf("playlist cache dir = %q, want %q", got, want)
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
	if !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "重新播放目标媒体后重试") {
		t.Fatalf("expected actionable remote failure, got: %v", err)
	}
	if strings.Contains(err.Error(), "按 c") || strings.Contains(err.Error(), "cache 标记") {
		t.Fatalf("error should not require clearing candidates or a cache marker: %v", err)
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
	}, "", "")
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

func TestDownloadDefaultNameUsesLocalTimeTypeAndShortCandidateID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("image"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{
		ID:      "1a2b3c4d5e6f",
		AppID:   "wx123",
		Kind:    "image",
		Suffix:  ".jpg",
		URL:     ts.URL + "/cover",
		Headers: map[string]string{},
	}
	output, err := d.Download(context.Background(), candidate, "", "wxTarget")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(output), "图片_20261025-130000_1a2b3c4d.jpg"; got != want {
		t.Fatalf("output filename = %q, want %q", got, want)
	}
	if got, want := filepath.Dir(output), filepath.Join(d.Dir, "wx123", miniprogram.DirImages); got != want {
		t.Fatalf("output dir = %q, want %q", got, want)
	}
}

func TestDownloadUnattributedCandidateUsesCurrentTargetAndCopyToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("video"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 5, 12, 0, time.Local))
	output, err := d.Download(context.Background(), miniprogram.Candidate{
		Kind:    "video",
		Suffix:  ".mp4",
		URL:     ts.URL + "/clip.mp4",
		Headers: map[string]string{},
	}, "", "wx456")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(output), "视频_20261025-130512_copy.mp4"; got != want {
		t.Fatalf("output filename = %q, want %q", got, want)
	}
	if got, want := filepath.Dir(output), filepath.Join(d.Dir, "wx456", miniprogram.DirVideos); got != want {
		t.Fatalf("output dir = %q, want %q", got, want)
	}

	historyBody, err := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(historyBody))), &record); err != nil {
		t.Fatal(err)
	}
	if record.AppID != "wx456" || record.SourceAppID != "" || record.Kind != "video" || record.Attribution != "" {
		t.Fatalf("unattributed history record = %#v", record)
	}
}

func TestDownloadM3U8NameUsesMP4AndSegmentsBucket(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	playlist := filepath.Join(dir, "captured.m3u8")
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := New(dir)
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 7, 33, 0, time.Local))
	d.FFmpegPath = "nonexistent_ffmpeg_xxx"
	output, err := d.Download(context.Background(), miniprogram.Candidate{
		ID:         "5a4b3c2d1e0f",
		AppID:      "wx789",
		Kind:       "m3u8",
		URL:        ts.URL + "/live/index.m3u8",
		CachedPath: playlist,
	}, "", "wx789")
	if err == nil || !strings.Contains(err.Error(), "未找到 ffmpeg") {
		t.Fatalf("Download() error = %v, want ffmpeg missing error", err)
	}
	if got, want := filepath.Base(output), "分段媒体_20261025-130733_5a4b3c2d.mp4"; got != want {
		t.Fatalf("output filename = %q, want %q", got, want)
	}
	if got, want := filepath.Dir(output), filepath.Join(d.Dir, "wx789", miniprogram.DirSegments); got != want {
		t.Fatalf("output dir = %q, want %q", got, want)
	}
}

func testBucket(d *Downloader) string {
	return filepath.Join(d.Dir, "wx123", miniprogram.DirSegments)
}

func stubDownloadClock(t *testing.T, at time.Time) {
	t.Helper()
	old := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = old })
}

func TestDownloadRecordsLiveFLVUntilStopped(t *testing.T) {
	frame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), make([]byte, 512)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 2000; i++ {
			if _, err := w.Write(frame); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}))
	defer ts.Close()

	d := New(t.TempDir())
	var recording atomic.Bool
	d.OnProgress = func(p Progress) {
		if p.Status == "recording" && p.Live {
			recording.Store(true)
		}
	}
	candidate := miniprogram.Candidate{
		ID:     "live-flv",
		URL:    ts.URL + "/live/7559_esd.flv",
		Kind:   "video",
		Suffix: ".flv",
		Live:   true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		output string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, err := d.Download(ctx, candidate, "", "")
		done <- result{output: output, err: err}
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !recording.Load() {
		if time.Now().After(deadline) {
			t.Fatal("download never reported recording progress for the live flv stream")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	got := <-done
	if !errors.Is(got.err, ErrRecordingStopped) {
		t.Fatalf("download error = %v, want ErrRecordingStopped", got.err)
	}
	if filepath.Ext(got.output) != ".flv" {
		t.Fatalf("recording output = %q, want .flv suffix", got.output)
	}
	if wantDir := filepath.Join(d.Dir, "_unknown", miniprogram.DirLive); filepath.Dir(got.output) != wantDir {
		t.Fatalf("recording dir = %q, want %q", filepath.Dir(got.output), wantDir)
	}
	if gotName := filepath.Base(got.output); !strings.HasPrefix(gotName, "直播_") {
		t.Fatalf("recording name = %q, want 直播_ prefix", gotName)
	}
	data, err := os.ReadFile(got.output)
	if err != nil {
		t.Fatalf("recording output missing: %v", err)
	}
	if len(data) == 0 || string(data[:3]) != "FLV" {
		t.Fatalf("recording output = %d bytes starting %q, want FLV payload", len(data), data[:min(3, len(data))])
	}
	if fileExists(partialPath(got.output, candidate)) {
		t.Fatal("recording .part file should be renamed after stopping")
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing for stopped recording: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != got.output || record.Kind != "live" {
		t.Fatalf("history record = %#v, want live recording at %q", record, got.output)
	}
}

func TestDownloadFiniteFLVIgnoresLiveHint(t *testing.T) {
	body := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), make([]byte, 4096)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	var progress []Progress
	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	d.OnProgress = func(p Progress) { progress = append(progress, p) }
	candidate := miniprogram.Candidate{
		ID:     "finite-flv",
		URL:    ts.URL + "/clip.flv",
		Kind:   "video",
		Suffix: ".flv",
		Live:   true,
	}
	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatalf("finite flv download failed: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil || len(got) != len(body) {
		t.Fatalf("finite flv output = %d bytes (err %v), want %d", len(got), err, len(body))
	}
	// 疑似直播按 直播/ 选名，实际响应为有限文件后移动到 视频/，默认前缀同步改为 视频_。
	if wantDir := filepath.Join(d.Dir, "_unknown", miniprogram.DirVideos); filepath.Dir(output) != wantDir {
		t.Fatalf("finite flv dir = %q, want %q", filepath.Dir(output), wantDir)
	}
	if gotName, want := filepath.Base(output), "视频_20261025-130000_finite-f.flv"; gotName != want {
		t.Fatalf("finite flv name = %q, want %q", gotName, want)
	}
	if stale := filepath.Join(d.Dir, "_unknown", miniprogram.DirLive, "直播_20261025-130000_finite-f.flv"); fileExists(stale) {
		t.Fatalf("finite flv left a stale file at %q", stale)
	}

	last := progress[len(progress)-1]
	if last.Status != "done" || last.Live {
		t.Fatalf("last progress = %#v, want done without live flag", last)
	}
	if last.Path != output {
		t.Fatalf("progress path = %q, want final path %q", last.Path, output)
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Kind != "video" || record.Size != int64(len(body)) {
		t.Fatalf("history record = %#v, want video at final path %q", record, output)
	}
}

func TestOutputNameKeepsSingleFLVSuffix(t *testing.T) {
	candidate := miniprogram.Candidate{ID: "a187d7aeff6f", Kind: "video", Suffix: ".flv", Live: true}
	if got := OutputName(candidate, "7559184446249188_esd.flv"); got != "7559184446249188_esd.flv" {
		t.Fatalf("preferred flv name = %q, want unchanged", got)
	}
	name := OutputName(candidate, "")
	if filepath.Ext(name) != ".flv" || strings.Count(name, ".flv") != 1 {
		t.Fatalf("default live flv name = %q, want exactly one .flv suffix", name)
	}
	if !strings.HasPrefix(name, "直播_") {
		t.Fatalf("default live flv name = %q, want live type prefix", name)
	}
	finite := miniprogram.Candidate{ID: "a187d7aeff6f", Kind: "video", Suffix: ".flv"}
	if got := OutputName(finite, ""); !strings.HasPrefix(got, "视频_") || filepath.Ext(got) != ".flv" {
		t.Fatalf("default finite flv name = %q, want video prefix and .flv suffix", got)
	}
}

func TestDownloadLiveInterruptedKeepsRecording(t *testing.T) {
	frame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), make([]byte, 512)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher := w.(http.Flusher)
		for i := 0; i < 4; i++ {
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		}
		panic(http.ErrAbortHandler)
	}))
	defer ts.Close()

	d := New(t.TempDir())
	candidate := miniprogram.Candidate{ID: "live-interrupted", AppID: "wx123", URL: ts.URL + "/live/7559_esd.flv", Kind: "video", Suffix: ".flv", Live: true}
	output, err := d.Download(context.Background(), candidate, "", "wx123")
	if !errors.Is(err, ErrRecordingInterrupted) {
		t.Fatalf("error = %v, want ErrRecordingInterrupted", err)
	}
	if wantDir := filepath.Join(d.Dir, "wx123", miniprogram.DirLive); filepath.Dir(output) != wantDir {
		t.Fatalf("interrupted recording dir = %q, want %q", filepath.Dir(output), wantDir)
	}
	data, readErr := os.ReadFile(output)
	if readErr != nil || len(data) == 0 || string(data[:3]) != "FLV" {
		t.Fatalf("interrupted recording not saved: %v, %d bytes", readErr, len(data))
	}
	if fileExists(partialPath(output, candidate)) {
		t.Fatal("interrupted recording .part should be committed")
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing for interrupted recording: %v", readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(history)), "\n")
	var record DownloadHistory
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Size == 0 || record.Kind != "live" {
		t.Fatalf("history record = %#v, want saved live interrupted recording", record)
	}
}

func TestDownloadStopReportsSaveFailure(t *testing.T) {
	frame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), make([]byte, 512)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher := w.(http.Flusher)
		for {
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "stop-save-fail", AppID: "wx123", URL: ts.URL + "/live/stop_save_fail.flv", Kind: "video", Suffix: ".flv", Live: true}
	// 用非空目录预占最终路径，使提交重命名必然失败。疑似直播先按 直播/ 选名。
	blocked := filepath.Join(d.Dir, "wx123", miniprogram.DirLive, OutputName(candidate, ""))
	if err := os.MkdirAll(filepath.Join(blocked, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "inner", "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var recording atomic.Bool
	d.OnProgress = func(p Progress) {
		if p.Status == "recording" && p.Live {
			recording.Store(true)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := d.Download(ctx, candidate, "", "wx123")
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !recording.Load() {
		if time.Now().After(deadline) {
			t.Fatal("没有进入录制状态")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	err := <-done
	if err == nil || errors.Is(err, ErrDownloadStopped) || errors.Is(err, ErrRecordingStopped) {
		t.Fatalf("save failure was masked as normal stop: %v", err)
	}
	if !strings.Contains(err.Error(), "保存已录内容失败") {
		t.Fatalf("error = %v, want save failure detail", err)
	}
	if !fileExists(partialPath(filepath.Join(blocked), candidate)) {
		t.Fatal("save failure should keep the recorded .part for recovery")
	}
}

func TestDownloadRetryAfterSecondBoundaryResumes(t *testing.T) {
	body := []byte(strings.Repeat("wx-mini-video", 1024))
	half := len(body) / 2
	var mu sync.Mutex
	var ranges []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		w.Header().Set("ETag", `"original"`)
		if r.Header.Get("Range") == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			_, _ = w.Write(body[:half])
			return
		}
		if got, want := r.Header.Get("Range"), "bytes="+strconv.Itoa(half)+"-"; got != want {
			t.Errorf("Range = %q, want %q", got, want)
		}
		if r.Header.Get("If-Range") != `"original"` {
			t.Error("resume did not validate the original entity")
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", half, len(body)-1, len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[half:])
	}))
	defer ts.Close()

	d := New(t.TempDir())
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local)
	oldNow := now
	calls := 0
	now = func() time.Time {
		calls++
		if calls <= 1 {
			return base
		}
		return base.Add(time.Second)
	}
	defer func() { now = oldNow }()

	candidate := miniprogram.Candidate{ID: "retry-boundary", AppID: "wx123", URL: ts.URL + "/resume.mp4", Kind: "video", Suffix: ".mp4", ContentLength: int64(len(body))}
	first, err := d.Download(context.Background(), candidate, "", "wx123")
	if err == nil {
		t.Fatal("first download should fail after the server closes the response early")
	}
	second, err := d.Download(context.Background(), candidate, "", "wx123")
	if err != nil {
		t.Fatalf("retry after the second boundary failed: %v", err)
	}
	if first != second {
		t.Fatalf("retry used a new output name: %q vs %q", first, second)
	}
	data, err := os.ReadFile(second)
	if err != nil || string(data) != string(body) {
		t.Fatalf("resumed content mismatch: %d bytes, err %v", len(data), err)
	}
	mu.Lock()
	got := append([]string(nil), ranges...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "" || !strings.HasPrefix(got[1], "bytes=") {
		t.Fatalf("requests = %v, want first full GET and second Range request", got)
	}
}

func TestRetryPreservesUnsavedLiveRecording(t *testing.T) {
	oldMarker := []byte("OLD-UNSAVED-LIVE-RECORDING-MARKER")
	oldPart := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), oldMarker...)
	newFrame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), bytes.Repeat([]byte("NEW"), 64)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher := w.(http.Flusher)
		_, _ = w.Write(newFrame)
		flusher.Flush()
		_, _ = w.Write(newFrame)
		flusher.Flush()
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "retry-live", URL: ts.URL + "/media/live-stream", Kind: "video", Suffix: ".mp4"}
	bucket := filepath.Join(d.Dir, "_unknown", miniprogram.DirVideos)
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		t.Fatal(err)
	}
	oldName := OutputName(candidate, "")
	oldPath := filepath.Join(bucket, oldName)
	oldPartPath := oldPath + partialSuffix(candidate)
	if err := os.WriteFile(oldPartPath, oldPart, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPartPath+".json", []byte(`{"total":-1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if output == oldPath {
		t.Fatalf("retry reused the unsaved recording path %q", output)
	}
	if filepath.Ext(output) != ".flv" {
		t.Fatalf("late-detected live output = %q, want .flv suffix", output)
	}
	oldData, err := os.ReadFile(oldPartPath)
	if err != nil || !bytes.Contains(oldData, oldMarker) {
		t.Fatalf("unsaved live recording was overwritten: err=%v data=%q", err, oldData)
	}
	if _, err := os.Stat(oldPartPath + ".json"); err != nil {
		t.Fatalf("unsaved live recording metadata missing: %v", err)
	}
	finalData, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(finalData, []byte("NEW")) || bytes.Contains(finalData, oldMarker) {
		t.Fatalf("new recording content invalid: err=%v data=%q", err, finalData)
	}
	history, err := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if err != nil || !strings.Contains(string(history), filepath.Base(output)) {
		t.Fatalf("history missing for new recording: err=%v history=%q", err, history)
	}
}

func TestLiveDownloadLateDetectionUsesFLVSuffix(t *testing.T) {
	frame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), bytes.Repeat([]byte("NEW"), 64)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write(frame)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "late-live", URL: ts.URL + "/media/live-stream", Kind: "video", Suffix: ".mp4"}
	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatalf("late-detected live download failed: %v", err)
	}
	if filepath.Ext(output) != ".flv" {
		t.Fatalf("output = %q, want .flv suffix for confirmed live FLV", output)
	}
	// 初始按 视频/ 选名，下载时确认直播后移动到 直播/，默认前缀同步改为 直播_。
	if wantDir := filepath.Join(d.Dir, "_unknown", miniprogram.DirLive); filepath.Dir(output) != wantDir {
		t.Fatalf("late live dir = %q, want %q", filepath.Dir(output), wantDir)
	}
	if gotName, want := filepath.Base(output), "直播_20261025-130000_late-liv.flv"; gotName != want {
		t.Fatalf("late live name = %q, want %q", gotName, want)
	}
	if stale := filepath.Join(d.Dir, "_unknown", miniprogram.DirVideos, "视频_20261025-130000_late-liv.mp4"); fileExists(stale) {
		t.Fatalf("stale .mp4 file left behind for live FLV: %q", stale)
	}
	data, err := os.ReadFile(output)
	if err != nil || !bytes.HasPrefix(data, []byte("FLV")) {
		t.Fatalf("output is not FLV: err=%v", err)
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing for late-detected live: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Kind != "live" {
		t.Fatalf("history record = %#v, want live at %q", record, output)
	}
}

func TestRetryAvoidsFinalAndPartialNameCollisions(t *testing.T) {
	oldMarker := []byte("OLD-UNSAVED-LIVE-RECORDING-MARKER")
	oldPart := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), oldMarker...)
	newFrame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), bytes.Repeat([]byte("NEW"), 64)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher := w.(http.Flusher)
		_, _ = w.Write(newFrame)
		flusher.Flush()
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "collision-retry", URL: ts.URL + "/live/stream", Kind: "video", Suffix: ".flv", Live: true}
	bucket := filepath.Join(d.Dir, "_unknown", miniprogram.DirLive)
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		t.Fatal(err)
	}
	baseName := OutputName(candidate, "")
	savedPath := filepath.Join(bucket, baseName)
	if err := os.WriteFile(savedPath, []byte("OLD-SAVED-RECORDING"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 成品占用 base；未保存录制占用 base_2；旧实现会先选 base、再被改名到 base_2 并清空它。
	ext := filepath.Ext(baseName)
	secondName := strings.TrimSuffix(baseName, ext) + "_2" + ext
	unsavedPart := filepath.Join(bucket, secondName+partialSuffix(candidate))
	if err := os.WriteFile(unsavedPart, oldPart, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unsavedPart+".json", []byte(`{"total":-1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if output == savedPath || output == filepath.Join(bucket, secondName) {
		t.Fatalf("retry reused %q despite existing final/partial collisions", output)
	}
	oldData, readErr := os.ReadFile(unsavedPart)
	if readErr != nil || !bytes.Contains(oldData, oldMarker) {
		t.Fatalf("unsaved recording was overwritten: err=%v data=%q", readErr, oldData)
	}
	if _, err := os.Stat(unsavedPart + ".json"); err != nil {
		t.Fatalf("unsaved recording metadata missing: %v", err)
	}
	if saved, err := os.ReadFile(savedPath); err != nil || string(saved) != "OLD-SAVED-RECORDING" {
		t.Fatalf("existing final recording changed: err=%v data=%q", err, saved)
	}
	finalData, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(finalData, []byte("NEW")) {
		t.Fatalf("new recording content invalid: err=%v", err)
	}
}

func TestDownloadUnknownFileUsesOtherDir(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("unknown-payload"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "file-candidate", Kind: "file", Suffix: ".bin", URL: ts.URL + "/download/asset"}
	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(output), "其他_20261025-130000_file-can.bin"; got != want {
		t.Fatalf("unknown file name = %q, want %q", got, want)
	}
	if got, want := filepath.Dir(output), filepath.Join(d.Dir, "_unknown", miniprogram.DirOther); got != want {
		t.Fatalf("unknown file dir = %q, want %q", got, want)
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Kind != "file" {
		t.Fatalf("history record = %#v, want file kind at %q", record, output)
	}
}

// TestDownloadSuspectedLiveMoveDoesNotOverwriteTargets 覆盖跨目录调整的重名保护：
// 疑似直播按 直播/ 选名，确认是有限文件后要移到 视频/；目标目录里已有成品和
// 未保存录制时，最终文件必须换名落位，不能覆盖任何既有内容。
func TestDownloadSuspectedLiveMoveDoesNotOverwriteTargets(t *testing.T) {
	body := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), bytes.Repeat([]byte("NEW"), 64)...)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	d := New(t.TempDir())
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	candidate := miniprogram.Candidate{ID: "collide123456", URL: ts.URL + "/clip.flv", Kind: "video", Suffix: ".flv", Live: true}
	bucket := filepath.Join(d.Dir, "_unknown", miniprogram.DirVideos)
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(bucket, "视频_20261025-130000_collide1.flv")
	if err := os.WriteFile(existingPath, []byte("OLD-FINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	unsavedPart := filepath.Join(bucket, "视频_20261025-130000_collide1_2.flv"+partialSuffix(candidate))
	unsavedData := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), []byte("OLD-UNSAVED")...)
	if err := os.WriteFile(unsavedPart, unsavedData, 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := d.Download(context.Background(), candidate, "", "")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if got, want := filepath.Base(output), "视频_20261025-130000_collide1_3.flv"; got != want {
		t.Fatalf("moved file name = %q, want %q", got, want)
	}
	if saved, readErr := os.ReadFile(existingPath); readErr != nil || string(saved) != "OLD-FINAL" {
		t.Fatalf("existing final file changed: err=%v data=%q", readErr, saved)
	}
	if kept, readErr := os.ReadFile(unsavedPart); readErr != nil || !bytes.Equal(kept, unsavedData) {
		t.Fatalf("unsaved recording changed: err=%v data=%q", readErr, kept)
	}
	moved, readErr := os.ReadFile(output)
	if readErr != nil || !bytes.Equal(moved, body) {
		t.Fatalf("moved finite flv content invalid: err=%v", readErr)
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Kind != "video" {
		t.Fatalf("history record = %#v, want video at %q", record, output)
	}
}

// TestDownloadM3U8KeepsArtifactsInSegmentsDir 确认 m3u8 合并产物、播放列表缓存与
// .segments 分片目录都位于 <AppID>/分段媒体/ 下。
func TestDownloadM3U8KeepsArtifactsInSegmentsDir(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("segment-bytes"))
	}))
	defer ts.Close()

	d := New(t.TempDir())
	d.FFmpegPath = fakeCopyFFmpeg(t, d.Dir)
	d.DownloadMode = "prefetch"
	d.KeepSegments = true
	stubDownloadClock(t, time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local))
	playlist := filepath.Join(d.Dir, "captured.m3u8")
	body := "#EXTM3U\n#EXTINF:1.000,\n" + ts.URL + "/seg1.ts\n#EXT-X-ENDLIST\n"
	if err := os.WriteFile(playlist, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	candidate := miniprogram.Candidate{ID: "seg-bucket", AppID: "wx789", Kind: "m3u8", URL: ts.URL + "/playlist.m3u8", CachedPath: playlist}
	output, err := d.Download(context.Background(), candidate, "", "wx789")
	if err != nil {
		t.Fatalf("m3u8 download failed: %v", err)
	}
	wantDir := filepath.Join(d.Dir, "wx789", miniprogram.DirSegments)
	if filepath.Dir(output) != wantDir {
		t.Fatalf("m3u8 output dir = %q, want %q", filepath.Dir(output), wantDir)
	}
	if !strings.HasPrefix(filepath.Base(output), miniprogram.DirSegments+"_") {
		t.Fatalf("m3u8 output name = %q, want 分段媒体_ prefix", filepath.Base(output))
	}
	segmentsDir := filepath.Join(wantDir, ".segments", candidate.ID)
	entries, readErr := os.ReadDir(segmentsDir)
	if readErr != nil || len(entries) != 1 {
		t.Fatalf(".segments dir = %q, entries %v, err %v", segmentsDir, entries, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(segmentsDir, entries[0].Name(), "seg_000001.ts")); statErr != nil {
		t.Fatalf("download segment missing under %q: %v", segmentsDir, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(segmentsDir, entries[0].Name(), "playlist.m3u8")); statErr != nil {
		t.Fatalf("localized playlist missing under %q: %v", segmentsDir, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(d.Dir, ".segments")); !os.IsNotExist(statErr) {
		t.Fatal(".segments must not be created in the download root")
	}
	history, readErr := os.ReadFile(filepath.Join(d.Dir, "history.jsonl"))
	if readErr != nil {
		t.Fatalf("history missing: %v", readErr)
	}
	var record DownloadHistory
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(history))), &record); err != nil {
		t.Fatalf("history record invalid: %v", err)
	}
	if record.Path != output || record.Kind != "m3u8" {
		t.Fatalf("history record = %#v, want m3u8 at %q", record, output)
	}
}

// fakeCopyFFmpeg 生成一个把 -i 输入原样复制到输出文件的假 ffmpeg，
// 用于在不依赖真实 ffmpeg 的情况下验证 m3u8 落盘位置。
func fakeCopyFFmpeg(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "copy-ffmpeg.cmd")
		script := "@echo off\r\n" +
			"setlocal enabledelayedexpansion\r\n" +
			"set \"input=\"\r\n" +
			"set \"output=\"\r\n" +
			"set \"prev=\"\r\n" +
			"for %%a in (%*) do (\r\n" +
			"  if \"!prev!\"==\"-i\" set \"input=%%a\"\r\n" +
			"  set \"prev=%%a\"\r\n" +
			"  set \"output=%%a\"\r\n" +
			")\r\n" +
			"copy /y \"!input!\" \"!output!\" >nul\r\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "copy-ffmpeg")
	script := "#!/bin/sh\nprev=\"\"\ninput=\"\"\noutput=\"\"\nfor arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"-i\" ]; then input=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n  output=\"$arg\"\ndone\ncp \"$input\" \"$output\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDownloadReportsLiveResolution 覆盖直播状态回写回调：下载前已知直播立即回调，
// 实际响应修正标记时再回调一次；状态未变化时不重复回调。
func TestDownloadReportsLiveResolution(t *testing.T) {
	frame := append([]byte("FLV\x01\x05\x00\x00\x00\x09"), bytes.Repeat([]byte("X"), 32)...)
	stream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write(frame)
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer stream.Close()
	finite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Content-Length", strconv.Itoa(len(frame)))
		_, _ = w.Write(frame)
	}))
	defer finite.Close()

	cases := []struct {
		name      string
		candidate miniprogram.Candidate
		want      []bool
	}{
		{
			name:      "suspected live confirmed finite",
			candidate: miniprogram.Candidate{ID: "cb-finite", URL: finite.URL + "/clip.flv", Kind: "video", Suffix: ".flv", Live: true},
			want:      []bool{true, false},
		},
		{
			name:      "finite candidate detected live",
			candidate: miniprogram.Candidate{ID: "cb-late", URL: stream.URL + "/media/live-stream", Kind: "video", Suffix: ".mp4"},
			want:      []bool{true},
		},
		{
			name:      "known live stays live",
			candidate: miniprogram.Candidate{ID: "cb-live", URL: stream.URL + "/live/7559_esd.flv", Kind: "video", Suffix: ".flv", Live: true},
			want:      []bool{true},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := New(t.TempDir())
			var mu sync.Mutex
			var got []bool
			d.OnLiveResolved = func(candidateID string, live bool) {
				if candidateID != tt.candidate.ID {
					t.Errorf("callback candidateID = %q, want %q", candidateID, tt.candidate.ID)
				}
				mu.Lock()
				got = append(got, live)
				mu.Unlock()
			}
			if _, err := d.Download(context.Background(), tt.candidate, "", ""); err != nil {
				t.Fatalf("download failed: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(got) != len(tt.want) {
				t.Fatalf("live callbacks = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("live callbacks = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestDownloadCancelBeforeResponseKeepsLiveHint 确认请求未拿到响应就被取消时不会回写“非直播”，
// 避免把还没确认的疑似直播错误改成有限视频。
func TestDownloadCancelBeforeResponseKeepsLiveHint(t *testing.T) {
	d := New(t.TempDir())
	var mu sync.Mutex
	var got []bool
	d.OnLiveResolved = func(candidateID string, live bool) {
		mu.Lock()
		got = append(got, live)
		mu.Unlock()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.Download(ctx, miniprogram.Candidate{ID: "cancel-live", URL: "https://example.com/live/7559_esd.flv", Kind: "video", Suffix: ".flv", Live: true}, "", "")
	if err == nil {
		t.Fatal("canceled download should report an error")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || !got[0] {
		t.Fatalf("live callbacks = %v, want only the pre-download live hint", got)
	}
}
