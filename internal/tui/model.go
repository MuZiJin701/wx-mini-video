package tui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	logs         []string
	showLogs     bool
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
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.candidates)-1 {
				m.selected++
			}
		case "r":
			m.refresh()
			m.addLog("已刷新候选资源")
		case "c":
			m.runtime.ClearCandidates()
			m.candidates = nil
			m.selected = 0
			m.addLog("已清空候选资源")
		case "l":
			m.showLogs = !m.showLogs
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
			if !m.downloading && len(m.candidates) > 0 {
				candidate := m.candidates[m.selected]
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
	headerLines := 4
	if ffmpegSetupLine != "" {
		headerLines++
	}
	logPanelLines := 0
	if m.showLogs {
		logPanelLines = min(9, max(len(m.logs), 1)+1)
	}
	listAvailable := h - headerLines - logPanelLines - 1
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
	b.WriteString(shortURL(infoLine, m.width-1))
	b.WriteString("\n")

	if m.startErr != nil {
		b.WriteString(errorStyle.Render(shortURL("代理未启动: "+m.startErr.Error(), m.width-1)))
		b.WriteString("\n")
	} else if m.started {
		b.WriteString(okStyle.Render(shortURL("代理运行中。建议先按 c 清空，再打开目标图片或播放目标视频，以便识别最新候选。", m.width-1)))
		b.WriteString("\n")
	} else {
		b.WriteString(mutedStyle.Render(shortURL("代理启动中...", m.width-1)))
		b.WriteString("\n")
	}

	shortcuts := "快捷键: ↑/↓ 选择  d/Enter 下载  r 刷新  c 清空  o 打开目录  l 日志  PgUp/PgDn 翻日志  q 退出"
	b.WriteString(mutedStyle.Render(shortURL(shortcuts, m.width-1)))
	b.WriteString("\n")
	if ffmpegSetupLine != "" {
		b.WriteString(okStyle.Render(shortURL(ffmpegSetupLine, m.width-1)))
		b.WriteString("\n")
	}

	b.WriteString(m.renderCandidates(listAvailable))

	if m.showLogs {
		logMax := min(8, logPanelLines-1)
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
	if len(m.candidates) == 0 {
		return mutedStyle.Render("暂无候选资源。打开图片或播放视频后，图片、mp4 或 m3u8 会出现在这里。") + "\n"
	}

	if maxLines <= 0 {
		return ""
	}

	downloadExtra := 0
	if m.downloading {
		downloadExtra = 2
	}

	visible := maxLines - downloadExtra
	if visible <= 0 {
		visible = 0
	}

	half := visible / 2
	start := m.selected - half
	end := m.selected + visible - half
	if start < 0 {
		end -= start
		start = 0
	}
	if end > len(m.candidates) {
		start -= end - len(m.candidates)
		end = len(m.candidates)
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
	labelW := 28
	urlW := max(20, lineW-76)

	for i := start; i < end; i++ {
		item := m.candidates[i]
		cache := "-"
		if item.CachedPath != "" {
			cache = "cache"
		}
		source := item.Source
		if source == "" {
			source = "-"
		}
		line := fmt.Sprintf("  %s %-5s %-9s %-5s %-8s %-28s %s",
			item.CreatedAt.Format("15:04:05"),
			item.Kind,
			formatBytes(item.ContentLength),
			cache,
			source,
			shortURL(candidateLabel(item), labelW),
			shortURL(item.URL, urlW),
		)
		if i == m.selected {
			line = selectedStyle.Render("> " + strings.TrimSpace(line))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if end < len(m.candidates) {
		more := len(m.candidates) - end
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more...", more)))
		b.WriteString("\n")
	}

	if m.downloading {
		b.WriteString(okStyle.Render(progressText(m.progress)))
		b.WriteString("\n")
		b.WriteString(okStyle.Render(progressBar(m.progress, max(10, lineW-2))))
		b.WriteString("\n")
	}

	return b.String()
}

func (m model) renderLogs(maxLines int) string {
	var b strings.Builder
	b.WriteString(mutedStyle.Render("日志: " + filepath.Join(m.runtime.Settings.DownloadDir, "wx-mini-video.log")))
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
		rendered := shortURL(line, m.width-2)
		b.WriteString(mutedStyle.Render("• " + rendered))
		b.WriteString("\n")
	}
	return b.String()
}

func (m *model) refresh() {
	prevLen := len(m.candidates)
	prevSelected := m.selected
	m.candidates = m.runtime.Candidates()
	if len(m.candidates) > prevLen && (prevLen == 0 || prevSelected == prevLen-1) {
		m.selected = len(m.candidates) - 1
		return
	}
	if m.selected >= len(m.candidates) {
		m.selected = len(m.candidates) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
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
	return fmt.Sprintf("%s %s %s", candidate.Kind, cache, shortURL(candidate.URL, 80))
}

func targetDisplay(target miniprogram.Target) string {
	if strings.TrimSpace(target.Name) == "" {
		return strings.TrimSpace(target.AppID)
	}
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(target.Name), strings.TrimSpace(target.AppID))
}

var recordTimeRegexp = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z-(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z`)

func candidateLabel(candidate miniprogram.Candidate) string {
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
