package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/pkg/system"
)

// state.go 汇总目标选择、候选刷新与下载动作，供键盘与鼠标路径共用。

func (m model) confirmTargetInput() (tea.Model, tea.Cmd) {
	raw := m.appIDInput
	appID, err := miniprogram.NormalizeTargetAppID(raw)
	if err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	return m.confirmTarget(appID)
}

func (m model) confirmTarget(appID string) (tea.Model, tea.Cmd) {
	if appID == "" {
		m.inputErr = "请先输入小程序 AppID"
		return m, nil
	}
	m.appIDInput = appID
	m.inputErr = ""
	m.showTargets = false
	m.targetReady = true
	m.category = categoryVideo
	m.selected = 0
	m.candidates = nil
	m.otherCandidates = nil
	m.progress = minidownload.Progress{}
	m.downloaded = make(map[string]string)
	m.lastSavedPath = ""
	m.runtime.SetTarget(miniprogram.Target{AppID: appID})
	m.addLog("已选定目标 " + appID + "，请重新打开图片或播放视频或音频；来源未确认的媒体会以 ? 标记并按类型显示")
	m.setNotice("目标已选定。在 PC 微信重新加载目标小程序的图片或音视频；来源未确认的媒体会以 ? 标记按类型显示。", 8*time.Second)
	return m.ensureProxy()
}

func (m *model) openTargetOverlay() {
	m.programs = m.runtime.Store.Programs()
	m.programIdx = 0
	current := m.runtime.Settings.Target.AppID
	for i, program := range m.programs {
		if program.AppID == current {
			m.programIdx = i
			break
		}
	}
	m.showTargets = true
	m.showDetails = false
	m.showLogs = false
}

func (m model) confirmSelectedProgram() (tea.Model, tea.Cmd) {
	if len(m.programs) == 0 {
		return m, nil
	}
	m.clampProgramIndex()
	program := m.programs[m.programIdx]
	if program.AppID == m.runtime.Settings.Target.AppID && m.targetReady {
		m.showTargets = false
		return m, nil
	}
	return m.confirmTarget(program.AppID)
}

func (m model) startDownload(candidate miniprogram.Candidate) (tea.Model, tea.Cmd) {
	if m.downloading {
		return m, nil
	}
	m.downloading = true
	m.downloadID = candidate.ID
	m.downloadLive = candidate.Live
	m.showTargets = false
	m.addLog("开始下载: " + attributionDisplay(candidate) + " " + candidateKindDisplay(candidate) + " " + candidateLabel(candidate))
	if candidate.Live {
		m.addLog("识别为直播流：会持续录制且不会自动结束，按 x 停止并保存")
		m.setNotice("直播流：下载会持续录制，按 x 停止并保存", 8*time.Second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.downloadCancel = cancel
	return m, downloadCmd(ctx, m.runtime, candidate)
}

func (m model) openDirectory() tea.Cmd {
	target := ""
	if m.targetReady {
		target = m.runtime.Settings.Target.AppID
	}
	return func() tea.Msg {
		if err := m.runtime.OpenDownloadDir(target); err != nil {
			return logMsg("打开下载目录失败: " + err.Error())
		}
		return logMsg("已打开下载目录")
	}
}

func (m model) openRepository() tea.Cmd {
	return func() tea.Msg {
		if err := openExternalURL("https://github.com/MuZiJin701/wx-mini-video"); err != nil {
			return logMsg("打开浏览器失败: " + err.Error())
		}
		return logMsg("已在浏览器打开项目仓库")
	}
}

func (m *model) refresh() {
	prevLen := len(m.filteredCandidates())
	prevSelected := m.selected
	prevOther := len(m.otherCandidates)
	m.candidates = m.runtime.Candidates()
	if m.runtime.Store != nil && m.showUnattributedEnabled() {
		m.otherCandidates = m.runtime.Store.Unattributed()
	} else {
		m.otherCandidates = nil
	}
	if m.showUnattributedEnabled() && len(m.otherCandidates) > 0 && prevOther == 0 {
		m.addLog("检测到来源未确认媒体，已按类型并入列表（以 ? 标记）")
	}
	visible := m.filteredCandidates()
	if len(visible) > prevLen && (prevLen == 0 || prevSelected == prevLen-1) {
		m.selected = len(visible) - 1
		return
	}
	if m.selected >= len(visible) {
		m.selected = len(visible) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m model) activeCategory() category {
	if m.category == "" {
		return categoryVideo
	}
	return m.category
}

// displayCandidates 合并目标候选与来源未确认候选，按 URL 去重，目标候选优先。
func (m model) displayCandidates() []miniprogram.Candidate {
	if len(m.otherCandidates) == 0 {
		return m.candidates
	}
	out := make([]miniprogram.Candidate, 0, len(m.candidates)+len(m.otherCandidates))
	seen := make(map[string]struct{}, len(m.candidates))
	for _, candidate := range m.candidates {
		out = append(out, candidate)
		seen[candidate.URL] = struct{}{}
	}
	for _, candidate := range m.otherCandidates {
		if _, exists := seen[candidate.URL]; exists {
			continue
		}
		seen[candidate.URL] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func (m model) filteredCandidates() []miniprogram.Candidate {
	active := m.activeCategory()
	pool := m.displayCandidates()
	if active == categoryAll {
		return pool
	}
	out := make([]miniprogram.Candidate, 0, len(pool))
	for _, candidate := range pool {
		if candidate.Kind == string(active) {
			out = append(out, candidate)
		}
	}
	return out
}

func (m *model) setCategory(next category) {
	m.category = next
	m.selected = 0
}

func (m *model) nextCategory(step int) {
	active := m.activeCategory()
	idx := 0
	for i, item := range categories {
		if item == active {
			idx = i
			break
		}
	}
	idx = (idx + step) % len(categories)
	if idx < 0 {
		idx += len(categories)
	}
	m.setCategory(categories[idx])
}

func (m model) categoryCount(item category) int {
	pool := m.displayCandidates()
	if item == categoryAll {
		return len(pool)
	}
	count := 0
	for _, candidate := range pool {
		if candidate.Kind == string(item) {
			count++
		}
	}
	return count
}

func (m model) downloadedPath(candidate miniprogram.Candidate) string {
	if m.downloaded == nil || candidate.ID == "" {
		return ""
	}
	return m.downloaded[candidate.ID]
}

func openExternalURL(rawURL string) error {
	return system.Open(rawURL)
}
