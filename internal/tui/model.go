package tui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/app"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
)

type startMsg struct {
	err      error
	duration time.Duration
}

type tickMsg time.Time

type downloadMsg struct {
	path string
	err  error
}

type logMsg string

type ffmpegDoneMsg struct {
	path string
	err  error
}

type model struct {
	runtime      *app.Runtime
	candidates   []miniprogram.Candidate
	selected     int
	category     category
	logs         []string
	showLogs     bool
	showDetails  bool
	width        int
	height       int
	started      bool
	startErr     error
	downloading  bool
	downloadPath string
	progress     minidownload.Progress
	ffmpegSetup  *ffmpegProgressState
	ffmpegErr    error
	logOffset    int
	targetReady  bool
	appIDInput   string
	nameInput    string
	inputFocus   int
	inputErr     string
}

type category string

const (
	categoryAll   category = "all"
	categoryImage category = "image"
	categoryVideo category = "video"
	categoryM3U8  category = "m3u8"
)

var categories = []category{categoryAll, categoryImage, categoryVideo, categoryM3U8}

type ffmpegProgressState struct {
	mu    sync.Mutex
	state minidownload.FFmpegState
}

func (s *ffmpegProgressState) set(state minidownload.FFmpegState) {
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
}

func (s *ffmpegProgressState) get() minidownload.FFmpegState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func Run(runtime *app.Runtime) error {
	ffmpegSetup := &ffmpegProgressState{}
	m := model{
		runtime:     runtime,
		showLogs:    true,
		ffmpegSetup: ffmpegSetup,
		category:    categoryVideo,
		appIDInput:  runtime.Settings.Target.AppID,
		nameInput:   runtime.Settings.Target.Name,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m model) Init() tea.Cmd {
	if !m.targetReady {
		return nil
	}
	if m.runtime.HasFFmpeg() {
		return tea.Batch(m.startProxy, tick())
	}
	return tea.Batch(m.startProxy, setupFFmpegCmd(m.runtime, m.ffmpegSetup), tick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
	case tea.KeyMsg:
		if !m.targetReady {
			return m.updateTargetInput(v)
		}
		switch v.String() {
		case "q", "ctrl+c":
			return m, tea.Sequence(m.stopProxy, tea.Quit)
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
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.filteredCandidates())-1 {
				m.selected++
			}
		case "r":
			m.refresh()
			m.addLog("已刷新候选资源")
		case "c":
			m.runtime.ClearCandidates()
			m.runtime.ClearProgress()
			m.candidates = nil
			m.selected = 0
			m.progress = minidownload.Progress{}
			m.addLog("已清空候选资源")
		case "l":
			m.showLogs = !m.showLogs
		case "i":
			m.showDetails = !m.showDetails
		case "pgup":
			m.logOffset += 8
		case "pgdown":
			m.logOffset -= 8
			if m.logOffset < 0 {
				m.logOffset = 0
			}
		case "home":
			m.logOffset = len(m.logs)
		case "end":
			m.logOffset = 0
		case "o":
			if err := m.runtime.OpenDownloadDir(); err != nil {
				m.addLog("打开下载目录失败: " + err.Error())
			} else {
				m.addLog("已打开下载目录")
			}
		case "d", "enter":
			visible := m.filteredCandidates()
			if !m.downloading && len(visible) > 0 {
				candidate := visible[m.selected]
				if candidate.Kind == "m3u8" && candidate.CachedPath == "" && candidate.Source == "json" {
					m.addLog("该 m3u8 尚未缓存，不能直接下载。请按 c 清空候选后重新播放目标视频，等待 cache 标记出现。")
					return m, nil
				}
				m.downloading = true
				m.addLog("开始下载: " + candidateSummary(candidate))
				return m, downloadCmd(m.runtime, candidate)
			}
		}
	case startMsg:
		m.started = v.err == nil
		m.startErr = v.err
		if v.err != nil {
			m.addLog("代理启动失败: " + v.err.Error())
		} else {
			m.addLog(fmt.Sprintf("代理已启动: %s (耗时 %s)", m.runtime.ProxyAddr(), v.duration.Round(time.Millisecond)))
		}
	case tickMsg:
		m.refresh()
		m.progress = m.runtime.DownloadProgress()
		return m, tick()
	case downloadMsg:
		m.downloading = false
		if v.err != nil {
			m.addLog("下载失败: " + v.err.Error())
		} else {
			m.downloadPath = v.path
			m.addLog("下载完成: " + v.path)
		}
	case ffmpegDoneMsg:
		m.ffmpegErr = v.err
		if v.err != nil {
			m.addLog("ffmpeg 自动下载失败: " + v.err.Error())
		} else {
			m.addLog("ffmpeg 就绪: " + v.path)
		}
	case logMsg:
		m.addLog(string(v))
	}
	return m, nil
}

