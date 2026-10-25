package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"wx_channel/internal/app"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/logo"
	"wx_channel/internal/tui/ui"
)

type startMsg struct {
	err      error
	duration time.Duration
	legacy   bool
}

type startupMsg struct{}

type tickMsg time.Time

type downloadMsg struct {
	candidateID string
	path        string
	err         error
}

type appearanceSavedMsg struct{ err error }

type logMsg string

type category string

const (
	categoryAll      category = "all"
	categoryImage    category = "image"
	categoryVideo    category = "video"
	categoryAudio    category = "audio"
	categoryLive     category = "live"
	categorySegments category = "segments"
)

// focusArea 是键盘焦点所在的界面区域。
type focusArea int

const (
	focusNone focusArea = iota
	focusAppearance
	focusInput
	focusPrograms
	focusConfirm
	focusCancel
)

type model struct {
	runtime *app.Runtime

	width   int
	height  int
	metrics ui.Metrics

	appearance ui.AppearanceMode
	dark       bool
	st         styles
	hits       *hitState

	// 目标选择区：输入与发现列表同时存在，确认后收起。
	targetReady     bool
	targetExpanded  bool
	appIDInput      string
	inputCursor     int
	inputErr        string
	focus           focusArea
	programs        []miniprogram.Program
	selectedProgram string // 按 AppID 保持选中身份，避免列表重排改选
	// programPicked 表示用户主动在发现列表里选过 AppID：确认按钮应优先确认该
	// 选择，而不是仍带着旧值的输入框；编辑输入框会清除该标记。
	programPicked bool

	// 外观下拉
	appearanceOpen bool
	appearanceIdx  int

	started      bool
	starting     bool
	startErr     error
	networkPause bool
	networkGen   int

	category        category
	candidates      []miniprogram.Candidate
	otherCandidates []miniprogram.Candidate
	selected        int
	selectedID      string
	mediaOffset     int
	mediaScrolled   bool
	showDetails     bool
	showLogs        bool
	logOffset       int

	downloading       bool
	progress          minidownload.Progress
	downloaded        map[string]string
	lastSavedPath     string
	downloadID        string
	downloadLive      bool
	downloadCancel    context.CancelFunc
	quitAfterDownload bool

	logs        []string
	notice      string
	noticeUntil time.Time

	stopping      bool
	stopErr       error
	confirmQuit   bool
	legacyPrompt  bool
	legacyWarning string
	legacyChecked bool

	logoCap logo.Capability
	painter *logo.Painter
}

func Run(runtime *app.Runtime) error {
	capability := logo.Probe()
	appearance := ui.LoadAppearance(runtime.Settings.DownloadDir)
	prefill := prepareDiscoveryTarget(runtime, runtime.Settings.Target.AppID)
	m := model{
		runtime:        runtime,
		appearance:     appearance,
		hits:           newHitState(),
		downloaded:     make(map[string]string),
		category:       categoryAll,
		appIDInput:     prefill,
		inputCursor:    len([]rune(prefill)),
		targetExpanded: true,
		focus:          focusInput,
		logs:           make([]string, 0, 64),
		logoCap:        capability,
	}
	var output io.Writer = os.Stdout
	if capability.Available() {
		m.painter = logo.NewPainter(os.Stdout, capability)
		output = m.painter
	}
	m.applyTheme()
	if capability.Available() {
		m.logs = append(m.logs, fmt.Sprintf("终端图片能力: %s，像素格 %dx%d（品牌 logo 使用真图）", capability.Protocol, capability.CellWidth, capability.CellHeight))
	} else {
		m.logs = append(m.logs, "终端图片能力: 不可用（品牌 logo 使用块字符方案）")
	}
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(output)}
	p := tea.NewProgram(m, options...)
	_, err := p.Run()
	if m.painter != nil {
		_ = m.painter.Close()
	}
	return err
}

