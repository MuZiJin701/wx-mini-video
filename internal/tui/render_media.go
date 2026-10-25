package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

// render_media.go 负责媒体视图：目标条、分类页签、操作按钮、候选表格、详情抽屉、
// 状态行与日志浮层。

type tableColumn struct {
	title string
	width int
}

func (m model) tableColumns(metrics ui.Metrics) []tableColumn {
	width := metrics.ContentWidth
	typeWidth := 8
	if width < 84 {
		typeWidth = 6
	}
	showWide := width >= 72
	columns := []tableColumn{
		{"时间", 8},
		{"类型", typeWidth},
		{"归属", 4},
		{"大小", 8},
	}
	if showWide {
		columns = append(columns, tableColumn{"来源", 4})
	}
	columns = append(columns, tableColumn{"标题", 0})
	if showWide {
		columns = append(columns, tableColumn{"状态", 8})
	}
	return columns
}

func tableLayout(columns []tableColumn, width int) []tableColumn {
	fixed := 0
	flexIndex := -1
	for i := range columns {
		if columns[i].width == 0 {
			flexIndex = i
			continue
		}
		fixed += columns[i].width
	}
	gaps := len(columns) - 1
	remaining := width - fixed - gaps*2
	if flexIndex >= 0 {
		if remaining < 12 {
			remaining = 12
		}
		columns[flexIndex].width = remaining
	}
	return columns
}

func renderTableRow(columns []tableColumn, values []string, style interface{ Render(...string) string }, width int) string {
	var b strings.Builder
	for i, column := range columns {
		value := ""
		if i < len(values) {
			value = values[i]
		}
		cell := ui.Fit(value, column.width)
		b.WriteString(style.Render(cell))
		if i < len(columns)-1 {
			b.WriteString(style.Render("  "))
		}
	}
	line := b.String()
	current := ui.Width(line)
	if current < width {
		line += style.Render(strings.Repeat(" ", width-current))
	}
	return line
}

// mediaActionButtons 返回媒体视图的操作按钮；禁用状态与快捷键处理保持一致。
func (m model) mediaActionButtons() []actionButton {
	hasSelection := m.hasCandidate()
	return []actionButton{
		{label: "下载 · d", action: hitActionDownload, primary: true, enabled: !m.downloading && hasSelection},
		{label: "停止 · x", action: hitActionStop, enabled: m.downloading},
		{label: "详情 · i", action: hitActionDetails, enabled: hasSelection},
		{label: "日志 · l", action: hitActionLogs, enabled: true},
		{label: "刷新 · r", action: hitActionRefresh, enabled: true},
		{label: "打开目录 · o", action: hitActionOpenDir, enabled: true},
		{label: "清空 · c", action: hitActionClear, enabled: true},
		{label: "更换目标 · s", action: hitActionTargetSwitch, enabled: !m.downloading},
	}
}

