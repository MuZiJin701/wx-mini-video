package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/app"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
	"wx_channel/pkg/system"
)

// state.go 汇总目标选择、候选刷新、媒体动作与外观下拉，供键盘与鼠标路径共用。

// appearanceModes 是外观下拉的固定顺序。
func appearanceModes() []ui.AppearanceMode {
	return []ui.AppearanceMode{ui.AppearanceSystem, ui.AppearanceLight, ui.AppearanceDark}
}

// prepareDiscoveryTarget 让“启动即发现”不自动确认预填目标：输入框保留
// target.appID 的预填值，但运行时目标在用户确认前保持为空，发现阶段只登记
// 小程序与来源未确认媒体，不产生目标候选（ADR 0008）。
func prepareDiscoveryTarget(runtime *app.Runtime, prefill string) string {
	if strings.TrimSpace(runtime.Settings.Target.AppID) != "" {
		runtime.SetTarget(miniprogram.Target{})
	}
	return strings.TrimSpace(prefill)
}

// ---- 目标选择 ----

func (m model) confirmFocusedTarget() (tea.Model, tea.Cmd) {
	switch m.focus {
	case focusPrograms:
		return m.confirmSelectedProgram()
	case focusCancel:
		m.cancelTargetSwitch()
		return m, nil
	case focusAppearance:
		m.openAppearanceDropdown()
		return m, nil
	}
	if trimSpaceText(m.appIDInput) != "" {
		return m.confirmTargetInput()
	}
	if m.selectedProgram != "" {
		return m.confirmSelectedProgram()
	}
	m.inputErr = "请输入 AppID，或按 Tab 选择已发现的小程序"
	return m, nil
}

func trimSpaceText(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}

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
	m.inputCursor = len([]rune(appID))
	m.inputErr = ""
	m.selectedProgram = appID
	m.targetReady = true
	m.targetExpanded = false
	m.focus = focusNone
	m.appearanceOpen = false
	m.showDetails = false
	m.showLogs = false
	m.category = categoryAll
	m.selected = 0
	m.selectedID = ""
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

// openTargetEditor 展开统一目标选择区更换目标；取消不会改变当前目标。
func (m *model) openTargetEditor() {
	if m.downloading {
		m.setNotice("下载正在进行，完成后才能切换目标。", 5*time.Second)
		return
	}
	m.targetExpanded = true
	m.showDetails = false
	m.showLogs = false
	m.appearanceOpen = false
	m.inputErr = ""
	if m.selectedProgram == "" && len(m.programs) > 0 {
		m.selectedProgram = m.programs[0].AppID
	}
	if len(m.programs) > 0 {
		m.focus = focusPrograms
	} else {
		m.focus = focusInput
	}
	m.inputCursor = len([]rune(m.appIDInput))
}

// cancelTargetSwitch 收起目标选择区，保留当前目标、候选与已输入内容。
func (m *model) cancelTargetSwitch() {
	m.targetExpanded = false
	m.focus = focusNone
	m.appearanceOpen = false
	m.inputErr = ""
}

func (m *model) currentProgramIndex() int {
	if m.selectedProgram == "" {
		return -1
	}
	for i, program := range m.programs {
		if program.AppID == m.selectedProgram {
			return i
		}
	}
	return -1
}

func (m *model) selectProgram(step int) {
	if len(m.programs) == 0 {
		return
	}
	m.clampProgramSelection()
	index := m.currentProgramIndex()
	if index < 0 {
		if step >= 0 {
			index = 0
		} else {
			index = len(m.programs) - 1
		}
	} else {
		index += step
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.programs) {
		index = len(m.programs) - 1
	}
	m.selectedProgram = m.programs[index].AppID
}

func (m model) confirmSelectedProgram() (tea.Model, tea.Cmd) {
	if len(m.programs) == 0 {
		m.inputErr = "尚未发现小程序，请先在 PC 微信打开小程序，或直接输入 AppID"
		return m, nil
	}
	m.clampProgramSelection()
	index := m.currentProgramIndex()
	if index < 0 {
		index = 0
	}
	return m.confirmTarget(m.programs[index].AppID)
}

// ---- 焦点 ----

func (m model) focusables() []focusArea {
	areas := []focusArea{focusAppearance, focusInput}
	if len(m.programs) > 0 {
		areas = append(areas, focusPrograms)
	}
	areas = append(areas, focusConfirm)
	if m.targetReady && m.targetExpanded {
		areas = append(areas, focusCancel)
	}
	return areas
}

func (m *model) cycleFocus(step int) {
	areas := m.focusables()
	if len(areas) == 0 {
		m.focus = focusNone
		return
	}
	index := 0
	for i, area := range areas {
		if area == m.focus {
			index = i
			break
		}
	}
	index += step
	if index < 0 {
		index += len(areas)
	}
	index %= len(areas)
	m.focus = areas[index]
	if m.focus == focusInput {
		m.inputCursor = len([]rune(m.appIDInput))
	}
	if m.focus == focusPrograms && m.selectedProgram == "" && len(m.programs) > 0 {
		m.selectedProgram = m.programs[0].AppID
	}
}

// ---- 外观下拉 ----

func (m *model) openAppearanceDropdown() {
	if m.metricsNow().Narrow {
		return
	}
	m.appearanceOpen = true
	m.appearanceIdx = appearanceIndex(m.appearance)
}

