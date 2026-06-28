package tui

import (
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
