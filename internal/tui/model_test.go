package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
)

func TestTargetInputRequiresAppID(t *testing.T) {
	m := model{}
	next, cmd := m.updateTargetInput(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command when AppID is empty")
	}
	got := next.(model)
	if got.targetReady {
		t.Fatal("target should not be ready with empty AppID")
	}
	if got.inputErr == "" {
		t.Fatal("expected validation error")
	}
}

func TestTargetDisplayAllowsEmptyName(t *testing.T) {
	got := targetDisplay(miniprogram.Target{AppID: "wx123"})
	if got != "wx123" {
		t.Fatalf("targetDisplay() = %q", got)
	}
}

func TestTrimLastRuneHandlesChinese(t *testing.T) {
	got := trimLastRune("测试")
	if got != "测" {
		t.Fatalf("trimLastRune() = %q", got)
	}
}

func TestDefaultFilterShowsVideos(t *testing.T) {
	m := model{
		candidates: []miniprogram.Candidate{
			{Kind: "image", URL: "https://example.com/cover.jpg"},
			{Kind: "video", URL: "https://example.com/video.mp4"},
			{Kind: "m3u8", URL: "https://example.com/live.m3u8"},
		},
	}

	got := m.filteredCandidates()
	if len(got) != 1 || got[0].Kind != "video" {
		t.Fatalf("filteredCandidates default = %#v", got)
	}
}