func (m model) View() string {
	if !m.targetReady {
		return m.renderTargetInput()
	}

	h := m.height
	if h <= 0 {
		h = 40
	}

	ffmpegSetupLine := m.renderFFmpegSetupProgress()
	headerLines := 5
	if ffmpegSetupLine != "" {
		headerLines++
	}
	progressLines := m.downloadProgressLines()
	logPanelLines := 0
	if m.showLogs {
		logPanelLines = min(7, max(len(m.logs), 1)+1)
	}
	detailLines := m.selectedCandidateDetails()
	listAvailable := h - headerLines - progressLines - logPanelLines - len(detailLines) - 1
	if listAvailable < 0 {
		listAvailable = 0
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("微信小程序视频下载"))
	b.WriteString("\n")

	ffmpegState := m.renderFFmpegState()
	infoLine := fmt.Sprintf("目标: %s  代理: %s  %s",
		targetDisplay(m.runtime.Settings.Target),
		m.runtime.ProxyAddr(),
		ffmpegState,
	)
	b.WriteString(fitLine(infoLine, m.width-1))
	b.WriteString("\n")

	if m.startErr != nil {
		b.WriteString(errorStyle.Render(fitLine("代理未启动: "+m.startErr.Error(), m.width-1)))
		b.WriteString("\n")
	} else if m.started {
		b.WriteString(okStyle.Render(fitLine("代理运行中。建议先按 c 清空，再打开目标图片或播放目标视频，以便识别最新候选。", m.width-1)))
		b.WriteString("\n")
	} else {
		b.WriteString(mutedStyle.Render(fitLine("代理启动中...", m.width-1)))
		b.WriteString("\n")
	}

	shortcuts := "快捷键: Tab 分类  1/2/3/4 全部/图片/视频/m3u8  ↑/↓ 选择  i 详情  d/Enter 下载  c 清空  l 日志  q 退出"
	b.WriteString(mutedStyle.Render(fitLine(shortcuts, m.width-1)))
	b.WriteString("\n")
	b.WriteString(m.renderCategoryTabs())
	b.WriteString("\n")
	if ffmpegSetupLine != "" {
		b.WriteString(okStyle.Render(shortURL(ffmpegSetupLine, m.width-1)))
		b.WriteString("\n")
	}

	b.WriteString(m.renderCandidates(listAvailable))
	if len(detailLines) > 0 {
		b.WriteString(strings.Join(detailLines, "\n"))
		b.WriteString("\n")
	}
	if progressLines > 0 {
		b.WriteString(m.renderDownloadProgress())
	}

	if m.showLogs {
		logMax := min(6, logPanelLines-1)
		if logMax > 0 {
			b.WriteString(m.renderLogs(logMax))
		}
	}

	return b.String()
}

