package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
