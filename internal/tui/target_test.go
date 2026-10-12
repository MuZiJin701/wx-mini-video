package tui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/interceptor"
	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
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

func targetKey(m model, key tea.KeyType, text string) (model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: key, Runes: []rune(text)})
	return next.(model), cmd
}

func TestDiscoveryRequiresExplicitSelection(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, runtime.Store, runtime.Settings.DownloadDir)
	m := model{runtime: runtime, width: 100, height: 24}
	m, cmd := targetKey(m, tea.KeyCtrlD, "")
	if cmd == nil || !strings.Contains(m.View(), "小程序发现") {
		t.Fatal("Ctrl+D should enter discovery and schedule proxy startup")
	}
	next, _ := m.Update(startMsg{})
	m = next.(model)
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	next, _ = m.Update(tickMsg{})
	m = next.(model)
	if m.targetReady || len(runtime.Store.List("")) != 0 {
		t.Fatal("discovery selected a target or collected media automatically")
	}
	if view := m.View(); strings.Count(view, "wx123") != 1 || !strings.Contains(view, "名称未识别") {
		t.Fatalf("discovery should show one unnamed AppID, got %q", view)
	}
	m, _ = targetKey(m, tea.KeyEnter, "")
	m, _ = targetKey(m, tea.KeyRunes, "测试小程序")
	m, _ = targetKey(m, tea.KeyEnter, "")
	if !m.targetReady || runtime.Settings.Target.AppID != "wx123" || runtime.Settings.Target.Name != "测试小程序" {
		t.Fatal("discovered target should allow name entry and explicit confirmation")
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	if got := runtime.Store.List(""); len(got) != 1 || got[0].AppName != "测试小程序" {
		t.Fatalf("selected target should collect reloaded media with its name, got %#v", got)
	}
}

func TestTargetSwitchClearsCandidatesAndKeepsFiles(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123", Name: "同名小程序"}, runtime.Store, runtime.Settings.DownloadDir)
	runtime.SetTarget(miniprogram.Target{AppID: "wx123", Name: "同名小程序"})
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	for _, name := range []string{"saved.mp4", "unfinished.part", "cached.m3u8", "history.jsonl"} {
		if err := os.WriteFile(filepath.Join(runtime.Settings.DownloadDir, name), []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := model{runtime: runtime, started: true, targetReady: true, candidates: runtime.Candidates(), progress: minidownload.Progress{Status: "done"}, downloadPath: "saved.mp4", width: 100, height: 24}
	m, _ = targetKey(m, tea.KeyRunes, "s")
	if m.targetReady || len(runtime.Store.List("")) != 0 || m.progress.Status != "" || m.downloadPath != "" {
		t.Fatal("switch should return to target selection and clear candidates and progress")
	}
	m, _ = targetKey(m, tea.KeyCtrlD, "")
	m, _ = targetKey(m, tea.KeyRunes, "wx456")
	m, _ = targetKey(m, tea.KeyTab, "")
	m, _ = targetKey(m, tea.KeyRunes, "同名小程序")
	m, cmd := targetKey(m, tea.KeyEnter, "")
	if cmd != nil {
		t.Fatal("switch should reuse the running proxy")
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	plugin.OnResponse(&targetProxyContext{appID: "wx456"})
	if got := runtime.Candidates(); len(got) != 1 || got[0].AppID != "wx456" || got[0].AppName != "同名小程序" {
		t.Fatalf("same-name targets should filter by the new AppID, got %#v", got)
	}
	for _, name := range []string{"saved.mp4", "unfinished.part", "cached.m3u8", "history.jsonl"} {
		data, err := os.ReadFile(filepath.Join(runtime.Settings.DownloadDir, name))
		if err != nil || string(data) != "keep" {
			t.Fatalf("switch changed %s: %v", name, err)
		}
	}
}

func TestDownloadInProgressBlocksTargetSwitch(t *testing.T) {
	runtime := targetRuntime(t)
	runtime.SetTarget(miniprogram.Target{AppID: "wx123"})
	m := model{runtime: runtime, targetReady: true, started: true, downloading: true}
	m, cmd := targetKey(m, tea.KeyRunes, "s")
	if cmd != nil || !m.targetReady || !m.downloading || runtime.Settings.Target.AppID != "wx123" {
		t.Fatal("target switch interrupted an active download")
	}
	if !m.showLogs || !strings.Contains(strings.Join(m.logs, "\n"), "下载正在进行") {
		t.Fatal("blocked switch should show its reason even when logs were hidden")
	}
}

func TestManualTargetUsesTrimmedAppIDWithOptionalName(t *testing.T) {
	runtime := targetRuntime(t)
	plugin := interceptor.CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, runtime.Store, runtime.Settings.DownloadDir)
	m := model{runtime: runtime, started: true, appIDInput: "  wx123  ", width: 100, height: 24}
	m, cmd := targetKey(m, tea.KeyEnter, "")
	if !m.targetReady || cmd != nil || runtime.Settings.Target.AppID != "wx123" || runtime.Settings.Target.Name != "" {
		t.Fatal("manual confirmation should trim AppID and allow an empty name")
	}
	if len(runtime.Store.Programs()) != 0 {
		t.Fatal("manual entry should not be presented as an observed Mini Program")
	}
	plugin.OnResponse(&targetProxyContext{appID: "wx456"})
	plugin.OnResponse(&targetProxyContext{appID: "wx123"})
	if got := runtime.Candidates(); len(got) != 1 || got[0].AppID != "wx123" {
		t.Fatalf("manual AppID was not used for filtering: %#v", got)
	}
	for _, text := range []string{"更多资料", "重新打开", "来源不明"} {
		view := m.View() + m.renderTargetInput() + strings.Join(m.logs, "\n")
		if !strings.Contains(view, text) {
			t.Fatalf("missing user guidance %q", text)
		}
	}
}