func (m model) Init() tea.Cmd {
	// 启动即发现：程序打开后自动启动代理，发现结果只供选择，不自动确认目标。
	return tea.Batch(tick(), func() tea.Msg { return startupMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		m.metrics = ui.ComputeMetrics(v.Width, v.Height)
	case startupMsg:
		return m.ensureProxy()
	case tea.KeyMsg:
		return m.handleKey(v)
	case tea.MouseMsg:
		return m.handleMouse(v)
	case startMsg:
		m.starting = false
		if m.stopping {
			return m, m.stopProxy
		}
		if v.legacy {
			m.legacyPrompt = true
			return m, nil
		}
		m.started = v.err == nil
		m.startErr = v.err
		if v.err != nil {
			m.addLog("代理启动失败: " + v.err.Error())
			m.setNotice("代理启动失败："+v.err.Error()+"（按 r 重试）", 8*time.Second)
		} else {
			m.addLog(fmt.Sprintf("代理已启动: %s (耗时 %s)", m.runtime.ProxyAddr(), v.duration.Round(time.Millisecond)))
			if runtime.GOOS == "darwin" && m.runtime.Settings.ProxySystem {
				m.networkGen++
				return m, watchNetwork(m.runtime, m.networkGen)
			}
		}
	case networkMsg:
		if v.generation != m.networkGen || !m.started || m.stopping {
			return m, nil
		}
		if v.err == nil {
			return m, watchNetwork(m.runtime, m.networkGen)
		}
		m.started = false
		m.networkPause = true
		m.startErr = v.err
		m.networkGen++
		return m, func() tea.Msg { return networkStoppedMsg{err: m.runtime.Stop()} }
	case networkStoppedMsg:
		m.networkPause = false
		if m.stopping {
			return m, m.stopProxy
		}
		if v.err != nil {
			m.stopErr = v.err
		} else {
			m.addLog("网络已变化，已恢复原代理；按 r 重新启动代理")
			m.setNotice("网络已变化，已恢复原代理；按 r 重新启动代理", 8*time.Second)
		}
	case legacyCleanupMsg:
		m.starting = false
		if m.stopping {
			return m, m.stopProxy
		}
		m.legacyChecked = true
		if v.err != nil {
			m.legacyWarning = "旧共享根证书清理失败，系统信任风险仍然存在: " + v.err.Error()
			m.addLog(m.legacyWarning)
		} else {
			m.addLog("旧共享根证书已按指纹清理")
		}
		m.starting = true
		return m, m.startProxy
	case stopMsg:
		m.stopping = false
		m.stopErr = v.err
		if v.err == nil {
			return m, tea.Quit
		}
		m.started = false
		m.addLog("恢复系统代理或关闭代理失败: " + v.err.Error())
	case tickMsg:
		m.refresh()
		m.progress = m.runtime.DownloadProgress()
		if programs := m.runtime.Store.Programs(); programs != nil {
			m.programs = programs
			m.clampProgramSelection()
		}
		m.expireNotice()
		if m.appearance == ui.AppearanceSystem {
			if dark := ui.ResolveDark(ui.AppearanceSystem); dark != m.dark {
				m.applyTheme()
			}
		}
		return m, tick()
	case downloadMsg:
		m.downloading = false
		m.downloadID = ""
		m.downloadLive = false
		if m.downloadCancel != nil {
			m.downloadCancel()
			m.downloadCancel = nil
		}
		switch {
		case v.err == nil:
			m.lastSavedPath = v.path
			if v.candidateID != "" {
				m.downloaded[v.candidateID] = v.path
			}
			m.addLog("下载完成: " + v.path)
			m.setNotice("已保存 → "+m.relativeDownloadPath(v.path), 8*time.Second)
		case errors.Is(v.err, minidownload.ErrRecordingStopped):
			m.lastSavedPath = v.path
			if v.candidateID != "" {
				m.downloaded[v.candidateID] = v.path
			}
			m.addLog("已停止录制: " + v.path)
			m.setNotice("已停止录制并保存 → "+m.relativeDownloadPath(v.path), 8*time.Second)
		case errors.Is(v.err, minidownload.ErrRecordingInterrupted):
			m.lastSavedPath = v.path
			if v.candidateID != "" {
				m.downloaded[v.candidateID] = v.path
			}
			m.addLog("直播中断: " + v.err.Error())
			m.setNotice("直播中断，已保存已录内容 → "+m.relativeDownloadPath(v.path)+"（"+v.err.Error()+"）", 10*time.Second)
		case errors.Is(v.err, minidownload.ErrDownloadStopped):
			m.addLog("已停止下载，未完成内容保留在下载目录")
			m.setNotice("已停止下载（未完成内容保留在下载目录）", 8*time.Second)
		default:
			m.addLog("下载失败: " + v.err.Error())
			m.setNotice("下载失败："+v.err.Error()+"（Enter 重试 / l 查看日志）", 10*time.Second)
		}
		if m.quitAfterDownload {
			m.quitAfterDownload = false
			if quitAfterDownloadAllowed(v.err) {
				return m.beginStop()
			}
			// 收尾失败（如保存已录内容失败）：取消自动退出，保留现场与错误，由用户决定后续处理。
			m.addLog("已取消退出：下载/录制未正常收尾，请处理后重试")
			m.setNotice("已取消退出："+v.err.Error()+"；错误与 .part 已保留，处理后可再按 q 退出", 12*time.Second)
		}
	case appearanceSavedMsg:
		if v.err != nil {
			m.setNotice("外观已切换，但无法保存偏好：请检查下载目录的写入权限。", 8*time.Second)
		}
	case logMsg:
		m.addLog(string(v))
	}
	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.hits == nil {
		return m, nil
	}
	// 代理恢复失败界面只允许它的两个按钮，避免用户误触其他区域。
	if m.stopErr != nil {
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			region, ok := m.hits.find(msg.X, msg.Y)
			if ok {
				if region.action == hitStopRetry || region.action == hitStopForce {
					return m.activateHit(region)
				}
			}
		}
		return m, nil
	}
	if m.stopping || m.networkPause {
		return m, nil
	}
	modal := m.confirmQuit || m.legacyPrompt
	if !modal {
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			if m.appearanceOpen {
				return m, nil
			}
			if !m.showLogs && !m.targetExpanded {
				region, ok := m.hits.find(msg.X, msg.Y)
				if !ok || region.action != hitRow {
					return m, nil
				}
			}
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m.scrollBy(-3), nil
		case tea.MouseButtonWheelDown:
			return m.scrollBy(3), nil
		}
	}
	switch msg.Action {
	case tea.MouseActionMotion:
		region, ok := m.hits.find(msg.X, msg.Y)
		if m.appearanceOpen {
			if ok && (region.action == hitAppearanceOption || region.action == hitAppearanceControl) {
				m.hits.setHover(region.action, region.index)
			} else {
				m.hits.clearHover()
			}
			return m, nil
		}
		if ok && (!modal || overlayHitAction(region.action)) {
			m.hits.setHover(region.action, region.index)
		} else {
			m.hits.clearHover()
		}
		return m, nil
	case tea.MouseActionPress:
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		region, ok := m.hits.find(msg.X, msg.Y)
		if modal {
			if !ok || !overlayHitAction(region.action) {
				return m, nil
			}
			return m.activateHit(region)
		}
		if m.appearanceOpen {
			if ok {
				switch region.action {
				case hitAppearanceOption, hitAppearanceControl:
					return m.activateHit(region)
				}
			}
			m.appearanceOpen = false
			return m, nil
		}
		if !ok {
			if m.showLogs || m.showDetails {
				m.showLogs = false
				m.showDetails = false
			}
			return m, nil
		}
		return m.activateHit(region)
	default:
		return m, nil
	}
}

