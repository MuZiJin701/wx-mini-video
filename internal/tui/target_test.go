package tui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/interceptor"
	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

type targetProxyContext struct{ appID string }

func (c *targetProxyContext) Req() *proxy.ContextReq {
	return &proxy.ContextReq{
		URL:    &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"},
		Header: http.Header{"Referer": {"https://servicewechat.com/" + c.appID + "/1/page-frame.html"}},
	}
}
func (c *targetProxyContext) Res() *proxy.ContextRes              { return nil }
func (c *targetProxyContext) Mock(int, map[string]string, string) {}
func (c *targetProxyContext) GetResponseHeader(string) string     { return "video/mp4" }
func (c *targetProxyContext) SetResponseHeader(string, string)    {}
func (c *targetProxyContext) SetResponseBody(string)              {}
func (c *targetProxyContext) GetResponseBody() ([]byte, error)    { return nil, nil }
func (c *targetProxyContext) SetStatusCode(int)                   {}

func targetRuntime(t *testing.T) *app.Runtime {
	t.Helper()
	dir := t.TempDir()
	return &app.Runtime{
		Settings:   config.AppSettings{DownloadDir: dir, ProxyHostname: "127.0.0.1"},
		Store:      miniprogram.NewStore(),
		Downloader: minidownload.New(dir),
	}
}

// testModel 返回启动初始状态：统一目标选择区展开、输入框聚焦。
func testModel(t *testing.T) model {
	t.Helper()
	runtime := targetRuntime(t)
	m := model{
		runtime:        runtime,
		hits:           newHitState(),
		downloaded:     make(map[string]string),
		width:          100,
		height:         30,
		category:       categoryAll,
		appearance:     ui.LoadAppearance(runtime.Settings.DownloadDir),
		targetExpanded: true,
		focus:          focusInput,
	}
	m.applyTheme()
	return m
}

func testModelWith(t *testing.T, runtime *app.Runtime) model {
	t.Helper()
	m := model{
		runtime:        runtime,
		hits:           newHitState(),
		downloaded:     make(map[string]string),
		width:          100,
		height:         30,
		category:       categoryAll,
		appearance:     ui.LoadAppearance(runtime.Settings.DownloadDir),
		targetExpanded: true,
		focus:          focusInput,
	}
	m.applyTheme()
	return m
}

// mediaTestModel 返回已确认目标、处于媒体视图的模型。
func mediaTestModel(t *testing.T) model {
	t.Helper()
	m := testModel(t)
	m.targetReady = true
	m.targetExpanded = false
	m.started = true
	return m
}

func targetKey(m model, key tea.KeyType, text string) (model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: key, Runes: []rune(text)})
	return next.(model), cmd
}

// typeText 模拟一次输入（Bubble Tea 粘贴/输入都携带 Runes）。
func typeText(m model, text string) (model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return next.(model), cmd
}

func TestStartupDiscoveryDoesNotConfirmPrefilledTarget(t *testing.T) {
	runtime := targetRuntime(t)
	runtime.Settings.Target = miniprogram.Target{AppID: "wx123"}
	prefill := prepareDiscoveryTarget(runtime, runtime.Settings.Target.AppID)
	if prefill != "wx123" {
		t.Fatalf("prefilled AppID should stay in the input, got %q", prefill)
	}
	if runtime.Settings.Target.AppID != "" {
		t.Fatalf("startup discovery must not confirm the prefilled target, got %q", runtime.Settings.Target.AppID)
	}
	if got := runtime.Store.Programs(); len(got) != 0 {
		t.Fatal("clearing the target must not fabricate discovered programs")
	}
}

