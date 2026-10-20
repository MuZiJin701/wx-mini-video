package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"wx_channel/pkg/certificate"
)

func TestQuitWaitsForStartupAndNeverStartsAfterQuit(t *testing.T) {
	for _, finished := range []tea.Msg{startMsg{}, startMsg{legacy: true}, legacyCleanupMsg{}} {
		m := model{runtime: targetRuntime(t), starting: true}
		next, cmd := m.requestQuit()
		m = next.(model)
		if cmd != nil || !m.stopping {
			t.Fatal("quit raced a pending startup")
		}
		next, cmd = m.Update(finished)
		m = next.(model)
		if cmd == nil || m.legacyPrompt {
			t.Fatal("pending startup did not proceed directly to cleanup")
		}
		if _, ok := cmd().(stopMsg); !ok {
			t.Fatal("startup continued instead of cleaning up")
		}
	}
}

func TestStopFailureRequiresRetryOrExplicitForce(t *testing.T) {
	for _, screen := range []model{{targetReady: true}, {discovering: true}, {}} {
		screen.runtime = targetRuntime(t)
		next, cmd := screen.requestQuit()
		m := next.(model)
		if cmd == nil || !m.stopping {
			t.Fatal("quit did not wait for cleanup")
		}
		next, cmd = m.Update(stopMsg{err: errors.New("restore failed")})
		m = next.(model)
		if cmd != nil || !strings.Contains(m.View(), "按 r 重试") {
			t.Fatal("cleanup error was hidden or quit automatically")
		}
		_, cmd = targetKey(m, tea.KeyRunes, "q")
		if cmd != nil {
			t.Fatal("ordinary quit bypassed failed cleanup")
		}
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		if cmd == nil || !next.(model).stopping {
			t.Fatal("retry unavailable")
		}
		_, cmd = m.Update(stopMsg{})
		if cmd == nil {
			t.Fatal("successful cleanup did not exit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("successful cleanup did not return Quit")
		}
		_, cmd = targetKey(m, tea.KeyRunes, "f")
		if cmd == nil {
			t.Fatal("explicit force exit unavailable")
		}
	}
}

func TestLegacyRootCleanupRequiresConfirmationAndKeepsFailuresVisible(t *testing.T) {
	originalCheck, originalRemove := legacyRootPresent, removeLegacyRoot
	t.Cleanup(func() { legacyRootPresent, removeLegacyRoot = originalCheck, originalRemove })
	legacyRootPresent = func() (bool, error) { return true, nil }
	removals := 0
	removeLegacyRoot = func() error { removals++; return errors.New("access denied") }
	m := model{runtime: targetRuntime(t), discovering: true}
	msg := m.startProxy()
	next, _ := m.Update(msg)
	m = next.(model)
	if !m.legacyPrompt || removals != 0 || !strings.Contains(m.View(), certificate.LegacyRootThumbprint) {
		t.Fatal("legacy cleanup was not explicitly offered")
	}
	kept, cmd := targetKey(m, tea.KeyRunes, "n")
	if cmd == nil || removals != 0 || !strings.Contains(kept.View(), "风险仍然存在") {
		t.Fatal("decline did not preserve legacy trust warning")
	}
	m, cmd = targetKey(m, tea.KeyRunes, "y")
	if removals != 0 || cmd == nil {
		t.Fatal("cleanup ran outside its command")
	}
	next, _ = m.Update(cmd())
	m = next.(model)
	if removals != 1 || !strings.Contains(m.View(), "清理失败") || !strings.Contains(m.View(), "风险仍然存在") {
		t.Fatal("failed cleanup was hidden")
	}
}
