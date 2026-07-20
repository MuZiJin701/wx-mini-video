package tui

import (
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
		ffmpegSetup: &ffmpegProgressState{state: minidownload.FFmpegState{State: "found", Path: "ffmpeg"}},
		width:       80,
		height:      14,
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
