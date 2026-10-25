package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/tui/ui"
)

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.stopErr != nil {
		switch key {
		case "r":
			return m.requestQuit()
		case "f":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.stopping || m.networkPause {
		return m, nil
	}
	if m.confirmQuit {
		switch key {
		case "enter", "y":
			m.confirmQuit = false
			return m.confirmQuitNow()
		case "esc", "n", "q", "ctrl+c":
			m.confirmQuit = false
		}
		return m, nil
	}
	if m.legacyPrompt {
		switch key {
		case "y":
			m.legacyPrompt = false
			m.starting = true
			return m, removeLegacyRootCmd
		case "n":
			m.legacyPrompt = false
			m.legacyChecked = true
			m.legacyWarning = "旧共享根证书尚未清理，其系统信任风险仍然存在。"
			m.starting = true
			return m, m.startProxy
		case "q", "ctrl+c", "esc":
			return m.requestQuit()
		}
		return m, nil
	}
	if m.showTargets {
		return m.handleTargetOverlayKey(msg)
	}

	switch key {
	case "ctrl+q":
		return m.requestQuit()
	case "ctrl+o":
		return m, m.openDirectory()
	case "ctrl+r":
		return m.retryOrRefresh()
	case "left":
		return m, m.cycleAppearance(-1)
	case "right":
		return m, m.cycleAppearance(1)
	}

	if m.showLogs {
		return m.handleLogKey(msg)
	}
	if !m.targetReady {
		return m.handleTargetInputKey(msg)
	}
	return m.handleMediaKey(msg)
}

func (m model) handleTargetInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m.requestQuit()
	case "ctrl+d":
		m.openTargetOverlay()
		return m.ensureProxy()
	case "backspace", "ctrl+h":
		m.appIDInput = trimLastRune(m.appIDInput)
		m.inputErr = ""
		return m, nil
	case "enter":
		return m.confirmTargetInput()
	default:
		if len(msg.Runes) == 0 {
			return m, nil
		}
		m.appIDInput += string(msg.Runes)
		m.inputErr = ""
		return m, nil
	}
}

func (m model) handleTargetOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+d":
		m.showTargets = false
		return m, nil
	case "q", "ctrl+c":
		return m.requestQuit()
	case "up", "k":
		if m.programIdx > 0 {
			m.programIdx--
		}
	case "down", "j":
		if m.programIdx < len(m.programs)-1 {
			m.programIdx++
		}
	case "enter":
		return m.confirmSelectedProgram()
	case "r":
		m.programs = m.runtime.Store.Programs()
		m.clampProgramIndex()
		return m.ensureProxy()
	}
	return m, nil
}

func (m model) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "l":
		m.showLogs = false
	case "q", "ctrl+c":
		return m.requestQuit()
	case "up", "k":
		m.logOffset++
		m.clampLogOffset()
	case "down", "j":
		m.logOffset--
		if m.logOffset < 0 {
			m.logOffset = 0
		}
	case "pgup":
		m.logOffset += 8
		m.clampLogOffset()
	case "pgdown":
		m.logOffset -= 8
		if m.logOffset < 0 {
			m.logOffset = 0
		}
	case "home":
		m.logOffset = len(m.logs)
		m.clampLogOffset()
	case "end":
		m.logOffset = 0
	}
	return m, nil
}

func (m model) handleMediaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m.requestQuit()
	case "esc":
		if m.showDetails {
			m.showDetails = false
		}
		return m, nil
	case "s", "ctrl+d":
		if m.downloading {
			m.setNotice("下载正在进行，完成后才能切换目标。", 5*time.Second)
			return m, nil
		}
		m.openTargetOverlay()
		return m, nil
	case "tab":
		m.nextCategory(1)
	case "shift+tab":
		m.nextCategory(-1)
	case "1":
		m.setCategory(categoryAll)
	case "2":
		m.setCategory(categoryImage)
	case "3":
		m.setCategory(categoryVideo)
	case "4":
		m.setCategory(categoryM3U8)
	case "5":
		m.setCategory(categoryAudio)
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.filteredCandidates())-1 {
			m.selected++
		}
	case "r":
		return m.retryOrRefresh()
	case "c":
		m.runtime.ClearCandidates()
		m.runtime.ClearProgress()
		m.candidates = nil
		m.otherCandidates = nil
		m.selected = 0
		m.progress = minidownload.Progress{}
		m.addLog("已清空候选资源（含来源未确认媒体）")
		m.setNotice("已清空候选资源，本地文件保留。", 4*time.Second)
	case "l":
		m.showLogs = !m.showLogs
		if m.showLogs {
			m.showDetails = false
		}
	case "i":
		m.showDetails = !m.showDetails
		if m.showDetails {
			m.showLogs = false
		}
	case "o":
		return m, m.openDirectory()
	case "x":
		if m.downloading {
			if m.downloadCancel != nil {
				m.downloadCancel()
			}
			m.setNotice("正在停止下载…", 8*time.Second)
		}
	case "d", "enter":
		visible := m.filteredCandidates()
		if !m.downloading && len(visible) > 0 {
			return m.startDownload(visible[m.selected])
		}
	}
	return m, nil
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func (m *model) clampLogOffset() {
	if m.logOffset < 0 {
		m.logOffset = 0
	}
	if m.logOffset > len(m.logs) {
		m.logOffset = len(m.logs)
	}
}

func (m *model) clampProgramIndex() {
	if len(m.programs) == 0 {
		m.programIdx = 0
		return
	}
	if m.programIdx >= len(m.programs) {
		m.programIdx = len(m.programs) - 1
	}
	if m.programIdx < 0 {
		m.programIdx = 0
	}
}

func (m model) retryOrRefresh() (tea.Model, tea.Cmd) {
	if m.startErr != nil && !m.downloading {
		return m.ensureProxy()
	}
	m.refresh()
	m.addLog("已刷新候选资源")
	return m, nil
}

func (m *model) cycleAppearance(step int) tea.Cmd {
	mode := m.appearance
	if step < 0 {
		mode = prevAppearance(mode)
	} else {
		mode = ui.NextAppearance(mode)
	}
	return m.setAppearance(mode)
}

func prevAppearance(mode ui.AppearanceMode) ui.AppearanceMode {
	switch mode {
	case ui.AppearanceSystem:
		return ui.AppearanceDark
	case ui.AppearanceLight:
		return ui.AppearanceSystem
	default:
		return ui.AppearanceLight
	}
}