func (m model) updateTargetInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "tab", "shift+tab", "up", "down":
		if m.inputFocus == 0 {
			m.inputFocus = 1
		} else {
			m.inputFocus = 0
		}
		m.inputErr = ""
		return m, nil
	case "backspace", "ctrl+h":
		if m.inputFocus == 0 && len(m.appIDInput) > 0 {
			m.appIDInput = trimLastRune(m.appIDInput)
		} else if m.inputFocus == 1 && len(m.nameInput) > 0 {
			m.nameInput = trimLastRune(m.nameInput)
		}
		m.inputErr = ""
		return m, nil
	case "enter":
		appID := strings.TrimSpace(m.appIDInput)
		if appID == "" {
			m.inputErr = "请先输入小程序 AppID"
			m.inputFocus = 0
			return m, nil
		}
		m.appIDInput = appID
		m.nameInput = strings.TrimSpace(m.nameInput)
		m.runtime.SetTarget(miniprogram.Target{AppID: m.appIDInput, Name: m.nameInput})
		m.targetReady = true
		m.inputErr = ""
		preflightLogs := m.runtime.PreflightChecks()
		m.logs = append(preflightLogs, "正在启动代理...", "正在检查 ffmpeg...")
		if m.runtime.HasFFmpeg() {
			m.ffmpegSetup.set(minidownload.FFmpegState{State: "found", Path: m.runtime.Downloader.FFmpegPath})
			m.logs = append(preflightLogs, "正在启动代理...", "ffmpeg 已可用")
			return m, tea.Batch(m.startProxy, tick())
		}
		return m, tea.Batch(m.startProxy, setupFFmpegCmd(m.runtime, m.ffmpegSetup), tick())
	default:
		if len(msg.Runes) == 0 {
			return m, nil
		}
		text := string(msg.Runes)
		if m.inputFocus == 0 {
			m.appIDInput += text
		} else {
			m.nameInput += text
		}
		m.inputErr = ""
		return m, nil
	}
}

func (m model) renderTargetInput() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("微信小程序视频下载"))
	b.WriteString("\n\n")
	b.WriteString("请输入本次要嗅探的小程序信息，AppID 必填，名称可选。\n")
	b.WriteString("APPID 作为本次会话标签；请确认后只播放目标小程序的视频。\n\n")
	b.WriteString(renderInputLine("AppID", m.appIDInput, m.inputFocus == 0, width))
	b.WriteString("\n")
	b.WriteString(renderInputLine("名称", m.nameInput, m.inputFocus == 1, width))
	b.WriteString("\n\n")
	if m.inputErr != "" {
		b.WriteString(errorStyle.Render(m.inputErr))
		b.WriteString("\n\n")
	}
	b.WriteString(mutedStyle.Render("快捷键: Tab 切换  Enter 确认并启动代理  Esc/Ctrl+C 退出"))
	b.WriteString("\n")
	return b.String()
}