func appearanceIndex(mode ui.AppearanceMode) int {
	for i, item := range appearanceModes() {
		if item == mode {
			return i
		}
	}
	return 0
}

func (m model) applyAppearanceSelection() (tea.Model, tea.Cmd) {
	modes := appearanceModes()
	if m.appearanceIdx < 0 || m.appearanceIdx >= len(modes) {
		m.appearanceOpen = false
		return m, nil
	}
	mode := modes[m.appearanceIdx]
	m.appearanceOpen = false
	m.focus = focusNone
	return m, m.setAppearance(mode)
}

// ---- 媒体动作 ----

func (m model) currentCandidate() (miniprogram.Candidate, bool) {
	visible := m.filteredCandidates()
	if m.selected < 0 || m.selected >= len(visible) {
		return miniprogram.Candidate{}, false
	}
	return visible[m.selected], true
}

func (m model) hasCandidate() bool {
	_, ok := m.currentCandidate()
	return ok
}

func (m *model) moveSelection(step int) {
	visible := m.filteredCandidates()
	if len(visible) == 0 {
		m.selected = 0
		m.selectedID = ""
		return
	}
	index := m.selected
	if index < 0 {
		if step >= 0 {
			index = 0
		} else {
			index = len(visible) - 1
		}
	} else {
		index += step
	}
	if index < 0 {
		index = 0
	}
	if index >= len(visible) {
		index = len(visible) - 1
	}
	m.selected = index
	m.selectedID = visible[index].ID
}

func (m model) downloadSelected() (tea.Model, tea.Cmd) {
	visible := m.filteredCandidates()
	if m.downloading {
		return m, nil
	}
	if len(visible) == 0 || m.selected < 0 || m.selected >= len(visible) {
		m.setNotice("当前分类没有可下载的候选资源。", 4*time.Second)
		return m, nil
	}
	return m.startDownload(visible[m.selected])
}

func (m model) startDownload(candidate miniprogram.Candidate) (tea.Model, tea.Cmd) {
	if m.downloading {
		return m, nil
	}
	m.downloading = true
	m.downloadID = candidate.ID
	m.selectedID = candidate.ID
	m.downloadLive = candidate.Live
	m.showDetails = false
	m.addLog("开始下载: " + attributionDisplay(candidate) + " " + candidateKindDisplay(candidate) + " " + candidateLabel(candidate))
	if candidate.Live {
		m.addLog("识别为直播流：会持续录制且不会自动结束，按 x 停止并保存")
		m.setNotice("直播流：下载会持续录制，按 x 停止并保存", 8*time.Second)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.downloadCancel = cancel
	return m, downloadCmd(ctx, m.runtime, candidate)
}

func (m *model) stopDownload() {
	if !m.downloading {
		m.setNotice("当前没有正在进行的下载或录制。", 4*time.Second)
		return
	}
	if m.downloadCancel != nil {
		m.downloadCancel()
	}
	m.setNotice("正在停止下载…", 8*time.Second)
}

func (m *model) toggleDetails() {
	if !m.hasCandidate() {
		m.setNotice("当前分类没有可查看详情的候选资源。", 4*time.Second)
		return
	}
	m.showDetails = !m.showDetails
	if m.showDetails {
		m.showLogs = false
	}
}

func (m *model) toggleLogs() {
	m.showLogs = !m.showLogs
	if m.showLogs {
		m.showDetails = false
		m.appearanceOpen = false
	}
}

func (m *model) clearCandidates() {
	m.runtime.ClearCandidates()
	m.runtime.ClearProgress()
	m.candidates = nil
	m.otherCandidates = nil
	m.selected = 0
	m.selectedID = ""
	m.progress = minidownload.Progress{}
	m.showDetails = false
	m.addLog("已清空候选资源（含来源未确认媒体）")
	m.setNotice("已清空候选资源，本地文件保留。", 4*time.Second)
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
	prevVisible := m.filteredCandidates()
	prevID := m.selectedID
	if prevID == "" && m.selected >= 0 && m.selected < len(prevVisible) {
		prevID = prevVisible[m.selected].ID
	}
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
	m.restoreSelection(prevID)
}

// restoreSelection 按候选 ID 维持选中身份；若候选被移出当前分类（例如直播状态
// 被修正），清空选中，避免用户下一步操作到另一条资源。
func (m *model) restoreSelection(prevID string) {
	visible := m.filteredCandidates()
	if prevID != "" {
		for index, candidate := range visible {
			if candidate.ID == prevID {
				m.selected = index
				m.selectedID = prevID
				return
			}
		}
		wasSelected := m.selectedID != ""
		m.selected = -1
		m.selectedID = ""
		if wasSelected {
			m.setNotice("选中候选已移出当前分类（类型可能被修正），请重新选择。", 6*time.Second)
		}
		return
	}
	if len(visible) == 0 {
		m.selected = 0
		m.selectedID = ""
		return
	}
	if m.selected < 0 {
		return
	}
	if m.selected >= len(visible) {
		m.selected = len(visible) - 1
	}
	m.selectedID = visible[m.selected].ID
}

func (m model) activeCategory() category {
	if m.category == "" {
		return categoryAll
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
		if candidateCategory(candidate) == active {
			out = append(out, candidate)
		}
	}
	return out
}

func (m *model) setCategory(next category) {
	m.category = next
	m.selected = 0
	m.selectedID = ""
	m.showDetails = false
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
		if candidateCategory(candidate) == item {
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