func (m model) renderMedia(startY, height int, metrics ui.Metrics) []string {
	left := metrics.BrandPadX + metrics.ContentLeft
	rows := make([]string, 0, height)
	y := startY

	rows = append(rows, m.renderTargetBar(y, metrics))
	y++
	rows = append(rows, m.renderCategoryTabs(y, metrics))
	y++

	compact := metrics.Width < 80 || metrics.Height < 24
	actionRows, actionHits := m.renderButtons(m.mediaActionButtons(), left, metrics.ContentWidth, compact)
	actionTop := len(rows)
	for _, line := range actionRows {
		rows = append(rows, line)
	}
	y += len(actionRows)

	drawerLines := 0
	if m.showDetails {
		drawerLines = height / 2
		if drawerLines < 6 {
			drawerLines = 6
		}
		if drawerLines > 12 {
			drawerLines = 12
		}
	}
	tableLines := height - (2 + len(actionRows) + 1 + drawerLines + 1)
	if tableLines < 1 {
		drawerLines = 0
		tableLines = height - (2 + len(actionRows) + 1 + 1)
	}
	if tableLines < 1 {
		tableLines = 1
	}

	columns := tableLayout(m.tableColumns(metrics), metrics.ContentWidth-2)
	header := renderTableRow(columns, columnTitles(columns), m.st.muted, metrics.ContentWidth-2)
	rows = append(rows, m.padLine(m.spaces(left), header))
	y++
	tableRows, tableHits := m.renderCandidateTable(columns, tableLines, y, left, metrics)
	rows = append(rows, tableRows...)
	y += len(tableRows)

	if drawerLines > 0 && drawerLines <= height-len(rows) {
		rows = append(rows, m.renderDrawer(metrics, y, drawerLines)...)
		y += drawerLines
	}

	rows = append(rows, m.renderStatusLine(y, metrics))

	for len(rows) < height {
		rows = append(rows, m.blankLine())
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	for _, hit := range actionHits {
		m.hits.add(hit.x, startY+actionTop+hit.y, hit.w, hit.h, hit.action, hit.index)
	}
	for _, hit := range tableHits {
		m.hits.add(hit.x, hit.y, hit.w, hit.h, hit.action, hit.index)
	}
	return rows
}

func columnTitles(columns []tableColumn) []string {
	titles := make([]string, len(columns))
	for i, column := range columns {
		titles[i] = column.title
	}
	return titles
}

func (m model) renderCategoryTabs(y int, metrics ui.Metrics) string {
	left := metrics.BrandPadX + metrics.ContentLeft
	width := metrics.ContentWidth
	labels := m.categoryTabLabels(width)
	var parts []string
	x := left
	for i, item := range categories {
		style := m.st.tabInactive
		if item == m.activeCategory() {
			style = m.st.tabActive
		} else if m.isHover(hitTab, i) {
			style = m.st.tabHover
		}
		rendered := style.Render(labels[i])
		renderedWidth := ui.Width(rendered)
		if x-left+renderedWidth > width {
			break
		}
		parts = append(parts, rendered)
		m.hits.add(x, y, renderedWidth, 1, hitTab, i)
		x += renderedWidth
		if i < len(categories)-1 {
			parts = append(parts, " ")
			x++
		}
	}
	return m.padLine(m.spaces(left), strings.Join(parts, ""))
}

// categoryTabLabels 依次尝试“带计数完整标签”“无计数完整标签”“紧凑标签”，
// 选择能在一行内放下的第一种，保证小窗口也能看到全部分类页签。
func (m model) categoryTabLabels(width int) []string {
	variants := [][]string{
		makeCategoryLabels(func(index int, name string) string {
			return fmt.Sprintf(" %d %s(%d) ", index+1, name, m.categoryCount(categories[index]))
		}),
		makeCategoryLabels(func(index int, name string) string {
			return fmt.Sprintf(" %d %s ", index+1, name)
		}),
		makeCategoryLabels(func(index int, name string) string {
			return fmt.Sprintf("%d%s", index+1, name)
		}),
	}
	for _, labels := range variants {
		total := 0
		for _, label := range labels {
			total += ui.Width(label)
		}
		total += len(labels) - 1
		if total <= width {
			return labels
		}
	}
	return variants[len(variants)-1]
}

func makeCategoryLabels(format func(index int, name string) string) []string {
	labels := make([]string, len(categories))
	for i, item := range categories {
		labels[i] = format(i, categoryDisplay(item))
	}
	return labels
}

func (m model) candidateKindShort(candidate miniprogram.Candidate, width int) string {
	display := candidateKindDisplay(candidate)
	if ui.Width(display) > width {
		if candidate.Live {
			return "直播"
		}
		if miniprogram.MediaKind(candidate) == "m3u8" {
			return "分段"
		}
	}
	return display
}

func (m model) renderCandidateTable(columns []tableColumn, maxRows, startY, left int, metrics ui.Metrics) ([]string, []paneHit) {
	visible := m.filteredCandidates()
	rows := make([]string, 0, maxRows)
	var hits []paneHit
	if maxRows < 1 {
		return rows, hits
	}
	width := metrics.ContentWidth - 2
	if width < 12 {
		width = 12
	}

	if len(visible) == 0 {
		guidance := "暂无候选资源。请在 PC 微信打开目标小程序并重新加载图片或播放音视频。"
		if len(m.displayCandidates()) > 0 {
			guidance = fmt.Sprintf("当前分类暂无资源。共 %d 个候选（其中 %d 个来源未确认），按 1 查看全部。", len(m.displayCandidates()), len(m.otherCandidates))
		}
		top := maxRows / 2
		for i := 0; i < maxRows; i++ {
			if i == top {
				rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(ui.Truncate(guidance, width))))
				continue
			}
			rows = append(rows, m.blankLine())
		}
		return rows, hits
	}

	selected := m.selected
	windowAnchor := selected
	if windowAnchor < 0 {
		windowAnchor = 0
	}
	offset := 0
	if len(visible) > maxRows {
		offset = windowAnchor - maxRows/2
		if m.mediaScrolled {
			offset = m.mediaOffset
		}
		if offset < 0 {
			offset = 0
		}
		if offset+maxRows > len(visible) {
			offset = len(visible) - maxRows
		}
	}
	end := offset + maxRows
	if end > len(visible) {
		end = len(visible)
	}
	for i := offset; i < end; i++ {
		candidate := visible[i]
		values := m.rowValues(candidate, columns)
		var line string
		switch {
		case i == selected:
			line = renderTableRow(columns, values, m.st.selected, width)
		case m.isHover(hitRow, i):
			line = renderTableRow(columns, values, m.st.rowHover, width)
		default:
			base := m.st.rowEven
			if (i-offset)%2 == 1 {
				base = m.st.rowOdd
			}
			line = renderTableRow(columns, values, base, width)
		}
		rows = append(rows, m.padLine(m.spaces(left), line))
		hits = append(hits, paneHit{x: left, y: startY + len(rows) - 1, w: width, h: 1, action: hitRow, index: i})
	}
	for len(rows) < maxRows {
		rows = append(rows, m.blankLine())
	}
	return rows, hits
}