func renderInputLine(label string, value string, focused bool, width int) string {
	prefix := "  "
	if focused {
		prefix = "> "
	}
	text := value
	if focused {
		text += "_"
	}
	line := fmt.Sprintf("%s%-6s %s", prefix, label+":", text)
	if focused {
		return selectedStyle.Render(shortURL(line, width-1))
	}
	return shortURL(line, width-1)
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func (m model) renderCandidates(maxLines int) string {
	visibleCandidates := m.filteredCandidates()
	if maxLines <= 0 {
		return ""
	}
	if len(m.candidates) == 0 {
		return mutedStyle.Render(fitLine("暂无候选资源。打开图片或播放视频后，图片、mp4 或 m3u8 会出现在这里。", m.width-1)) + "\n"
	}
	if len(visibleCandidates) == 0 {
		return mutedStyle.Render(fitLine(fmt.Sprintf("当前分类暂无资源。已嗅探 %d 个资源，可按 Tab 或 1/2/3/4 切换分类。", len(m.candidates)), m.width-1)) + "\n"
	}

	visible := maxLines
	half := visible / 2
	start := m.selected - half
	end := m.selected + visible - half
	if start < 0 {
		end -= start
		start = 0
	}
	if end > len(visibleCandidates) {
		start -= end - len(visibleCandidates)
		end = len(visibleCandidates)
		if start < 0 {
			start = 0
		}
	}

	var b strings.Builder

	if start > 0 {
		more := start
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↑ %d more...", more)))
		b.WriteString("\n")
	}

	lineW := m.width
	if lineW <= 0 {
		lineW = 80
	}

	for i := start; i < end; i++ {
		item := visibleCandidates[i]
		line := candidateListLine(item, lineW-1)
		if i == m.selected {
			line = selectedStyle.Render(fitLine("> "+strings.TrimSpace(line), lineW-1))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if end < len(visibleCandidates) {
		more := len(visibleCandidates) - end
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more...", more)))
		b.WriteString("\n")
	}

	return b.String()
}

func (m model) downloadProgressLines() int {
	if !m.shouldShowProgress() {
		return 0
	}
	return 2
}

func (m model) shouldShowProgress() bool {
	if m.downloading {
		return true
	}
	switch m.progress.Status {
	case "done", "error":
		return true
	default:
		return false
	}
}

func (m model) renderDownloadProgress() string {
	lineW := m.width
	if lineW <= 0 {
		lineW = 80
	}
	var b strings.Builder
	b.WriteString(okStyle.Render(fitLine(progressText(m.progress), lineW-1)))
	b.WriteString("\n")
	b.WriteString(okStyle.Render(fitLine(progressBar(m.progress, max(10, lineW-2)), lineW-1)))
	b.WriteString("\n")
	return b.String()
}

func (m model) renderLogs(maxLines int) string {
	var b strings.Builder
	b.WriteString(mutedStyle.Render(fitLine("日志: "+filepath.Join(m.runtime.Settings.DownloadDir, "wx-mini-video.log"), m.width-1)))
	b.WriteString("\n")
	start := 0
	if len(m.logs) > maxLines {
		start = len(m.logs) - maxLines
	}
	if m.logOffset > 0 {
		start = len(m.logs) - maxLines - m.logOffset
		if start < 0 {
			start = 0
		}
	}
	end := start + maxLines
	if end > len(m.logs) {
		end = len(m.logs)
	}
	for _, line := range m.logs[start:end] {
		rendered := fitLine(line, m.width-4)
		b.WriteString(mutedStyle.Render("• " + rendered))
		b.WriteString("\n")
	}
	return b.String()
}

func candidateListLine(candidate miniprogram.Candidate, width int) string {
	if width <= 0 {
		width = 80
	}
	cache := "-"
	if candidate.CachedPath != "" {
		cache = "cache"
	}
	source := candidate.Source
	if source == "" {
		source = "-"
	}
	prefix := fmt.Sprintf("  %s %-4s %-8s %-5s %-4s ",
		candidate.CreatedAt.Format("15:04:05"),
		kindDisplay(candidate.Kind),
		formatBytes(candidate.ContentLength),
		cache,
		sourceDisplay(source),
	)
	restWidth := width - lipgloss.Width(prefix)
	if restWidth <= 8 {
		return fitLine(prefix, width)
	}
	label := candidateLabel(candidate)
	info := candidateReadableInfo(candidate)
	labelWidth := min(26, max(10, restWidth/3))
	infoWidth := restWidth - labelWidth - 3
	if infoWidth < 8 {
		infoWidth = 8
	}
	return fitLine(prefix+fitLine(label, labelWidth)+" · "+fitLine(info, infoWidth), width)
}

func (m model) selectedCandidateDetails() []string {
	if !m.showDetails {
		return nil
	}
	visible := m.filteredCandidates()
	if m.selected < 0 || m.selected >= len(visible) {
		return nil
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	return renderCandidateDetails(visible[m.selected], width-1)
}

func renderCandidateDetails(candidate miniprogram.Candidate, width int) []string {
	lines := []string{mutedStyle.Render("选中资源详情")}
	lines = appendDetailLines(lines, "标题", candidate.Title, width)
	lines = appendDetailLines(lines, "类型/来源", kindDisplay(candidate.Kind)+" / "+sourceDisplay(candidate.Source), width)
	lines = appendDetailLines(lines, "URL", candidate.URL, width)
	lines = appendDetailLines(lines, "来源 URL", candidate.SourceURL, width)
	lines = appendDetailLines(lines, "请求头", headerSummary(candidate.Headers), width)
	cachePath := candidate.CachedPath
	if cachePath == "" {
		cachePath = "未缓存"
	}
	return appendDetailLines(lines, "本地缓存", cachePath, width)
}

func appendDetailLines(lines []string, label, value string, width int) []string {
	if strings.TrimSpace(value) == "" {
		value = "-"
	}
	return append(lines, detailLines(label, value, width)...)
}

func detailLines(label, value string, width int) []string {
	prefix := label + ": "
	if width <= lipgloss.Width(prefix) {
		width = lipgloss.Width(prefix) + 1
	}
	chunks := splitDisplayText(value, max(1, width-lipgloss.Width(prefix)))
	lines := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		if i == 0 {
			lines = append(lines, prefix+chunk)
			continue
		}
		lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+chunk)
	}
	return lines
}

func splitDisplayText(value string, width int) []string {
	if value == "" {
		return []string{""}
	}
	var chunks []string
	var current strings.Builder
	for _, r := range value {
		candidate := current.String() + string(r)
		if current.Len() > 0 && lipgloss.Width(candidate) > width {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func headerSummary(headers map[string]string) string {
	if len(headers) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(headers[key])
		switch strings.ToLower(key) {
		case "authorization", "cookie", "proxy-authorization":
			value = "已捕获"
		}
		if value == "" {
			value = "-"
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "; ")
}

func (m *model) refresh() {
	prevLen := len(m.filteredCandidates())
	prevSelected := m.selected
	m.candidates = m.runtime.Candidates()
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

func (m model) filteredCandidates() []miniprogram.Candidate {
	active := m.activeCategory()
	if active == categoryAll {
		return m.candidates
	}
	out := make([]miniprogram.Candidate, 0, len(m.candidates))
	for _, candidate := range m.candidates {
		if active == categoryImage && candidate.Kind == "image" {
			out = append(out, candidate)
		}
		if active == categoryVideo && candidate.Kind == "video" {
			out = append(out, candidate)
		}
		if active == categoryM3U8 && candidate.Kind == "m3u8" {
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

func (m model) renderCategoryTabs() string {
	var parts []string
	for i, item := range categories {
		label := fmt.Sprintf("%d %s", i+1, categoryDisplay(item))
		count := m.categoryCount(item)
		if item != categoryAll {
			label = fmt.Sprintf("%s(%d)", label, count)
		} else {
			label = fmt.Sprintf("%s(%d)", label, len(m.candidates))
		}
		if item == m.activeCategory() {
			parts = append(parts, selectedStyle.Render(" "+label+" "))
			continue
		}
		parts = append(parts, mutedStyle.Render(" "+label+" "))
	}
	return strings.Join(parts, " ")
}

func (m model) categoryCount(item category) int {
	count := 0
	for _, candidate := range m.candidates {
		switch item {
		case categoryImage:
			if candidate.Kind == "image" {
				count++
			}
		case categoryVideo:
			if candidate.Kind == "video" {
				count++
			}
		case categoryM3U8:
			if candidate.Kind == "m3u8" {
				count++
			}
		}
	}
	return count
}

func (m *model) addLog(line string) {
	entry := time.Now().Format("15:04:05") + " " + line
	m.logs = append(m.logs, entry)
	m.appendLogFile(entry)
}

func (m model) appendLogFile(line string) {
	dir := m.runtime.Settings.DownloadDir
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "wx-mini-video.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line + "\n")
}

func (m model) startProxy() tea.Msg {
	startedAt := time.Now()
	return startMsg{err: m.runtime.Start(), duration: time.Since(startedAt)}
}

func (m model) stopProxy() tea.Msg {
	if err := m.runtime.Stop(); err != nil {
		return logMsg("关闭代理失败: " + err.Error())
	}
	return logMsg("代理已关闭")
}

func setupFFmpegCmd(runtime *app.Runtime, state *ffmpegProgressState) tea.Cmd {
	return func() tea.Msg {
		err := runtime.EnsureFFmpeg(context.Background(), func(progress minidownload.FFmpegState) {
			state.set(progress)
		})
		progress := state.get()
		if err != nil {
			return ffmpegDoneMsg{err: err}
		}
		return ffmpegDoneMsg{path: progress.Path}
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func downloadCmd(runtime *app.Runtime, candidate miniprogram.Candidate) tea.Cmd {
	return func() tea.Msg {
		path, err := runtime.Download(context.Background(), candidate, "")
		return downloadMsg{path: path, err: err}
	}
}

func shortURL(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func candidateSummary(candidate miniprogram.Candidate) string {
	cache := "no-cache"
	if candidate.CachedPath != "" {
		cache = "cache"
	}
	return fmt.Sprintf("%s %s %s %s", kindDisplay(candidate.Kind), cache, formatBytes(candidate.ContentLength), candidateReadableInfo(candidate))
}

func fitLine(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	var b strings.Builder
	for _, r := range value {
		next := b.String() + string(r)
		if lipgloss.Width(next)+3 > width {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "..."
}

func categoryDisplay(value category) string {
	switch value {
	case categoryAll:
		return "全部"
	case categoryImage:
		return "图片"
	case categoryVideo:
		return "视频"
	case categoryM3U8:
		return "m3u8"
	default:
		return string(value)
	}
}

func kindDisplay(kind string) string {
	switch kind {
	case "image":
		return "图片"
	case "video":
		return "视频"
	case "m3u8":
		return "m3u8"
	default:
		return kind
	}
}

func sourceDisplay(source string) string {
	switch source {
	case "response":
		return "响应"
	case "json":
		return "JSON"
	case "-":
		return "-"
	default:
		return source
	}
}

func targetDisplay(target miniprogram.Target) string {
	if strings.TrimSpace(target.Name) == "" {
		return strings.TrimSpace(target.AppID)
	}
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(target.Name), strings.TrimSpace(target.AppID))
}

var recordTimeRegexp = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z-(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z`)

func candidateLabel(candidate miniprogram.Candidate) string {
	if title := strings.TrimSpace(candidate.Title); title != "" {
		return title
	}
	u, err := url.Parse(candidate.URL)
	if err != nil {
		return candidate.Kind
	}
	base := path.Base(u.Path)
	if candidate.Kind == "m3u8" {
		if match := recordTimeRegexp.FindStringSubmatch(base); len(match) == 5 {
			if match[1] == match[3] {
				return "回放 " + match[1] + " " + match[2] + "-" + match[4]
			}
			return "回放 " + match[1] + " " + match[2] + " 至 " + match[3] + " " + match[4]
		}
		stream := streamName(u.Path)
		if stream != "" {
			return "直播流 " + stream
		}
		return "m3u8 " + base
	}
	if base != "" && base != "." && base != "/" {
		return strings.TrimSuffix(base, path.Ext(base))
	}
	if u.Host != "" {
		return u.Host
	}
	return candidate.Kind
}

func candidateReadableInfo(candidate miniprogram.Candidate) string {
	u, err := url.Parse(candidate.URL)
	if err != nil {
		return candidate.URL
	}
	var parts []string
	if field := fieldDisplay(candidate.FieldPath); field != "" {
		parts = append(parts, field)
	}
	if u.Host != "" {
		parts = append(parts, u.Host)
	}
	base := path.Base(u.Path)
	if base != "" && base != "." && base != "/" {
		parts = append(parts, base)
	}
	if len(parts) == 0 {
		return candidate.URL
	}
	return strings.Join(parts, " · ")
}

func fieldDisplay(fieldPath string) string {
	key := strings.ToLower(fieldPath)
	switch {
	case strings.Contains(key, "cover"):
		return "封面"
	case strings.Contains(key, "poster"):
		return "海报"
	case strings.Contains(key, "avatar"):
		return "头像"
	case strings.Contains(key, "thumb"):
		return "缩略图"
	case strings.Contains(key, "banner"):
		return "横幅"
	case strings.Contains(key, "logo"):
		return "标志"
	case strings.Contains(key, "video"):
		return "视频"
	case strings.Contains(key, "image"), strings.Contains(key, "img"), strings.Contains(key, "pic"), strings.Contains(key, "photo"):
		return "图片"
	default:
		return ""
	}
}

func streamName(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		if part == "streams" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "-"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value = value / unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fPB", value/unit)
}

func progressText(progress minidownload.Progress) string {
	switch progress.Status {
	case "prepare-ffmpeg":
		return "正在准备 ffmpeg，完成后会继续下载..."
	case "starting":
		return "准备下载..."
	case "cache-m3u8":
		return "正在缓存 m3u8..."
	case "segments":
		if progress.Items > 0 {
			pct := int(float64(progress.Completed) / float64(progress.Items) * 100)
			extra := ""
			if progress.Reused > 0 {
				extra += fmt.Sprintf("  复用%d", progress.Reused)
			}
			if progress.Failed > 0 {
				extra += fmt.Sprintf("  失败%d，建议降低并发", progress.Failed)
			}
			return fmt.Sprintf("分片下载中 %d%%  %d/%d  %s%s", pct, progress.Completed, progress.Items, formatBytes(progress.Downloaded), extra)
		}
		return "分片下载中 " + formatBytes(progress.Downloaded)
	case "ffmpeg":
		prefix := "ffmpeg 合并中"
		if !progress.LastUpdate.IsZero() && time.Since(progress.LastUpdate) > 10*time.Second {
			prefix = "ffmpeg 仍在运行，最近 10 秒无进度更新"
		}
		if progress.Total > 0 && progress.Downloaded > 0 {
			pct := int(float64(progress.Downloaded) / float64(progress.Total) * 100)
			if pct > 100 {
				pct = 100
			}
			return fmt.Sprintf("%s %d%%  %s/%s", prefix, pct, formatDuration(progress.Downloaded), formatDuration(progress.Total))
		}
		if progress.Downloaded > 0 {
			return fmt.Sprintf("%s  已处理 %s", prefix, formatDuration(progress.Downloaded))
		}
		return prefix + "..."
	case "downloading":
		if progress.Total > 0 {
			pct := int(float64(progress.Downloaded) / float64(progress.Total) * 100)
			return fmt.Sprintf("下载中 %d%%  %s/%s", pct, formatBytes(progress.Downloaded), formatBytes(progress.Total))
		}
		return "下载中 " + formatBytes(progress.Downloaded)
	case "done":
		return "下载完成: " + progress.Path
	case "error":
		return "下载失败: " + progress.Error
	default:
		return "下载中，请稍候..."
	}
}

func (m model) renderFFmpegState() string {
	state := m.ffmpegSetup.get()
	switch state.State {
	case "checking":
		return "ffmpeg: 检查中"
	case "found":
		return "ffmpeg: 可用"
	case "downloading":
		if state.Total > 0 {
			pct := int(float64(state.Current) / float64(state.Total) * 100)
			return fmt.Sprintf("ffmpeg: 下载中 %d%%", pct)
		}
		return "ffmpeg: 下载中"
	case "extracting":
		return "ffmpeg: 解压中"
	case "done":
		return "ffmpeg: 可用"
	case "error":
		return "ffmpeg: 自动下载失败"
	default:
		if m.runtime.HasFFmpeg() {
			return "ffmpeg: 可用"
		}
		if m.ffmpegErr != nil {
			return "ffmpeg: 自动下载失败"
		}
		return "ffmpeg: 未检测到"
	}
}

func (m model) renderFFmpegSetupProgress() string {
	state := m.ffmpegSetup.get()
	switch state.State {
	case "downloading":
		if state.Total > 0 {
			pct := int(float64(state.Current) / float64(state.Total) * 100)
			return fmt.Sprintf("下载 ffmpeg %s %d%%  %s/%s", simpleBar(state.Current, state.Total, 24), pct, formatBytes(state.Current), formatBytes(state.Total))
		}
		return "下载 ffmpeg " + simpleSpinner(24)
	case "extracting":
		return "正在解压 ffmpeg..."
	default:
		return ""
	}
}

func progressBar(progress minidownload.Progress, width int) string {
	if width <= 0 {
		width = 20
	}
	if width > 60 {
		width = 60
	}
	if progress.Status == "done" {
		return simpleBar(1, 1, width)
	}
	if progress.Status == "error" {
		return simpleBar(0, 1, width)
	}
	if progress.Total <= 0 {
		return simpleSpinner(width)
	}
	if progress.Status == "segments" && progress.Items > 0 {
		return simpleBar(int64(progress.Completed), int64(progress.Items), width)
	}
	return simpleBar(progress.Downloaded, progress.Total, width)
}

func simpleBar(current, total int64, width int) string {
	if width <= 0 {
		width = 20
	}
	if width > 60 {
		width = 60
	}
	filled := 0
	if total > 0 {
		filled = int(float64(current) / float64(total) * float64(width))
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}

func simpleSpinner(width int) string {
	if width <= 0 {
		width = 20
	}
	if width > 60 {
		width = 60
	}
	phase := time.Now().Second() % width
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < width; i++ {
		if i == phase {
			b.WriteString("=")
		} else {
			b.WriteString(" ")
		}
	}
	b.WriteString("]")
	return b.String()
}

func formatDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	totalSeconds := ms / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
