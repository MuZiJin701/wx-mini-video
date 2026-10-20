package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"wx_channel/pkg/certificate"
)

var legacyRootPresent = func() (bool, error) { return certificate.CheckHasCertificate(certificate.LegacyRootThumbprint) }
var removeLegacyRoot = certificate.RemoveLegacyRoot

type stopMsg struct{ err error }
type legacyCleanupMsg struct{ err error }

func removeLegacyRootCmd() tea.Msg { return legacyCleanupMsg{err: removeLegacyRoot()} }

func (m model) requestQuit() (tea.Model, tea.Cmd) {
	m.stopErr = nil
	m.stopping = true
	if m.starting {
		return m, nil
	}
	return m, m.stopProxy
}