func TestCategorySwitchingUsesTabAndNumbers(t *testing.T) {
	m := model{targetReady: true}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(model)
	if got.category != categoryM3U8 {
		t.Fatalf("Tab should move video -> m3u8, got %q", got.category)
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got = next.(model)
	if got.category != categoryImage {
		t.Fatalf("2 should select image, got %q", got.category)
	}
}

func TestAudioCategorySelectsAndStartsDownload(t *testing.T) {
	m := model{
		runtime: targetRuntime(t), targetReady: true,
		candidates: []miniprogram.Candidate{
			{Kind: "image", URL: "https://example.com/cover.jpg"},
			{Kind: "audio", URL: "https://example.com/song.mp3", Suffix: ".mp3", Title: "音频标题"},
		},
		width: 100, height: 24,
	}
	if view := m.View(); !strings.Contains(view, "5 查看音频") {
		t.Fatal("default video filter should guide users to the detected audio")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	m = next.(model)
	if got := m.filteredCandidates(); len(got) != 1 || got[0].Kind != "audio" {
		t.Fatalf("5 should show only audio: %#v", got)
	}
	if view := m.View(); !strings.Contains(view, "5 音频(1)") || !strings.Contains(view, "音频标题") {
		t.Fatalf("audio tab count or candidate was not displayed: %q", view)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(model).downloading || cmd == nil {
		t.Fatal("Enter should start the selected audio download")
	}
	// Existing keys remain stable; Tab from m3u8 enters audio, then unattributed, then wraps to all.
	m, _ = targetKey(m, tea.KeyRunes, "4")
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.category != categoryAudio {
		t.Fatal("Tab from m3u8 should enter audio")
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.category != categoryOther {
		t.Fatal("Tab from audio should enter unattributed")
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if len(m.filteredCandidates()) != 2 {
		t.Fatal("Tab from unattributed should show all media")
	}
}

func TestDownloadAllowsUncachedDirectM3U8(t *testing.T) {
	m := model{
		runtime: &app.Runtime{Settings: config.AppSettings{DownloadDir: t.TempDir()}},
		candidates: []miniprogram.Candidate{{
			Kind:    "m3u8",
			Source:  "response",
			URL:     "https://example.com/playlist.m3u8",
			Headers: map[string]string{"User-Agent": "wx-test"},
		}},
		category:    categoryM3U8,
		targetReady: true,
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := next.(model)
	if !got.downloading {
		t.Fatal("uncached direct m3u8 should enter the download flow")
	}
	if cmd == nil {
		t.Fatal("uncached direct m3u8 should schedule a download")
	}
}

func TestCandidateReadableInfoUsesFieldHostAndFilename(t *testing.T) {
	candidate := miniprogram.Candidate{
		Kind:      "image",
		URL:       "https://cdn.example.com/assets/product-cover.webp?token=abc",
		FieldPath: "data.items[].coverUrl",
		Source:    "json",
	}

	info := candidateReadableInfo(candidate)
	for _, want := range []string{"封面", "cdn.example.com", "product-cover.webp"} {
		if !strings.Contains(info, want) {
			t.Fatalf("candidateReadableInfo() = %q, want %q", info, want)
		}
	}
}

func TestCandidateSummaryDoesNotExposeQueryString(t *testing.T) {
	candidate := miniprogram.Candidate{
		Kind:          "video",
		URL:           "https://wxsmw.wxs.qq.com/path/video.mp4?ck=secret&sha256=long",
		ContentLength: 1024,
	}

	got := candidateSummary(candidate)
	if strings.Contains(got, "?ck=") || strings.Contains(got, "sha256=") {
		t.Fatalf("candidateSummary() exposed query string: %q", got)
	}
	if !strings.Contains(got, "wxsmw.wxs.qq.com") || !strings.Contains(got, "video.mp4") {
		t.Fatalf("candidateSummary() = %q, want host and filename", got)
	}
}

func TestCandidateLabelPrefersExtractedTitle(t *testing.T) {
	got := candidateLabel(miniprogram.Candidate{
		Title: "春日活动",
		Kind:  "video",
		URL:   "https://cdn.example.com/videos/spring.mp4",
	})
	if got != "春日活动" {
		t.Fatalf("candidateLabel() = %q, want extracted title", got)
	}
}

func TestCandidateDetailsShowFullMetadata(t *testing.T) {
	candidate := miniprogram.Candidate{
		Title:     "春日活动",
		Kind:      "video",
		Source:    "json",
		URL:       "https://cdn.example.com/videos/spring.mp4?token=secret",
		SourceURL: "https://mini.example.com/api/feed?id=7",
		Headers: map[string]string{
			"Referer":    "https://mini.example.com/",
			"User-Agent": "wx-test",
			"Cookie":     "session=secret",
		},
		CachedPath: "downloads/cache.m3u8",
	}

	got := renderCandidateDetails(candidate, 200)
	joined := strings.Join(got, "\n")
	for _, want := range []string{
		"春日活动",
		candidate.URL,
		candidate.SourceURL,
		"Referer",
		"User-Agent",
		"Cookie=已捕获",
		candidate.CachedPath,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("renderCandidateDetails() = %q, want %q", joined, want)
		}
	}
}

func TestDetailsShortcutTogglesSelectedCandidate(t *testing.T) {
	m := model{targetReady: true}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !next.(model).showDetails {
		t.Fatal("i should show candidate details")
	}
}

func TestViewKeepsProgressWhenCategoryChanges(t *testing.T) {
	m := model{
		runtime: &app.Runtime{
			Settings: config.AppSettings{
				DownloadDir:   "downloads",
				ProxyHostname: "127.0.0.1",
				ProxyPort:     2023,
				Target:        miniprogram.Target{AppID: "wx123"},
			},
			Downloader: minidownload.New(t.TempDir()),
		},
		candidates: []miniprogram.Candidate{
			{Kind: "image", URL: "https://example.com/cover.jpg"},
			{Kind: "video", URL: "https://example.com/video.mp4"},
		},
		category:    categoryVideo,
		targetReady: true,
		started:     true,
		downloading: true,
		progress: minidownload.Progress{
			Status:     "downloading",
			Downloaded: 20,
			Total:      100,
		},
		width:  80,
		height: 14,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	got := next.(model).View()
	if !strings.Contains(got, "下载中 20%") {
		t.Fatalf("View() lost download progress after category switch:\n%s", got)
	}
}

func TestCandidateListLineTruncatesLongResource(t *testing.T) {
	candidate := miniprogram.Candidate{
		Kind:          "image",
		URL:           "https://wximg.wxs.qq.com/141/20204/snscosdownload/SZ/reserved/very-long-resource-name.jpg?ck=secret&sha256=long",
		ContentLength: 27491,
		Source:        "response",
	}

	got := candidateListLine(candidate, 48)
	if strings.Contains(got, "?ck=") || strings.Contains(got, "sha256=") {
		t.Fatalf("candidateListLine() exposed query string: %q", got)
	}
	if len([]rune(got)) > 60 {
		t.Fatalf("candidateListLine() was not truncated enough: %q", got)
	}
}

func TestOtherCategoryShowsOnlyUnattributedWithMarkers(t *testing.T) {
	confirmed := miniprogram.Candidate{
		Kind: "video", URL: "https://cdn.example.com/target.mp4",
		Attribution: miniprogram.AttributionConfirmed, SourceAppID: "wx123",
	}
	unattributed := miniprogram.Candidate{
		Kind: "video", URL: "https://h5.example.com/lesson.mp4", Source: "response",
		Attribution: miniprogram.AttributionUnattributed,
	}
	m := model{
		runtime: targetRuntime(t), targetReady: true, started: true,
		candidates:      []miniprogram.Candidate{confirmed},
		otherCandidates: []miniprogram.Candidate{unattributed},
		category:        categoryVideo,
		width:           140, height: 30,
	}
	if got := m.filteredCandidates(); len(got) != 1 || got[0].URL != confirmed.URL {
		t.Fatalf("video category = %#v, want only the confirmed candidate", got)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	m = next.(model)
	if m.category != categoryOther {
		t.Fatalf("6 should select unattributed category, got %q", m.category)
	}
	if got := m.filteredCandidates(); len(got) != 1 || got[0].URL != unattributed.URL {
		t.Fatalf("unattributed category = %#v, want only the H5 candidate", got)
	}
	view := m.View()
	for _, want := range []string{"6 未归属(1)", "?"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() = %q, want %q", view, want)
		}
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	all := next.(model)
	if got := all.filteredCandidates(); len(got) != 1 || got[0].URL != confirmed.URL {
		t.Fatalf("all category = %#v, want only target candidates", got)
	}
	if view := all.View(); !strings.Contains(view, "✓") {
		t.Fatalf("confirmed candidate should be marked with ✓: %q", view)
	}

	m.showDetails = true
	details := strings.Join(m.selectedCandidateDetails(), "\n")
	for _, want := range []string{"来源未确认", "来源 AppID", "响应类型/URL 推断，来源未知"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details = %q, want %q", details, want)
		}
	}
}

func TestCandidateBasisExplainsProvisionalAndFileCandidates(t *testing.T) {
	if got := candidateBasis(miniprogram.Candidate{Provisional: true}); got != "字段名推断（低置信）" {
		t.Fatalf("provisional basis = %q", got)
	}
	if got := candidateBasis(miniprogram.Candidate{Kind: "file"}); got != "未知二进制兜底（≥256KB）" {
		t.Fatalf("file basis = %q", got)
	}
	if got := kindDisplay("file"); got != "文件" {
		t.Fatalf("kindDisplay(file) = %q", got)
	}
}

func TestTickRefreshLoadsUnattributedCandidates(t *testing.T) {
	runtime := targetRuntime(t)
	runtime.Store.AddUnattributed(miniprogram.Candidate{
		Kind: "video", URL: "https://h5.example.com/lesson.mp4",
		Attribution: miniprogram.AttributionUnattributed,
	})
	m := model{runtime: runtime, targetReady: true, started: true}
	next, _ := m.Update(tickMsg{})
	got := next.(model)
	if len(got.otherCandidates) != 1 || len(m.candidates) != 0 {
		t.Fatalf("refresh candidates=%#v other=%#v", got.candidates, got.otherCandidates)
	}
	if len(got.filteredCandidates()) != 0 {
		t.Fatal("default video category must not mix in unattributed candidates")
	}
}

func TestClearKeyClearsUnattributedList(t *testing.T) {
	runtime := targetRuntime(t)
	runtime.SetTarget(miniprogram.Target{AppID: "wx123"})
	runtime.Store.AddUnattributed(miniprogram.Candidate{
		Kind: "video", URL: "https://h5.example.com/lesson.mp4",
		Attribution: miniprogram.AttributionUnattributed,
	})
	m := model{
		runtime: runtime, targetReady: true, started: true,
		otherCandidates: runtime.Store.Unattributed(),
		width:           100, height: 24,
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got := next.(model)
	if len(got.otherCandidates) != 0 || len(runtime.Store.Unattributed()) != 0 {
		t.Fatalf("clear key kept unattributed candidates: %#v", got.otherCandidates)
	}
}

func TestDownloadOtherCandidateReplaysCapturedHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://course.example.com/learn/1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("media-bytes"))
	}))
	defer server.Close()

	dir := t.TempDir()
	runtime := &app.Runtime{
		Settings:   config.AppSettings{DownloadDir: dir},
		Downloader: minidownload.New(dir),
	}
	candidate := miniprogram.Candidate{
		Kind:        "video",
		Suffix:      ".mp4",
		URL:         server.URL + "/lesson.mp4",
		Attribution: miniprogram.AttributionUnattributed,
		Headers:     map[string]string{"Referer": "https://course.example.com/learn/1"},
	}
	msg := downloadCmd(runtime, candidate)()
	result, ok := msg.(downloadMsg)
	if !ok || result.err != nil {
		t.Fatalf("download msg = %#v, want success", msg)
	}
	data, err := os.ReadFile(result.path)
	if err != nil || string(data) != "media-bytes" {
		t.Fatalf("downloaded file = %q, %v", data, err)
	}
}