func TestStartupAutoStartsProxyAndShowsDiscoveryArea(t *testing.T) {
	m := testModel(t)
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init should schedule the automatic proxy start")
	}
	updated, cmd := m.Update(startupMsg{})
	m = updated.(model)
	if !m.starting || cmd == nil {
		t.Fatalf("startup message should start the proxy: starting=%v cmd=%v", m.starting, cmd)
	}
	view := m.View()
	for _, want := range []string{"目标小程序", "AppID", "已发现的小程序", "正在启动代理", "确认目标"} {
		if !strings.Contains(view, want) {
			t.Fatalf("target area missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Ctrl+D") {
		t.Fatal("Ctrl+D mode switching should be removed")
	}
}

func TestManualTargetTrimsAppIDAndCollapses(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, runtime.Store, runtime.Settings.DownloadDir)
	m := testModelWith(t, runtime)
	m.started = true
	m, _ = typeText(m, "  wx123  ")
	if m.appIDInput != "  wx123  " {
		t.Fatalf("input should accept text verbatim, got %q", m.appIDInput)
	}
	m, cmd := targetKey(m, tea.KeyEnter, "")
	if cmd != nil {
		t.Fatal("confirming a target with a running proxy should not restart it")
	}
	if !m.targetReady || m.targetExpanded || runtime.Settings.Target.AppID != "wx123" {
		t.Fatalf("manual target not confirmed/collapsed: ready=%v expanded=%v target=%#v", m.targetReady, m.targetExpanded, runtime.Settings.Target)
	}
	if len(runtime.Store.Programs()) != 0 {
		t.Fatal("manual entry should not be presented as an observed Mini Program")
	}
	view := m.View()
	for _, text := range []string{"目标 wx123", "更换目标", "来源未确认"} {
		if !strings.Contains(view, text) {
			t.Fatalf("view missing guidance %q", text)
		}
	}
	if strings.Contains(view, "名称") {
		t.Fatal("name feature should be removed from the UI")
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx456"})
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	if got := runtime.Candidates(); len(got) != 1 || got[0].AppID != "wx123" {
		t.Fatalf("manual AppID was not used for filtering: %#v", got)
	}
}

func TestInputEditingHandlesArrowsAndBusinessKeys(t *testing.T) {
	m := testModel(t)
	m, _ = typeText(m, "wx12")
	m, _ = targetKey(m, tea.KeyLeft, "")
	m, _ = targetKey(m, tea.KeyBackspace, "")
	m, _ = typeText(m, "9")
	if m.appIDInput != "wx92" || m.inputCursor != 3 {
		t.Fatalf("editing result = %q cursor=%d", m.appIDInput, m.inputCursor)
	}
	// 业务单键不得吞掉 AppID 输入：q/d/s 在输入聚焦时都是文本。
	m, _ = typeText(m, "qds")
	if m.appIDInput != "wx9qds2" {
		t.Fatalf("business keys leaked into shortcuts: %q", m.appIDInput)
	}
	if m.confirmQuit || m.stopping {
		t.Fatal("business keys must not trigger actions while editing")
	}
	m, _ = targetKey(m, tea.KeyHome, "")
	if m.inputCursor != 0 {
		t.Fatalf("home cursor = %d", m.inputCursor)
	}
	m, _ = targetKey(m, tea.KeyDelete, "")
	if m.appIDInput != "x9qds2" {
		t.Fatalf("delete at home = %q", m.appIDInput)
	}
	m, _ = targetKey(m, tea.KeyEnd, "")
	if m.inputCursor != len([]rune("x9qds2")) {
		t.Fatalf("end cursor = %d", m.inputCursor)
	}
	m, _ = targetKey(m, tea.KeyCtrlU, "")
	if m.appIDInput != "" || m.inputCursor != 0 {
		t.Fatal("ctrl+u should clear the input")
	}
}

func TestFocusCyclingCoversInputProgramsButtons(t *testing.T) {
	m := testModel(t)
	m.programs = []miniprogram.Program{{AppID: "wx111", LastSeen: time.Now()}}
	m.selectedProgram = "wx111"
	m.targetReady = true
	// 焦点顺序：外观 → 输入 → 发现列表 → 确认 → 取消（更换目标时）。
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.focus != focusPrograms {
		t.Fatalf("tab from input should focus the discovery list, got %v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.focus != focusConfirm {
		t.Fatalf("tab should focus confirm, got %v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.focus != focusCancel {
		t.Fatalf("tab should focus cancel while switching, got %v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.focus != focusAppearance {
		t.Fatalf("tab should wrap to appearance, got %v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyTab, "")
	if m.focus != focusInput {
		t.Fatalf("tab should reach input, got %v", m.focus)
	}
	m, _ = targetKey(m, tea.KeyShiftTab, "")
	if m.focus != focusAppearance {
		t.Fatalf("shift+tab should move backwards to appearance, got %v", m.focus)
	}
	if view := m.View(); !strings.Contains(view, "取消") {
		t.Fatal("switch mode should offer a cancel action")
	}
}

func TestDiscoveryListSelectionUsesAppIDIdentity(t *testing.T) {
	m := testModel(t)
	m.started = true
	now := time.Now()
	m.programs = []miniprogram.Program{
		{AppID: "wx111", FirstSeen: now, LastSeen: now},
		{AppID: "wx222", FirstSeen: now, LastSeen: now.Add(-time.Second)},
	}
	m.selectedProgram = "wx222"
	m.focus = focusPrograms
	_ = m.View()
	// 发现结果重排：wx111 变成最近出现，选中身份必须仍是 wx222。
	m.programs = []miniprogram.Program{
		{AppID: "wx111", FirstSeen: now, LastSeen: now.Add(3 * time.Second)},
		{AppID: "wx222", FirstSeen: now, LastSeen: now},
	}
	m = updateTick(t, m)
	if m.selectedProgram != "wx222" || m.focus != focusPrograms {
		t.Fatalf("discovery update stole selection/focus: program=%q focus=%v", m.selectedProgram, m.focus)
	}
	m, cmd := targetKey(m, tea.KeyEnter, "")
	if cmd != nil || !m.targetReady || m.runtime.Settings.Target.AppID != "wx222" || m.targetExpanded {
		t.Fatalf("Enter on the focused list should confirm the selected program: cmd=%v target=%q", cmd, m.runtime.Settings.Target.AppID)
	}
}

func TestDiscoveryNavigationAndConfirmButton(t *testing.T) {
	m := testModel(t)
	m.started = true
	now := time.Now()
	m.programs = []miniprogram.Program{
		{AppID: "wx111", LastSeen: now},
		{AppID: "wx222", LastSeen: now},
	}
	m.selectedProgram = "wx111"
	m.focus = focusPrograms
	m, _ = targetKey(m, tea.KeyDown, "")
	if m.selectedProgram != "wx222" {
		t.Fatalf("down should select the next program, got %q", m.selectedProgram)
	}
	m.focus = focusConfirm
	m, _ = targetKey(m, tea.KeyEnter, "")
	if m.runtime.Settings.Target.AppID != "wx222" {
		t.Fatalf("confirm button should use the selected program: target=%q", m.runtime.Settings.Target.AppID)
	}
}

func TestTargetSwitchExpandCancelAndDownloadGuard(t *testing.T) {
	runtime := targetRuntime(t)
	runtime.SetTarget(miniprogram.Target{AppID: "wx123"})
	for _, name := range []string{"saved.mp4", "unfinished.part", "history.jsonl"} {
		path := filepath.Join(runtime.Settings.DownloadDir, name)
		if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := testModelWith(t, runtime)
	m.targetReady = true
	m.targetExpanded = false
	m.started = true
	m.appIDInput = "wx123"
	m.candidates = []miniprogram.Candidate{{ID: "a", URL: "https://cdn.example.com/a.jpg", Kind: "image", AppID: "wx123"}}
	m, _ = targetKey(m, tea.KeyRunes, "s")
	if !m.targetExpanded || !m.targetReady || len(m.candidates) != 1 {
		t.Fatal("opening the target editor must not clear the current target or candidates")
	}
	m.appIDInput = "wx456"
	m, _ = targetKey(m, tea.KeyEsc, "")
	if m.targetExpanded || !m.targetReady || len(m.candidates) != 1 {
		t.Fatal("cancelling the target editor changed the target or candidates")
	}
	if m.appIDInput != "wx456" {
		t.Fatal("cancel should preserve the already-entered text")
	}
	for _, name := range []string{"saved.mp4", "unfinished.part", "history.jsonl"} {
		data, err := os.ReadFile(filepath.Join(runtime.Settings.DownloadDir, name))
		if err != nil || string(data) != "keep" {
			t.Fatalf("switch changed %s: %v", name, err)
		}
	}
	m.downloading = true
	m.targetExpanded = false
	m, cmd := targetKey(m, tea.KeyRunes, "s")
	if cmd != nil || m.targetExpanded || !m.downloading {
		t.Fatal("target switch interrupted an active download")
	}
	if !strings.Contains(m.notice, "下载正在进行") {
		t.Fatalf("blocked switch should show its reason, notice=%q", m.notice)
	}
}

func TestQuitDuringDownloadStopsRecordingThenExits(t *testing.T) {
	m := mediaTestModel(t)
	m.downloading = true
	m, cmd := targetKey(m, tea.KeyRunes, "q")
	if cmd != nil || !m.confirmQuit || m.stopping {
		t.Fatal("quitting during a download should ask for confirmation")
	}
	m, _ = targetKey(m, tea.KeyEsc, "")
	if m.confirmQuit || m.stopping {
		t.Fatal("cancelling the quit confirmation did not restore the view")
	}
	m, _ = targetKey(m, tea.KeyRunes, "q")
	m, cmd = targetKey(m, tea.KeyEnter, "")
	if cmd != nil || m.stopping || !m.quitAfterDownload {
		t.Fatal("confirmed quit should stop the download before exiting")
	}
	next, stopCmd := m.Update(downloadMsg{candidateID: "recording", path: "downloads/wx/直播/直播_1.flv", err: minidownload.ErrRecordingStopped})
	m = next.(model)
	if !m.stopping || stopCmd == nil || m.quitAfterDownload {
		t.Fatal("quit should resume after the recording is finalized")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{{110, 40}, {80, 24}, {60, 20}}
	for _, size := range sizes {
		m := testModel(t)
		m.width, m.height = size.width, size.height
		m.started = true
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d target view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, m, lines)

		media := mediaTestModel(t)
		media.width, media.height = size.width, size.height
		media.candidates = testCandidates(3)
		media.candidates[0].Attribution = miniprogram.AttributionConfirmed
		media.candidates[1].Attribution = miniprogram.AttributionUnattributed
		lines = strings.Split(media.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d media view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, media, lines)

		details := media
		details.showDetails = true
		lines = strings.Split(details.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d details view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, details, lines)

		logs := media
		logs.showLogs = true
		logs.logs = []string{"13:00:01 启动代理", "13:00:02 已发现小程序"}
		lines = strings.Split(logs.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d logs view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, logs, lines)
	}
}

func assertLineWidths(t *testing.T, m model, lines []string) {
	t.Helper()
	for i, line := range lines {
		if got := ui.Width(line); got != m.width {
			t.Fatalf("%dx%d line %d width = %d, want %d: %q", m.width, m.height, i, got, m.width, line)
		}
	}
}

func updateTick(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(tickMsg{})
	return next.(model)
}

func testCandidates(count int) []miniprogram.Candidate {
	out := make([]miniprogram.Candidate, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, miniprogram.Candidate{
			ID:            strings.Repeat("a", 8) + string(rune('0'+i)),
			Title:         "候选标题",
			URL:           "https://cdn.example.com/video.mp4",
			Kind:          "video",
			AppID:         "wx123",
			Source:        "response",
			ContentLength: 1234,
			Attribution:   miniprogram.AttributionConfirmed,
		})
	}
	return out
}
