package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"wx_channel/pkg/certificate"
)

var legacyRootPresent = func() (bool, error) { return certificate.CheckHasCertificate(certificate.LegacyRootThumbprint) }
var removeLegacyRoot = certificate.RemoveLegacyRoot

type stopMsg struct{ err error }
type legacyCleanupMsg struct{ err error }

func removeLegacyRootCmd() tea.Msg { return legacyCleanupMsg{err: removeLegacyRoot()} }

// beginStop 进入退出流程：恢复系统代理并关闭嗅探。
func (m model) beginStop() (tea.Model, tea.Cmd) {
	m.stopErr = nil
	m.stopping = true
	m.confirmQuit = false
	if m.starting {
		return m, nil
	}
	return m, m.stopProxy
}

// requestQuit 在下载中先请求二次确认，其余情况直接退出。
func (m model) requestQuit() (tea.Model, tea.Cmd) {
	if m.downloading {
		m.confirmQuit = true
		return m, nil
	}
	return m.beginStop()
}

// confirmQuitNow 执行退出确认：下载/录制中先停止并保存现场，等 downloadMsg 收尾后再退出。
func (m model) confirmQuitNow() (tea.Model, tea.Cmd) {
	if m.downloading {
		m.quitAfterDownload = true
		if m.downloadCancel != nil {
			m.downloadCancel()
		}
		m.setNotice("正在停止下载，完成后退出…", 10*time.Second)
		return m, nil
	}
	return m.beginStop()
}
