package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

// render_media.go 负责媒体视图：目标条、分类页签、候选表格、详情抽屉、状态行与日志浮层。

type tableColumn struct {
	title string
	width int
}

func (m model) tableColumns(metrics ui.Metrics) []tableColumn {
	width := metrics.ContentWidth
	columns := []tableColumn{
		{"时间", 8},
		{"类型", 4},
		{"归属", 4},
		{"大小", 8},
	}
	if width >= 72 {
		columns = append(columns, tableColumn{"来源", 4})
	}
	columns = append(columns, tableColumn{"标题", 0})
	if width >= 72 {
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

func (m model) renderMedia(startY, height int, metrics ui.Metrics) []string {
	padX := metrics.BrandPadX
	left := padX + metrics.ContentLeft
	rows := make([]string, 0, height)
	y := startY

	rows = append(rows, m.renderTargetBar(y, metrics))
	y++
	rows = append(rows, m.renderCategoryTabs(y, metrics))
	y++

	drawerLines := 0
	if m.showDetails {
		drawerLines = min(10, max(6, height/2))
	}
	tableLines := height - 4 - drawerLines
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

	if m.showDetails {
		rows = append(rows, m.renderDrawer(metrics, drawerLines)...)
	}

	rows = append(rows, m.renderStatusLine(y, metrics))

	for len(rows) < height {
		rows = append(rows, m.blankLine())
	}
	if len(rows) > height {
		rows = rows[:height]
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

func (m model) renderTargetBar(y int, metrics ui.Metrics) string {
	left := metrics.BrandPadX + metrics.ContentLeft
	target := m.runtime.Settings.Target.AppID
	ffmpeg := "ffmpeg 下载 m3u8 时准备"
	if m.runtime.HasFFmpeg() {
		ffmpeg = "ffmpeg 就绪"
	}
	switchLabel := " 切换 · s "
	style := m.st.tabInactive
	if m.isHover(hitTargetSwitch, 0) {
		style = m.st.buttonHover
	}
	right := style.Render(switchLabel)
	prefix := m.st.title.Render("目标 ") + m.st.emphasis.Render(target)
	prefixWidth := ui.Width(prefix)
	infoRoom := metrics.ContentWidth - prefixWidth - ui.Width(right) - 4
	info := ""
	if infoRoom > 10 {
		info = m.st.muted.Render(ui.Truncate("  · 代理 "+m.runtime.ProxyAddr()+" · "+ffmpeg, infoRoom))
	}
	used := prefixWidth + ui.Width(info)
	gap := metrics.Width - left - used - ui.Width(right) - metrics.BrandPadX
	if gap < 0 {
		gap = 0
	}
	line := m.padLine(m.spaces(left), prefix, info, m.spaces(gap), right, m.spaces(metrics.BrandPadX))
	m.hits.add(metrics.Width-metrics.BrandPadX-ui.Width(right), y, ui.Width(right), 1, hitTargetSwitch, 0)
	return line
}

func (m model) renderCategoryTabs(y int, metrics ui.Metrics) string {
	left := metrics.BrandPadX + metrics.ContentLeft
	showCounts := metrics.ContentWidth >= 70
	var parts []string
	x := left
	for i, item := range categories {
		label := fmt.Sprintf(" %d %s ", i+1, categoryDisplay(item))
		if showCounts {
			label = fmt.Sprintf(" %d %s(%d) ", i+1, categoryDisplay(item), m.categoryCount(item))
		}
		label = ui.Truncate(label, metrics.ContentWidth)
		style := m.st.tabInactive
		if item == m.activeCategory() {
			style = m.st.tabActive
		} else if m.isHover(hitTab, i) {
			style = m.st.buttonHover
		}
		rendered := style.Render(label)
		width := ui.Width(rendered)
		if x-left+width > metrics.ContentWidth {
			break
		}
		parts = append(parts, rendered)
		m.hits.add(x, y, width, 1, hitTab, i)
		x += width
		if i < len(categories)-1 && len(parts) > 0 {
			parts = append(parts, " ")
			x++
		}
	}
	return m.padLine(m.spaces(left), strings.Join(parts, ""))
}

func (m model) renderCandidateTable(columns []tableColumn, maxRows, startY, left int, metrics ui.Metrics) ([]string, []paneHit) {
	visible := m.filteredCandidates()
	rows := make([]string, 0, maxRows)
	var hits []paneHit
	width := metrics.ContentWidth - 2

	if len(visible) == 0 {
		var guidance string
		if len(m.displayCandidates()) == 0 {
			guidance = "暂无候选资源。请在 PC 微信打开目标小程序并重新加载图片或播放音视频。"
			if !m.targetReady {
				guidance = "未选择目标。"
			}
		} else {
			guidance = fmt.Sprintf("当前分类暂无资源。共 %d 个候选（其中 %d 个来源未确认），按 1 查看全部或 Tab 切换分类。", len(m.displayCandidates()), len(m.otherCandidates))
		}
		top := startY + maxRows/2
		for i := 0; i < maxRows; i++ {
			rowY := startY + i
			if rowY == top {
				rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(ui.Truncate(guidance, width))))
				continue
			}
			rows = append(rows, m.blankLine())
		}
		return rows, hits
	}

	rowCount := maxRows
	if len(visible) > maxRows {
		rowCount = maxRows - 2
	}
	if rowCount < 1 {
		rowCount = 1
	}
	start := m.selected - rowCount/2
	if start < 0 {
		start = 0
	}
	if start+rowCount > len(visible) {
		start = len(visible) - rowCount
	}
	if start < 0 {
		start = 0
	}
	end := start + rowCount
	if end > len(visible) {
		end = len(visible)
	}

	if start > 0 {
		rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(fmt.Sprintf("  ↑ 还有 %d 条", start))))
	}
	for i := start; i < end; i++ {
		candidate := visible[i]
		values := m.rowValues(candidate, columns)
		var line string
		if i == m.selected {
			line = renderTableRow(columns, values, m.st.selected, width)
		} else if m.isHover(hitRow, i) {
			line = renderTableRow(columns, values, m.st.buttonHover, width)
		} else {
			base := m.st.rowEven
			if (i-start)%2 == 1 {
				base = m.st.rowOdd
			}
			line = renderTableRow(columns, values, base, width)
		}
		rows = append(rows, m.padLine(m.spaces(left), line))
		hits = append(hits, paneHit{x: left, y: startY + len(rows) - 1, w: width, h: 1, action: hitRow, index: i})
	}
	if end < len(visible) {
		rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(fmt.Sprintf("  ↓ 还有 %d 条", len(visible)-end))))
	}
	return rows, hits
}

