package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/tui/ui"
)

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.stopErr != nil {
		switch key {
		case "r", "enter":
			return m.beginStop()
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
	if m.appearanceOpen {
		switch key {
		case "up", "k":
			if m.appearanceIdx > 0 {
				m.appearanceIdx--
			}
		case "down", "j":
			if m.appearanceIdx < len(appearanceModes())-1 {
				m.appearanceIdx++
			}
		case "enter":
			return m.applyAppearanceSelection()
		case "esc":
			m.appearanceOpen = false
		case "q", "ctrl+q", "ctrl+c":
			return m.requestQuit()
		}
		return m, nil
	}

	switch key {
	case "ctrl+q", "ctrl+c":
		return m.requestQuit()
	case "ctrl+o":
		return m, m.openDirectory()
	case "ctrl+r":
		return m.retryOrRefresh()
	}

	if m.targetExpanded {
		return m.handleTargetAreaKey(msg)
	}
	if m.showLogs {
		return m.handleLogKey(msg)
	}
	return m.handleMediaKey(msg)
}

// handleTargetAreaKey 处理统一目标选择区的键盘：Tab 切换焦点，输入框按文本编辑，
// 列表用上下方向键导航，Enter 执行当前焦点对应的确认动作。
func (m model) handleTargetAreaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "tab":
		m.cycleFocus(1)
		return m, nil
	case "shift+tab":
		m.cycleFocus(-1)
		return m, nil
	case "esc":
		if m.targetReady {
			m.cancelTargetSwitch()
			return m, nil
		}
		return m.requestQuit()
	case "up", "down":
		if m.focus == focusPrograms {
			step := 1
			if key == "up" {
				step = -1
			}
			m.selectProgram(step)
			return m, nil
		}
	case "left", "right":
		if m.focus == focusAppearance {
			step := 1
			if key == "left" {
				step = -1
			}
			return m, m.cycleAppearance(step)
		}
	case "enter":
		switch m.focus {
		case focusPrograms:
			return m.confirmSelectedProgram()
		case focusCancel:
			m.cancelTargetSwitch()
			return m, nil
		case focusAppearance:
			m.openAppearanceDropdown()
			return m, nil
		default:
			return m.confirmFocusedTarget()
		}
	}
	if m.focus == focusInput {
		return m.handleInputKey(msg)
	}
	// 目标选择区中未聚焦输入框时，仍允许 q 退出、r 重试启动。
	switch key {
	case "q":
		return m.requestQuit()
	case "r":
		return m.retryOrRefresh()
	}
	return m, nil
}

// handleInputKey 把可打印字符与编辑键交给 AppID 输入框，业务单键不得吞掉输入。
func (m model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	runes := []rune(m.appIDInput)
	cursor := m.inputCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	switch msg.String() {
	case "backspace", "ctrl+h":
		if cursor > 0 {
			runes = append(runes[:cursor-1], runes[cursor:]...)
			cursor--
		}
		m.appIDInput = string(runes)
		m.inputCursor = cursor
		m.inputErr = ""
		m.programPicked = false
		return m, nil
	case "delete":
		if cursor < len(runes) {
			runes = append(runes[:cursor], runes[cursor+1:]...)
		}
		m.appIDInput = string(runes)
		m.inputCursor = cursor
		m.inputErr = ""
		m.programPicked = false
		return m, nil
	case "left":
		if cursor > 0 {
			cursor--
		}
		m.inputCursor = cursor
		return m, nil
	case "right":
		if cursor < len(runes) {
			cursor++
		}
		m.inputCursor = cursor
		return m, nil
	case "home", "ctrl+a":
		m.inputCursor = 0
		return m, nil
	case "end", "ctrl+e":
		m.inputCursor = len(runes)
		return m, nil
	case "ctrl+u":
		m.appIDInput = ""
		m.inputCursor = 0
		m.inputErr = ""
		m.programPicked = false
		return m, nil
	}
	if len(msg.Runes) == 0 {
		return m, nil
	}
	insert := []rune(string(msg.Runes))
	updated := make([]rune, 0, len(runes)+len(insert))
	updated = append(updated, runes[:cursor]...)
	updated = append(updated, insert...)
	updated = append(updated, runes[cursor:]...)
	m.appIDInput = string(updated)
	m.inputCursor = cursor + len(insert)
	m.inputErr = ""
	m.programPicked = false
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
	case "q":
		return m.requestQuit()
	case "esc":
		if m.showDetails {
			m.showDetails = false
		} else if m.showLogs {
			m.showLogs = false
		}
		return m, nil
	case "s":
		if m.downloading {
			m.setNotice("下载正在进行，完成后才能切换目标。", 5*time.Second)
			return m, nil
		}
		m.openTargetEditor()
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
		m.setCategory(categoryAudio)
	case "5":
		m.setCategory(categoryLive)
	case "6":
		m.setCategory(categorySegments)
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "r":
		return m.retryOrRefresh()
	case "c":
		m.clearCandidates()
	case "l":
		m.toggleLogs()
	case "i":
		m.toggleDetails()
	case "o":
		return m, m.openDirectory()
	case "x":
		m.stopDownload()
	case "d", "enter":
		return m.downloadSelected()
	}
	return m, nil
}

func (m *model) clampLogOffset() {
	if m.logOffset < 0 {
		m.logOffset = 0
	}
	if m.logOffset > len(m.logs) {
		m.logOffset = len(m.logs)
	}
}

func (m *model) clampProgramSelection() {
	if m.selectedProgram == "" {
		if m.focus == focusPrograms && len(m.programs) > 0 {
			m.selectedProgram = m.programs[0].AppID
		}
		return
	}
	for _, program := range m.programs {
		if program.AppID == m.selectedProgram {
			return
		}
	}
	m.selectedProgram = ""
}

func (m model) retryOrRefresh() (tea.Model, tea.Cmd) {
	if m.startErr != nil && !m.downloading {
		return m.ensureProxy()
	}
	m.refresh()
	m.addLog("已刷新候选资源")
	m.setNotice("已刷新候选资源与发现列表。", 4*time.Second)
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
