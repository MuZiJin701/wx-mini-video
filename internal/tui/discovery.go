package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) ensureProxy() (tea.Model, tea.Cmd) {
	if m.started || m.starting {
		return m, nil
	}
	m.starting = true
	m.startErr = nil
	m.logs = append(m.logs, m.runtime.PreflightChecks()...)
	m.addLog("正在启动代理...")
	return m, tea.Batch(m.startProxy, tick())
}

func (m model) updateDiscovery(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "esc":
		return m.requestQuit()
	case "ctrl+d":
		m.discovering = false
		m.inputErr = ""
		return m.ensureProxy()
	case "up", "k":
		if m.programIndex > 0 {
			m.programIndex--
		}
	case "down", "j":
		if m.programIndex < len(m.programs)-1 {
			m.programIndex++
		}
	case "enter":
		if len(m.programs) > 0 {
			target := m.programs[m.programIndex]
			m.appIDInput = target.AppID
			m.nameInput = target.Name
			m.inputFocus = 1
			m.inputErr = ""
			m.discovering = false
		}
	case "r":
		m.programs = m.runtime.Store.Programs()
		return m.ensureProxy()
	}
	return m, nil
}

func (m model) renderDiscovery() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("小程序发现"))
	b.WriteString("\n\n请在 PC 微信中打开目标小程序，再从下面选择 AppID。\n")
	b.WriteString("选定目标前不收集媒体；名称可在确认界面补填。\n")
	if m.startErr != nil {
		b.WriteString(errorStyle.Render(fitLine("代理启动失败: "+m.startErr.Error()+"；按 r 重试", width-1)))
	} else if m.started {
		b.WriteString(okStyle.Render("代理运行中，等待小程序请求。"))
	} else {
		b.WriteString(mutedStyle.Render("代理启动中..."))
	}
	b.WriteString("\n\n")
	if len(m.programs) == 0 {
		b.WriteString("未发现小程序。请重新打开小程序，或按 Ctrl+D 手动输入 AppID。\n")
	} else {
		visible := max(1, height-10)
		start := max(0, m.programIndex-visible+1)
		end := min(len(m.programs), start+visible)
		for i := start; i < end; i++ {
			target := m.programs[i]
			name := target.Name
			if name == "" {
				name = "名称未识别"
			}
			line := fitLine(fmt.Sprintf("  %s (%s)", name, target.AppID), width-1)
			if i == m.programIndex {
				line = selectedStyle.Render(fitLine("> "+strings.TrimSpace(line), width-1))
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n快捷键: ↑/↓ 选择  Enter 补填名称并确认  Ctrl+D 手动输入  r 刷新/重试  q 退出\n")
	return b.String()
}