func (m model) rowValues(candidate miniprogram.Candidate, columns []tableColumn) []string {
	values := make([]string, len(columns))
	status := ""
	if m.downloading && m.downloadID == candidate.ID {
		status = "下载中"
		if m.progress.ID == candidate.ID && m.progress.Live {
			status = "录制中"
		}
	} else if m.downloadedPath(candidate) != "" {
		status = "已下载"
	}
	for i, column := range columns {
		switch column.title {
		case "时间":
			values[i] = candidate.CreatedAt.Format("15:04:05")
		case "类型":
			values[i] = m.candidateKindShort(candidate, column.width)
		case "归属":
			values[i] = attributionMarker(candidate)
		case "大小":
			values[i] = formatBytes(candidate.ContentLength)
		case "来源":
			values[i] = sourceDisplay(candidate.Source)
		case "标题":
			values[i] = candidateLabel(candidate)
		case "状态":
			values[i] = status
		}
	}
	return values
}

func (m model) renderDrawer(metrics ui.Metrics, startY, height int) []string {
	visible := m.filteredCandidates()
	left := metrics.BrandPadX + metrics.ContentLeft
	width := metrics.ContentWidth - 2
	if width < 12 {
		width = 12
	}
	rows := make([]string, 0, height)
	rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(strings.Repeat("─", width))))
	if len(visible) == 0 || m.selected < 0 || m.selected >= len(visible) {
		rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render("未选中候选资源。")))
	} else {
		candidate := visible[m.selected]
		details := m.candidateDetailLines(candidate, width)
		for _, line := range details {
			if len(rows) >= height-1 {
				break
			}
			rows = append(rows, m.padLine(m.spaces(left), m.st.text.Render(ui.Fit(line, width))))
		}
	}
	for len(rows) < height-1 {
		rows = append(rows, m.blankLine())
	}
	closeText := "关闭 · Esc / i"
	closeLine := m.padLine(m.spaces(left), m.st.closeHint.Render(closeText))
	m.hits.add(left, startY+height-1, ui.Width(closeText), 1, hitDrawerClose, 0)
	rows = append(rows, closeLine)
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