func (m model) rowValues(candidate miniprogram.Candidate, columns []tableColumn) []string {
	values := []string{
		candidate.CreatedAt.Format("15:04:05"),
		kindDisplay(candidate.Kind),
		attributionMarker(candidate),
		formatBytes(candidate.ContentLength),
		sourceDisplay(candidate.Source),
		candidateLabel(candidate),
	}
	status := ""
	if m.downloading && m.downloadID == candidate.ID {
		status = "下载中"
	} else if m.downloadedPath(candidate) != "" {
		status = "已下载"
	}
	switch len(columns) {
	case 5:
		values = []string{values[0], values[1], values[2], values[3], values[5]}
	case 7:
		values = append(values, status)
	}
	return values
}

func (m model) renderDrawer(metrics ui.Metrics, height int) []string {
	visible := m.filteredCandidates()
	left := metrics.BrandPadX + metrics.ContentLeft
	width := metrics.ContentWidth - 2
	rows := make([]string, 0, height)
	rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render(strings.Repeat("─", width))))
	if len(visible) == 0 || m.selected < 0 || m.selected >= len(visible) {
		rows = append(rows, m.padLine(m.spaces(left), m.st.muted.Render("未选中候选资源。")))
		return rows
	}
	candidate := visible[m.selected]
	details := m.candidateDetailLines(candidate, width)
	for _, line := range details {
		if len(rows) >= height {
			break
		}
		rows = append(rows, m.padLine(m.spaces(left), m.st.text.Render(ui.Fit(line, width))))
	}
	for len(rows) < height {
		rows = append(rows, m.blankLine())
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
		"类型/来源: " + kindDisplay(candidate.Kind) + " / " + sourceDisplay(candidate.Source),
		"归属: " + attributionDisplay(candidate),
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
		text = "代理启动失败：" + m.startErr.Error() + "（按 r 重试）"
		m.hits.add(left, y, width, 1, hitRetryProxy, 0)
	case m.downloading || m.progress.Status == "done" || m.progress.Status == "error":
		style = m.st.success
		if m.progress.Status == "error" {
			style = m.st.errStyle
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
	}
	return m.padLine(m.spaces(left), style.Render(ui.Truncate(text, width)))
}

func (m model) statusText() string {
	if m.noticeText() != "" {
		return m.noticeText()
	}
	pool := m.displayCandidates()
	if len(pool) == 0 {
		return "提示：确认目标后，在 PC 微信重新加载目标小程序的图片或播放音视频；来源未确认的媒体会以 ? 标记按类型显示。"
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
		return "正在缓存 m3u8..."
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
			return fmt.Sprintf("ffmpeg 合并中 %d%%  %s/%s", pct, formatDuration(progress.Downloaded), formatDuration(progress.Total))
		}
		return "ffmpeg 合并中..."
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