// overlayHitAction 判断命中动作是否属于退出确认/旧证书确认面板的按钮。
func overlayHitAction(action hitAction) bool {
	switch action {
	case hitQuitConfirm, hitOverlayCancel, hitLegacyClean, hitLegacyKeep:
		return true
	}
	return false
}

func (m model) scrollBy(delta int) model {
	if m.showLogs {
		m.logOffset -= delta
		if m.logOffset < 0 {
			m.logOffset = 0
		}
		maxOffset := len(m.logs)
		if m.logOffset > maxOffset {
			m.logOffset = maxOffset
		}
		return m
	}
	if m.targetExpanded {
		if len(m.programs) == 0 {
			return m
		}
		m.selectProgram(programDirection(delta))
		return m
	}
	if !m.targetReady {
		return m
	}
	rows, first := 0, len(m.filteredCandidates())
	for _, region := range m.hits.regions {
		if region.action == hitRow {
			rows++
			first = min(first, region.index)
		}
	}
	if rows > 0 {
		if !m.mediaScrolled {
			m.mediaOffset = first
		}
		m.mediaScrolled = true
		m.mediaOffset = max(0, min(m.mediaOffset+delta, len(m.filteredCandidates())-rows))
		m.hits.clearHover()
	}
	return m
}

// programDirection 把滚轮增量折算成发现列表的移动步数（正数向下）。
func programDirection(delta int) int {
	if delta > 0 {
		return 1
	}
	if delta < 0 {
		return -1
	}
	return 0
}

