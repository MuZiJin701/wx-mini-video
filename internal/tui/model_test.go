package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

func TestCategoryFilteringAndCounts(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.candidates = []miniprogram.Candidate{
		{ID: "a", URL: "https://cdn.example.com/a.jpg", Kind: "image", AppID: "wx123"},
		{ID: "b", URL: "https://cdn.example.com/b.mp4", Kind: "video", AppID: "wx123"},
		{ID: "c", URL: "https://cdn.example.com/c.m3u8", Kind: "m3u8", AppID: "wx123"},
	}
	m.otherCandidates = []miniprogram.Candidate{
		{ID: "d", URL: "https://cdn.example.com/d.mp3", Kind: "audio", SourceAppID: "wx999", Attribution: miniprogram.AttributionUnattributed},
	}
	if got := m.categoryCount(categoryAll); got != 4 {
		t.Fatalf("all count = %d", got)
	}
	if got := m.categoryCount(categoryImage); got != 1 {
		t.Fatalf("image count = %d", got)
	}
	m.setCategory(categoryAudio)
	visible := m.filteredCandidates()
	if len(visible) != 1 || visible[0].ID != "d" {
		t.Fatalf("audio filter = %#v", visible)
	}
	m.setCategory(categoryAll)
	m.nextCategory(1)
	if m.activeCategory() != categoryImage {
		t.Fatalf("tab cycle landed on %q", m.activeCategory())
	}
	view := m.View()
	if !strings.Contains(view, "1 全部(4)") || !strings.Contains(view, "?") {
		t.Fatal("tabs or attribution marker missing from view")
	}
}

func TestDownloadedMarkerAndLocalPathInDrawer(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.candidates = testCandidates(2)
	m.downloaded[m.candidates[0].ID] = "downloads/wx123/视频/视频_20261025-130000_aaaaaaaa.mp4"
	m.showDetails = true
	view := m.View()
	if !strings.Contains(view, "已下载") {
		t.Fatal("downloaded marker missing")
	}
	if !strings.Contains(view, "本地路径") {
		t.Fatal("detail drawer should show the local path")
	}
	if !strings.Contains(view, "本地缓存") {
		t.Fatal("detail drawer should keep cache info")
	}
}

func TestAppearanceSwitchPersistsToUIJSON(t *testing.T) {
	m := testModel(t)
	if m.appearance != ui.AppearanceSystem {
		t.Fatalf("default appearance = %q", m.appearance)
	}
	cmd := m.setAppearance(ui.AppearanceDark)
	if !m.dark || cmd == nil {
		t.Fatal("switching to dark did not apply immediately")
	}
	msg := cmd()
	if saved, ok := msg.(appearanceSavedMsg); !ok || saved.err != nil {
		t.Fatalf("saving appearance failed: %#v", msg)
	}
	if got := ui.LoadAppearance(m.runtime.Settings.DownloadDir); got != ui.AppearanceDark {
		t.Fatalf("persisted appearance = %q", got)
	}
	cmd = m.cycleAppearance(-1)
	if m.appearance != ui.AppearanceLight {
		t.Fatalf("left should cycle back to light, got %q", m.appearance)
	}
	if cmd == nil {
		t.Fatal("appearance change should schedule a save")
	}
}

func TestArrowKeysCycleAppearance(t *testing.T) {
	m := testModel(t)
	m, _ = targetKey(m, tea.KeyRight, "")
	if m.appearance != ui.AppearanceLight {
		t.Fatalf("right arrow should select light, got %q", m.appearance)
	}
	m, _ = targetKey(m, tea.KeyRight, "")
	if m.appearance != ui.AppearanceDark {
		t.Fatalf("right arrow should select dark, got %q", m.appearance)
	}
	m, _ = targetKey(m, tea.KeyLeft, "")
	if m.appearance != ui.AppearanceLight {
		t.Fatalf("left arrow should return to light, got %q", m.appearance)
	}
}