func (m model) candidateDetailLines(candidate miniprogram.Candidate, width int) []string {
	downloaded := m.downloadedPath(candidate)
	cachePath := candidate.CachedPath
	if cachePath == "" {
		cachePath = "未缓存"
	}
	lines := []string{
		"标题: " + displayValue(candidate.Title),
		"类型/来源: " + candidateKindDisplay(candidate) + " / " + sourceDisplay(candidate.Source),
		"归属: " + attributionDisplay(candidate),
	}
	switch {
	case candidate.Live:
		lines = append(lines, "直播: HTTP-FLV 直播流，下载会持续录制（不会自动结束，按 x 停止并保存）")
	case miniprogram.MediaKind(candidate) == "m3u8":
		lines = append(lines, "格式: m3u8/HLS 分段媒体（由播放列表描述，下载后合并为 mp4）")
	case candidate.Kind == "video":
		lines = append(lines, "格式: 直链视频（FFmpeg 合并产物见分段媒体分类）")
	}
	if downloaded != "" {
		lines = append(lines, "本地路径: "+downloaded)
	}
	lines = append(lines,
		"来源 AppID: "+displayValue(candidate.SourceAppID),
		"判定依据: "+candidateBasis(candidate),
		"URL: "+candidate.URL,
		"本地缓存: "+cachePath,
	)
	if candidate.ContentLength > 0 {
		lines = append(lines, "大小: "+formatBytes(candidate.ContentLength))
	}
	if host := candidateHost(candidate.URL); host != "" {
		lines = append(lines, "Host: "+host)
	}
	return lines
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func candidateHost(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i >= 0 {
		rest := rawURL[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return ""
}

func (m model) renderStatusLine(y int, metrics ui.Metrics) string {
	left := metrics.BrandPadX + metrics.ContentLeft
	width := metrics.ContentWidth
	text := m.statusText()
	style := m.st.text
	switch {
	case m.startErr != nil:
		style = m.st.errStyle
		text = "代理启动失败：" + m.startErr.Error() + "（点击此处或按 r 重试）"
		m.hits.add(left, y, width, 1, hitTargetRetry, 0)
	case m.downloading || m.progress.Status == "done" || m.progress.Status == "error" || m.progress.Status == "stopped" || m.progress.Status == "interrupted":
		style = m.st.success
		if m.progress.Status == "error" {
			style = m.st.errStyle
		}
		if (m.progress.Status == "stopped" && !m.progress.Live) || m.progress.Status == "interrupted" {
			style = m.st.warning
		}
		text = progressLine(m.progress)
	case m.noticeText() != "":
		style = m.st.warning
	case m.legacyWarning != "":
		style = m.st.warning
		text = m.legacyWarning
	case m.starting:
		text = "代理启动中..."
		style = m.st.muted
	case m.selected < 0 && len(m.filteredCandidates()) > 0:
		style = m.st.warning
		text = "选中项已移出当前分类，按 ↑/↓ 重新选择；按 1 查看全部。"
	}
	return m.padLine(m.spaces(left), style.Render(ui.Truncate(text, width)))
}

func (m model) statusText() string {
	if m.noticeText() != "" {
		return m.noticeText()
	}
	pool := m.displayCandidates()
	if len(pool) == 0 {
		return "提示：目标已选定后，在 PC 微信重新加载目标小程序的图片或播放音视频；来源未确认的媒体会以 ? 标记按类型显示。"
	}
	return fmt.Sprintf("共 %d 个候选，其中 %d 个来源未确认（?）。↑/↓ 选择，d/Enter 下载，i 详情，l 日志。", len(pool), countUnattributed(pool))
}

func progressLine(progress minidownload.Progress) string {
	switch progress.Status {
	case "prepare-ffmpeg":
		return "正在准备 ffmpeg，完成后会继续下载..."
	case "starting":
		return "准备下载..."
	case "cache-m3u8":
		return "正在缓存播放列表（分段媒体）..."
	case "segments":
		if progress.Items > 0 {
			pct := int(float64(progress.Completed) / float64(progress.Items) * 100)
			return fmt.Sprintf("分片下载中 %d%%  %d/%d  %s", pct, progress.Completed, progress.Items, formatBytes(progress.Downloaded))
		}
		return "分片下载中 " + formatBytes(progress.Downloaded)
	case "ffmpeg":
		if progress.Total > 0 && progress.Downloaded > 0 {
			pct := int(float64(progress.Downloaded) / float64(progress.Total) * 100)
			if pct > 100 {
				pct = 100
			}
			return fmt.Sprintf("分段媒体合并中 %d%%  %s/%s", pct, formatDuration(progress.Downloaded), formatDuration(progress.Total))
		}
		return "分段媒体合并中..."
	case "downloading":
		if progress.Total > 0 {
			pct := int(float64(progress.Downloaded) / float64(progress.Total) * 100)
			return fmt.Sprintf("下载中 %d%%  %s/%s · 按 x 停止", pct, formatBytes(progress.Downloaded), formatBytes(progress.Total))
		}
		return "下载中 " + formatBytes(progress.Downloaded) + " · 按 x 停止"
	case "recording":
		return "录制直播中 " + formatBytes(progress.Downloaded) + "（不会自动结束，按 x 停止并保存）"
	case "stopped":
		if progress.Live {
			return "直播录制已停止，已保存: " + progress.Path
		}
		return "下载已停止（未完成内容保留在下载目录）"
	case "interrupted":
		return "直播中断并保存: " + progress.Path + "（" + progress.Error + "）"
	case "done":
		return "下载完成: " + progress.Path
	case "error":
		return "下载失败: " + progress.Error
	default:
		return "下载中，请稍候..."
	}
}

func countUnattributed(candidates []miniprogram.Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Attribution == miniprogram.AttributionUnattributed {
			count++
		}
	}
	return count
}

func (m model) logOverlayPane(metrics ui.Metrics) ([]string, []paneHit) {
	width := metrics.Width - 2*metrics.BrandPadX
	if width > 100 {
		width = 100
	}
	if width < 40 {
		width = min(metrics.Width-2, 40)
	}
	inner := width - 2
	bodyRows := m.screenHeight() - 8
	if bodyRows < 3 {
		bodyRows = 3
	}
	total := len(m.logs)
	start := total - bodyRows - m.logOffset
	if start < 0 {
		start = 0
	}
	end := start + bodyRows
	if end > total {
		end = total
	}
	var interior []string
	for i := start; i < end; i++ {
		interior = append(interior, m.interiorLine(" "+m.logs[i], inner, m.st.text))
	}
	for len(interior) < bodyRows {
		interior = append(interior, m.interiorLine("", inner, m.st.text))
	}
	path := filepath.Join(m.runtime.Settings.DownloadDir, "wx-mini-video.log")
	pane := m.panelFrame("日志", interior, "关闭 · Esc / l   "+path, width)
	hits := []paneHit{
		{x: 1, y: len(pane) - 2, w: inner, h: 1, action: hitLogClose},
	}
	return pane, hits
}
