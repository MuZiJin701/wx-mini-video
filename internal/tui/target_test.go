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

func testModel(t *testing.T) model {
	t.Helper()
	runtime := targetRuntime(t)
	m := model{
		runtime:    runtime,
		hits:       newHitState(),
		downloaded: make(map[string]string),
		width:      100,
		height:     30,
		category:   categoryAll,
		appearance: ui.LoadAppearance(runtime.Settings.DownloadDir),
	}
	m.applyTheme()
	return m
}

func testModelWith(t *testing.T, runtime *app.Runtime) model {
	t.Helper()
	m := model{
		runtime:    runtime,
		hits:       newHitState(),
		downloaded: make(map[string]string),
		width:      100,
		height:     30,
		category:   categoryAll,
		appearance: ui.LoadAppearance(runtime.Settings.DownloadDir),
	}
	m.applyTheme()
	return m
}

func targetKey(m model, key tea.KeyType, text string) (model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: key, Runes: []rune(text)})
	return next.(model), cmd
}

func TestManualTargetTrimsAppIDWithoutName(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, runtime.Store, runtime.Settings.DownloadDir)
	m := testModelWith(t, runtime)
	m.started = true
	m.appIDInput = "  wx123  "
	if view := m.View(); !strings.Contains(view, "更多资料") || !strings.Contains(view, "Ctrl+D") {
		t.Fatalf("target card should explain how to find an AppID: %q", view)
	}
	m, cmd := targetKey(m, tea.KeyEnter, "")
	if cmd != nil {
		t.Fatal("confirming a target with a running proxy should not restart it")
	}
	if !m.targetReady || runtime.Settings.Target.AppID != "wx123" {
		t.Fatalf("manual target not trimmed: %#v", runtime.Settings.Target)
	}
	if len(runtime.Store.Programs()) != 0 {
		t.Fatal("manual entry should not be presented as an observed Mini Program")
	}
	view := m.View()
	for _, text := range []string{"目标 wx123", "来源未确认"} {
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

func TestTargetOverlayListsProgramsAndConfirmsImmediately(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, runtime.Store, runtime.Settings.DownloadDir)
	m := testModelWith(t, runtime)
	m, cmd := targetKey(m, tea.KeyCtrlD, "")
	if !m.showTargets || cmd == nil {
		t.Fatal("Ctrl+D should open the target overlay and start the proxy")
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx111"})
	plugin.OnResponse(&targetProxyContext{appID: "wx222"})
	m, _ = targetKey(m, tea.KeyRunes, "")
	m = updateTick(t, m)
	view := m.View()
	for _, text := range []string{"选择小程序", "wx111", "wx222", "最近出现"} {
		if !strings.Contains(view, text) {
			t.Fatalf("target overlay missing %q", text)
		}
	}
	m.programIdx = 1
	m, _ = targetKey(m, tea.KeyEnter, "")
	if !m.targetReady || runtime.Settings.Target.AppID != "wx222" || m.showTargets {
		t.Fatalf("selecting a discovered program should confirm immediately, target=%q", runtime.Settings.Target.AppID)
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx222"})
	if got := runtime.Candidates(); len(got) != 1 {
		t.Fatalf("confirmed target should collect media, got %d", len(got))
	}
}

func updateTick(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(tickMsg{})
	return next.(model)
}

func TestTargetSwitchOverlayKeepsCandidatesUntilConfirmed(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, runtime.Store, runtime.Settings.DownloadDir)
	runtime.SetTarget(miniprogram.Target{AppID: "wx123"})
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	m := testModelWith(t, runtime)
	m.targetReady = true
	m.started = true
	m.candidates = runtime.Candidates()

	m, _ = targetKey(m, tea.KeyRunes, "s")
	if !m.showTargets || !m.targetReady || len(m.candidates) != 1 {
		t.Fatal("opening the target overlay must not clear the current target or candidates")
	}
	m, _ = targetKey(m, tea.KeyEsc, "")
	if m.showTargets || !m.targetReady || len(m.candidates) != 1 {
		t.Fatal("cancelling the target overlay changed the target")
	}

	time.Sleep(2 * time.Millisecond)
	plugin.OnResponse(&targetProxyContext{appID: "wx456"})
	m = updateTick(t, m)
	m, _ = targetKey(m, tea.KeyRunes, "s")
	for i, program := range m.programs {
		if program.AppID == "wx456" {
			m.programIdx = i
			break
		}
	}
	m, _ = targetKey(m, tea.KeyEnter, "")
	if runtime.Settings.Target.AppID != "wx456" || len(m.candidates) != 0 {
		t.Fatalf("confirming a new target should clear candidates, target=%q", runtime.Settings.Target.AppID)
	}
}

func TestSwitchClearsCandidatesButKeepsFiles(t *testing.T) {
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
	m.started = true
	m.candidates = []miniprogram.Candidate{{ID: "a", URL: "https://cdn.example.com/a.jpg", Kind: "image", AppID: "wx123"}}
	m, _ = targetKey(m, tea.KeyRunes, "s")
	m.programIdx = 0
	if runtime.Settings.Target.AppID == "" {
		t.Fatal("target switch should keep the previous target until confirmation")
	}
	for _, name := range []string{"saved.mp4", "unfinished.part", "history.jsonl"} {
		data, err := os.ReadFile(filepath.Join(runtime.Settings.DownloadDir, name))
		if err != nil || string(data) != "keep" {
			t.Fatalf("switch changed %s: %v", name, err)
		}
	}
}

func TestDownloadInProgressBlocksTargetSwitch(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
	m.started = true
	m.downloading = true
	m, cmd := targetKey(m, tea.KeyRunes, "s")
	if cmd != nil || m.showTargets || !m.downloading {
		t.Fatal("target switch interrupted an active download")
	}
	if !strings.Contains(m.notice, "下载正在进行") {
		t.Fatalf("blocked switch should show its reason, notice=%q", m.notice)
	}
}

func TestQuitDuringDownloadRequiresConfirmation(t *testing.T) {
	m := testModel(t)
	m.targetReady = true
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
	if cmd == nil || !m.stopping || m.confirmQuit {
		t.Fatal("confirmed quit should start the stop flow")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{{110, 40}, {80, 24}, {60, 20}}
	for _, size := range sizes {
		m := testModel(t)
		m.width, m.height = size.width, size.height
		m.targetReady = false
		lines := strings.Split(m.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d target view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, m, lines)
		m.targetReady = true
		m.candidates = testCandidates(3)
		m.candidates[0].Attribution = miniprogram.AttributionConfirmed
		m.candidates[1].Attribution = miniprogram.AttributionUnattributed
		lines = strings.Split(m.View(), "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d media view has %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, m, lines)
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