func TestMouseClicksAppearanceAndTabs(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.candidates = testCandidates(2)
	m.width, m.height = 110, 40
	_ = m.View() // 生成命中区域

	darkHit, ok := findHit(m, hitAppearanceDark)
	if !ok {
		t.Fatal("appearance control hit region missing")
	}
	m = clickHit(t, m, darkHit)
	if m.appearance != ui.AppearanceDark {
		t.Fatalf("clicking dark segment did not switch appearance: %q", m.appearance)
	}

	_ = m.View()
	tabHit, ok := findHit(m, hitTab)
	if !ok {
		t.Fatal("category tab hit region missing")
	}
	m = clickHit(t, m, tabHit)
	if m.activeCategory() != categoryAll {
		t.Fatalf("clicking first tab should select all, got %q", m.activeCategory())
	}
}

func TestMouseClicksTargetOverlayProgram(t *testing.T) {
	m := testModel(t)
	m.started = true
	now := time.Now()
	m.programs = []miniprogram.Program{
		{AppID: "wx111", LastSeen: now},
		{AppID: "wx222", LastSeen: now},
	}
	m.showTargets = true
	_ = m.View()
	hit, ok := findHitIndex(m, hitOverlayProgram, 1)
	if !ok {
		t.Fatal("program row hit region missing")
	}
	m = clickHit(t, m, hit)
	if !m.targetReady || m.runtime.Settings.Target.AppID != "wx222" {
		t.Fatalf("clicking a program should confirm it, target=%q", m.runtime.Settings.Target.AppID)
	}
}

func findHit(m model, action hitAction) (hitRegion, bool) {
	return findHitIndex(m, action, 0)
}

func findHitIndex(m model, action hitAction, index int) (hitRegion, bool) {
	for _, region := range m.hits.regions {
		if region.action == action && region.index == index {
			return region, true
		}
	}
	return hitRegion{}, false
}

func clickHit(t *testing.T, m model, region hitRegion) model {
	t.Helper()
	x := region.x + region.w/2
	y := region.y
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	return next.(model)
}

func TestViewLinesFitWidth(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 110, 40
	m.targetReady = true
	m.candidates = testCandidates(3)
	for _, line := range strings.Split(m.View(), "\n") {
		if ui.Width(line) != m.width {
			t.Fatalf("line width = %d, want %d: %q", ui.Width(line), m.width, line)
		}
	}
}

func TestProgressAndNoticeStatus(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.downloading = true
	m.progress = minidownload.Progress{Status: "downloading", Downloaded: 1200, Total: 4800}
	view := m.View()
	if !strings.Contains(view, "下载中 25%") {
		t.Fatalf("progress percentage missing: %q", view)
	}
	m.downloading = false
	m.progress = minidownload.Progress{}
	m.setNotice("已保存 → downloads/wx123/图片/图片_20261025-130000_aaaaaaaa.jpg", time.Minute)
	_ = m.View()
	if !strings.Contains(m.notice, "已保存") {
		t.Fatal("notice not set")
	}
}

func TestNoNameReferencesAnywhere(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 110, 40
	if strings.Contains(m.View(), "名称") {
		t.Fatal("name feature still referenced in target view")
	}
	m.targetReady = true
	m.candidates = testCandidates(2)
	m.showDetails = true
	if strings.Contains(m.View(), "名称") {
		t.Fatal("name feature still referenced in media view")
	}
	m.showDetails = false
	m.programs = []miniprogram.Program{{AppID: "wx123", LastSeen: time.Now()}}
	m.showTargets = true
	if strings.Contains(m.View(), "名称") {
		t.Fatal("name feature still referenced in target overlay")
	}
}

func TestHoverStateTracksRegions(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.candidates = testCandidates(1)
	m.width, m.height = 110, 40
	_ = m.View()
	tabHit, ok := findHit(m, hitTab)
	if !ok {
		t.Fatal("tab hit missing")
	}
	next, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: tabHit.x + 1, Y: tabHit.y})
	hovered := next.(model)
	if !hovered.hits.isHovered(hitTab, tabHit.index) {
		t.Fatal("motion over a tab did not set hover state")
	}
	next, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionMotion, X: 0, Y: 0})
	if next.(model).hits.hoverAction != hitNone {
		t.Fatal("hover state not cleared outside regions")
	}
}