func (m model) activateHit(region hitRegion) (tea.Model, tea.Cmd) {
	switch region.action {
	case hitAppearanceControl:
		if m.appearanceOpen {
			m.appearanceOpen = false
		} else {
			m.openAppearanceDropdown()
		}
		return m, nil
	case hitAppearanceOption:
		m.appearanceIdx = region.index
		return m.applyAppearanceSelection()
	case hitTargetInput:
		m.focus = focusInput
		m.inputCursor = len([]rune(m.appIDInput))
		return m, nil
	case hitTargetProgram:
		if region.index >= 0 && region.index < len(m.programs) {
			m.selectedProgram = m.programs[region.index].AppID
			m.programPicked = true
			m.focus = focusPrograms
			return m.confirmSelectedProgram()
		}
		return m, nil
	case hitTargetConfirm:
		// 按钮动作与键盘焦点无关：鼠标点击“确认目标”始终执行确认，
		// 不会因为键盘焦点停在“取消”上而执行取消。
		return m.confirmTargetButton()
	case hitTargetCancel:
		m.cancelTargetSwitch()
		return m, nil
	case hitTargetRetry:
		return m.ensureProxy()
	case hitTab:
		if region.index >= 0 && region.index < len(categories) {
			m.setCategory(categories[region.index])
		}
		return m, nil
	case hitRow:
		return m.activateRow(region.index)
	case hitActionDownload:
		return m.downloadSelected()
	case hitActionStop:
		m.stopDownload()
		return m, nil
	case hitActionDetails:
		m.toggleDetails()
		return m, nil
	case hitActionLogs:
		m.toggleLogs()
		return m, nil
	case hitActionRefresh:
		return m.retryOrRefresh()
	case hitActionOpenDir:
		return m, m.openDirectory()
	case hitActionClear:
		m.clearCandidates()
		return m, nil
	case hitActionTargetSwitch:
		m.openTargetEditor()
		return m, nil
	case hitActionQuit:
		return m.requestQuit()
	case hitGitHub:
		return m, m.openRepository()
	case hitDrawerClose:
		m.showDetails = false
		return m, nil
	case hitLogClose:
		m.showLogs = false
		return m, nil
	case hitQuitConfirm:
		m.confirmQuit = false
		return m.confirmQuitNow()
	case hitOverlayCancel:
		m.confirmQuit = false
		m.showDetails = false
		m.showLogs = false
		return m, nil
	case hitLegacyClean:
		m.legacyPrompt = false
		m.starting = true
		return m, removeLegacyRootCmd
	case hitLegacyKeep:
		m.legacyPrompt = false
		m.legacyChecked = true
		m.legacyWarning = "旧共享根证书尚未清理，其系统信任风险仍然存在。"
		m.starting = true
		return m, m.startProxy
	case hitStopRetry:
		return m.beginStop()
	case hitStopForce:
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m model) activateRow(index int) (tea.Model, tea.Cmd) {
	visible := m.filteredCandidates()
	if index < 0 || index >= len(visible) {
		return m, nil
	}
	candidate := visible[index]
	m.selected = index
	m.selectedID = candidate.ID
	now := time.Now().UnixMilli()
	if m.hits.lastClickID == candidate.ID && m.hits.lastClickAt >= 0 && now-m.hits.lastClickAt < 500 {
		m.hits.lastClickID = ""
		m.hits.lastClickAt = -1
		return m.startDownload(candidate)
	}
	m.hits.lastClickID = candidate.ID
	m.hits.lastClickAt = now
	return m, nil
}

func (m *model) applyTheme() {
	m.dark = ui.ResolveDark(m.appearance)
	palette := ui.PaletteFor(m.dark)
	m.st = newStyles(palette)
	if m.painter != nil {
		m.painter.SetBackground(ui.RGB(palette.Background))
	}
}

func (m *model) setAppearance(mode ui.AppearanceMode) tea.Cmd {
	if !mode.Valid() {
		return nil
	}
	m.appearance = mode
	m.applyTheme()
	dir := m.runtime.Settings.DownloadDir
	return func() tea.Msg { return appearanceSavedMsg{err: ui.SaveAppearance(dir, mode)} }
}

func (m *model) setNotice(text string, duration time.Duration) {
	m.notice = text
	m.noticeUntil = time.Now().Add(duration)
}

func (m *model) expireNotice() {
	if m.notice != "" && time.Now().After(m.noticeUntil) {
		m.notice = ""
	}
}

func (m model) noticeText() string {
	if m.notice != "" {
		return m.notice
	}
	return ""
}

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

func (m model) startProxy() tea.Msg {
	startedAt := time.Now()
	if !m.legacyChecked && !m.runtime.Settings.ProxySkipRootCert {
		legacy, err := legacyRootPresent()
		if err != nil {
			return startMsg{err: fmt.Errorf("检查旧共享根证书失败: %w", err)}
		}
		if legacy {
			return startMsg{legacy: true}
		}
	}
	return startMsg{err: m.runtime.Start(), duration: time.Since(startedAt)}
}

func (m model) stopProxy() tea.Msg {
	return stopMsg{err: m.runtime.Stop()}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func downloadCmd(ctx context.Context, runtime *app.Runtime, candidate miniprogram.Candidate) tea.Cmd {
	return func() tea.Msg {
		path, err := runtime.Download(ctx, candidate, "")
		return downloadMsg{candidateID: candidate.ID, path: path, err: err}
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

func (m model) relativeDownloadPath(path string) string {
	root := m.runtime.Settings.DownloadDir
	if root == "" {
		return path
	}
	if rel, err := filepath.Rel(root, path); err == nil && !filepath.IsAbs(rel) && !hasDotDot(rel) {
		return "downloads/" + filepath.ToSlash(rel)
	}
	return path
}

func hasDotDot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, `..\`)
}

func (m model) showUnattributedEnabled() bool {
	if m.runtime == nil {
		return true
	}
	return !m.runtime.Settings.HideUnattributed
}
