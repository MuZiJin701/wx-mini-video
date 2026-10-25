package tui

import (
	"strings"
	"testing"

	"wx_channel/internal/tui/ui"
)

func TestActionsAndFooterStayVisible(t *testing.T) {
	for _, size := range []struct{ width, height int }{{110, 40}, {110, 24}, {80, 24}, {60, 20}} {
		m := mediaTestModel(t)
		m.width, m.height = size.width, size.height
		m.candidates = testCandidates(2)
		m.downloading = true
		stopped := false
		m.downloadCancel = func() { stopped = true }
		lines := strings.Split(m.View(), "\n")
		assertLineWidths(t, m, lines)
		for _, label := range []string{"下载 · d", "停止 · x", "详情 · i", "日志 · l", "刷新 · r", "打开目录 · o", "清空 · c", "更换目标 · s", "退出 · Ctrl+Q"} {
			findLineContaining(t, lines, label)
		}
		stop, ok := findHit(m, hitActionStop)
		if !ok || stop.y+stop.h > m.height-m.footerHeight(m.metricsNow()) {
			t.Fatalf("%dx%d: stop button missing or clipped: %+v", size.width, size.height, stop)
		}
		m = clickHitRegion(t, m, stop)
		if !stopped {
			t.Fatal("stop button did not cancel the download")
		}
		quit, ok := findHit(m, hitActionQuit)
		quitRow := findLineContaining(t, lines, "退出 · Ctrl+Q")
		if !ok || quit.h != 3 || quitRow != quit.y+1 {
			t.Fatalf("quit button is not framed or its hit region is misaligned: %+v", quit)
		}
		link, ok := findHit(m, hitGitHub)
		linkRow := findLineContaining(t, lines, "GitHub")
		if !ok || link.y != linkRow {
			t.Fatalf("GitHub hit row %d differs from rendered row %d", link.y, linkRow)
		}
		if got := ui.Width(strings.Split(lines[quitRow], "退出")[0]); got < quit.x {
			t.Fatal("quit label is left of its clickable button")
		}
	}
}

func TestTargetAreaHasGlobalShortcutButtons(t *testing.T) {
	for _, size := range []struct{ width, height int }{{110, 40}, {80, 24}, {60, 20}} {
		m := testModel(t)
		m.width, m.height = size.width, size.height
		lines := strings.Split(m.View(), "\n")
		for _, action := range []hitAction{hitActionRefresh, hitActionOpenDir, hitTargetConfirm} {
			region, ok := findHit(m, action)
			if !ok || region.y+region.h > m.height-m.footerHeight(m.metricsNow()) {
				t.Fatalf("%dx%d: target action %d missing or clipped", size.width, size.height, action)
			}
		}
		findLineContaining(t, lines, "Ctrl+R")
		findLineContaining(t, lines, "Ctrl+O")
	}
}
