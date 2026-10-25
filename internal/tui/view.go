package tui

import (
	"strings"

	"wx_channel/internal/tui/ui"
)

// view.go 负责整体布局：品牌区、渐变色带、主区、底栏与覆盖层。

type paneHit struct {
	x, y, w, h int
	action     hitAction
	index      int
}

func (m model) View() string {
	if m.hits == nil {
		m.hits = newHitState()
	}
	m.hits.reset()
	if m.stopErr != nil {
		m.disableLogo()
		return m.viewStopError()
	}
	if m.networkPause {
		m.disableLogo()
		return m.singleLineScreen(m.st.muted.Render("网络已变化，正在停止嗅探并恢复原代理..."))
	}
	if m.stopping {
		m.disableLogo()
		return m.singleLineScreen(m.st.muted.Render("正在恢复系统代理并退出..."))
	}

	metrics := ui.ComputeMetrics(m.screenWidth(), m.screenHeight())
	height := m.screenHeight()

	brand := m.renderBrand(0, metrics)
	ribbon := m.renderRibbon(metrics)
	footer := m.renderFooter()
	footerTop := height - len(footer)
	mainTop := len(brand) + 1
	mainHeight := footerTop - mainTop
	if mainHeight < 3 {
		mainHeight = 3
	}

	var main []string
	if !m.targetReady {
		main = m.renderTargetCard(mainTop, mainHeight, metrics)
	} else {
		main = m.renderMedia(mainTop, mainHeight, metrics)
	}

	canvas := make([]string, 0, height)
	canvas = append(canvas, brand...)
	canvas = append(canvas, ribbon)
	canvas = append(canvas, main...)
	canvas = append(canvas, footer...)
	for len(canvas) < height {
		canvas = append(canvas, m.blankLine())
	}
	if len(canvas) > height {
		canvas = canvas[:height]
	}

	if m.confirmQuit {
		pane, hits := m.quitConfirmPane()
		_, x, y := m.overlayPane(canvas, pane)
		m.addPaneHits(x, y, hits)
	}
	if m.legacyPrompt {
		pane, hits := m.legacyPane()
		_, x, y := m.overlayPane(canvas, pane)
		m.addPaneHits(x, y, hits)
	}
	if m.showTargets {
		pane, hits := m.targetOverlayPane(metrics)
		_, x, y := m.overlayPane(canvas, pane)
		m.addPaneHits(x, y, hits)
	}
	if m.showLogs {
		pane, hits := m.logOverlayPane(metrics)
		_, x, y := m.overlayPane(canvas, pane)
		m.addPaneHits(x, y, hits)
	}
	m.syncLogo(metrics)
	return strings.Join(canvas, "\n")
}

// disableLogo 在非主界面状态下清除终端图片。
func (m model) disableLogo() {
	if m.painter != nil {
		m.painter.SetEnabled(false)
	}
}

// overlayOpen 报告是否存在会遮挡品牌区的覆盖层。
func (m model) overlayOpen() bool {
	return m.confirmQuit || m.legacyPrompt || m.showTargets || m.showLogs
}

// logoSize 计算真图 logo 的单元格尺寸；不可用或空间不足时返回 ok=false。
func (m model) logoSize(metrics ui.Metrics) (cols, rows int, ok bool) {
	if m.painter == nil || !m.logoCap.Available() {
		return 0, 0, false
	}
	if metrics.Height < 32 || metrics.Width < 60 {
		return 0, 0, false
	}
	rows = 3
	if metrics.Width >= ui.CompactWidth {
		rows = 5
	}
	cellWidth, cellHeight := m.logoCap.CellWidth, m.logoCap.CellHeight
	if cellWidth <= 0 || cellHeight <= 0 {
		return 0, 0, false
	}
	cols = (rows*cellHeight + cellWidth - 1) / cellWidth
	if cols < 5 {
		cols = 5
	}
	if cols > 24 {
		cols = 24
	}
	if metrics.ContentWidth < cols+ui.BrandGap+40 {
		return 0, 0, false
	}
	return cols, rows, true
}

// syncLogo 把当前 logo 矩形同步给输出层；覆盖层或强制退出时关闭绘制。
func (m model) syncLogo(metrics ui.Metrics) {
	if m.painter == nil {
		return
	}
	cols, rows, ok := m.logoSize(metrics)
	if !ok || m.overlayOpen() {
		m.painter.SetEnabled(false)
		return
	}
	m.painter.SetEnabled(true)
	m.painter.SetPlacement(1, metrics.BrandPadX, cols, rows)
}

func (m model) singleLineScreen(line string) string {
	canvas := make([]string, 0, m.screenHeight())
	canvas = append(canvas, m.padLine(line))
	for len(canvas) < m.screenHeight() {
		canvas = append(canvas, m.blankLine())
	}
	if len(canvas) > m.screenHeight() {
		canvas = canvas[:m.screenHeight()]
	}
	return strings.Join(canvas, "\n")
}

func (m model) overlayPane(canvas []string, pane []string) ([]string, int, int) {
	if len(pane) == 0 {
		return canvas, 0, 0
	}
	paneWidth := 0
	for _, line := range pane {
		if w := ui.Width(line); w > paneWidth {
			paneWidth = w
		}
	}
	x := (m.screenWidth() - paneWidth) / 2
	if x < 0 {
		x = 0
	}
	y := (len(canvas) - len(pane)) / 2
	if y < 0 {
		y = 0
	}
	for i, line := range pane {
		row := y + i
		if row < 0 || row >= len(canvas) {
			continue
		}
		right := m.screenWidth() - x - ui.Width(line)
		if right < 0 {
			right = 0
		}
		canvas[row] = m.padLine(m.spaces(x), line, m.spaces(right))
	}
	return canvas, x, y
}

func (m *model) addPaneHits(x, y int, hits []paneHit) {
	for _, hit := range hits {
		m.hits.add(x+hit.x, y+hit.y, hit.w, hit.h, hit.action, hit.index)
	}
}

// isHover 报告某个可交互元素当前是否被鼠标悬停。
func (m model) isHover(action hitAction, index int) bool {
	return m.hits.isHovered(action, index)
}

func (m model) brandHeight(metrics ui.Metrics) int {
	if metrics.Short {
		return 2
	}
	height := 5
	if _, rows, ok := m.logoSize(metrics); ok && rows+2 > height {
		height = rows + 2
	}
	return height
}

func (m model) renderBrand(startY int, metrics ui.Metrics) []string {
	height := m.brandHeight(metrics)
	padX := metrics.BrandPadX
	contentWidth := metrics.Width - 2*padX
	if contentWidth <= 0 {
		return []string{m.blankLine()}
	}

	imageCols, imageRows, showImage := m.logoSize(metrics)
	showMark := !showImage && !metrics.Narrow && !metrics.Short
	var mark []string
	markWidth := 0
	if showMark {
		mark = ui.BrandMark(m.st.palette)
		markWidth = ui.BrandMarkWidth + ui.BrandGap
	}
	leftWidth := markWidth
	if showImage {
		leftWidth = imageCols + ui.BrandGap
	}

	compact := metrics.Compact
	segments, segWidth := m.appearanceSegments(compact)
	appearance := m.renderAppearanceControl(segments, compact)
	segY := startY + 1
	if metrics.Short {
		segY = startY
	}

	textLines := []string{
		m.st.kicker.Render("WX-MINI-VIDEO / 媒体嗅探"),
		m.st.title.Render("微信小程序媒体下载"),
		m.st.note.Render("保存小程序里值得留下的图片与音视频"),
	}
	if metrics.Short {
		textLines = []string{m.st.title.Render("微信小程序媒体下载")}
	} else if metrics.Compact {
		textLines = textLines[:2]
	}
	if metrics.Narrow {
		appearance = ""
	}

	rows := make([]string, 0, height)
	if !metrics.Short {
		rows = append(rows, m.blankLine())
	}
	contentHeight := height
	if !metrics.Short {
		contentHeight = height - 2
	}
	if contentHeight < len(textLines) {
		contentHeight = len(textLines)
	}
	if showImage && contentHeight < imageRows {
		contentHeight = imageRows
	}
	for i := 0; i < contentHeight; i++ {
		left := ""
		switch {
		case showImage:
			left = m.spaces(leftWidth)
		case showMark && i < len(mark):
			left = mark[i] + m.spaces(ui.BrandGap)
		case showMark:
			left = m.spaces(leftWidth)
		}
		text := ""
		if i < len(textLines) {
			text = textLines[i]
		}
		right := ""
		rightWidth := 0
		if i == 0 {
			right = appearance
			rightWidth = segWidth
		}
		textAvail := contentWidth - leftWidth - rightWidth - 1
		if textAvail < 8 {
			textAvail = 8
		}
		text = ui.Truncate(text, textAvail)
		leftPart := left + text
		gap := contentWidth - ui.Width(leftPart) - rightWidth
		if gap < 0 {
			gap = 0
		}
		rows = append(rows, m.padLine(m.spaces(padX), leftPart, m.spaces(gap), right, m.spaces(padX)))
	}
	if !metrics.Short {
		rows = append(rows, m.blankLine())
	}
	for len(rows) < height {
		rows = append(rows, m.blankLine())
	}
	if len(rows) > height {
		rows = rows[:height]
	}

	// 记录外观控件的点击区域。
	if appearance != "" {
		x := metrics.Width - padX - segWidth
		m.hits.add(x, segY, segWidth, 1, hitAppearanceSystem, 0)
		offset := 0
		for _, segment := range segments {
			m.hits.add(x+offset, segY, segment.width, 1, segment.action, 0)
			offset += segment.width
		}
	}
	return rows
}

type appearanceSegment struct {
	label  string
	width  int
	action hitAction
	active bool
}

func (m model) appearanceSegments(compact bool) ([]appearanceSegment, int) {
	labels := []struct {
		label  string
		action hitAction
		mode   ui.AppearanceMode
	}{
		{"系统", hitAppearanceSystem, ui.AppearanceSystem},
		{"浅色", hitAppearanceLight, ui.AppearanceLight},
		{"深色", hitAppearanceDark, ui.AppearanceDark},
	}
	if !compact {
		labels[0].label = "跟随系统"
	}
	segments := make([]appearanceSegment, 0, len(labels))
	total := 0
	for _, item := range labels {
		width := ui.Width(item.label) + 2
		segments = append(segments, appearanceSegment{
			label:  item.label,
			width:  width,
			action: item.action,
			active: m.appearance == item.mode,
		})
		total += width
	}
	if len(segments) > 1 {
		total += len(segments) - 1
	}
	if !compact {
		total += 5 // "外观 "
	}
	return segments, total
}

func (m model) renderAppearanceControl(segments []appearanceSegment, compact bool) string {
	var b strings.Builder
	if !compact {
		b.WriteString(m.st.muted.Render("外观 "))
	}
	for i, segment := range segments {
		style := m.st.tabInactive
		if segment.active {
			style = m.st.tabActive
		}
		if m.isHover(segment.action, 0) && !segment.active {
			style = m.st.buttonHover
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(style.Render(" " + segment.label + " "))
	}
	return b.String()
}

func (m model) renderRibbon(metrics ui.Metrics) string {
	padX := metrics.BrandPadX
	width := metrics.Width - 2*padX
	if width <= 0 {
		return m.blankLine()
	}
	return m.padLine(m.spaces(padX), ui.GradientLine(width, m.st.palette, m.dark), m.spaces(padX))
}

func (m model) footerHeight(metrics ui.Metrics) int {
	if metrics.Short {
		return 1
	}
	return 4
}

func (m model) renderFooter() []string {
	metrics := ui.ComputeMetrics(m.screenWidth(), m.screenHeight())
	height := m.footerHeight(metrics)
	top := m.screenHeight() - height
	padX := metrics.BrandPadX

	linkText := "GitHub · MuZiJin701/wx-mini-video ↗"
	linkStyle := m.st.link
	quitLabel := "退出 · Ctrl+Q"
	quitWidth := buttonWidth(quitLabel, 16)

	if metrics.Short {
		link := linkStyle.Render(ui.Truncate(linkText, metrics.Width-2*padX-ui.Width(quitLabel)-4))
		right := m.st.muted.Render(ui.Fit(" "+quitLabel+" ", quitWidth))
		line := m.padLine(m.spaces(padX), link, m.spaces(metrics.Width-padX-2*ui.Width(link)-quitWidth), right, m.spaces(padX))
		m.hits.add(metrics.Width-padX-quitWidth, top, quitWidth, 1, hitQuit, 0)
		m.hits.add(padX, top, ui.Width(link), 1, hitGitHub, 0)
		return []string{line}
	}

	quitLines := m.buttonLines(quitLabel, buttonSecondary, false, m.isHover(hitQuit, 0), quitWidth)
	rows := make([]string, 0, height)
	rows = append(rows, m.blankLine())
	for i := 0; i < len(quitLines); i++ {
		row := m.blankLine()
		rightX := metrics.Width - padX - quitWidth
		right := quitLines[i]
		var left string
		if i == 1 {
			link := linkStyle.Render(linkText)
			left = link
			m.hits.add(padX, top+1, ui.Width(link), 1, hitGitHub, 0)
		}
		leftWidth := ui.Width(left)
		gap := rightX - padX - leftWidth
		if gap < 0 {
			gap = 0
		}
		row = m.padLine(m.spaces(padX), left, m.spaces(gap), right, m.spaces(padX))
		rows = append(rows, row)
	}
	m.hits.add(metrics.Width-padX-quitWidth, top+1, quitWidth, len(quitLines), hitQuit, 0)
	for len(rows) < height {
		rows = append(rows, m.blankLine())
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

func (m model) viewStopError() string {
	metrics := ui.ComputeMetrics(m.screenWidth(), m.screenHeight())
	message := "系统代理可能仍指向本工具，网络可能不可用。"
	lines := []string{
		"",
		m.st.errStyle.Render("退出未完成：恢复系统代理或关闭代理失败"),
		"",
		m.st.text.Render(ui.Truncate(m.stopErr.Error(), metrics.Width-2*metrics.BrandPadX)),
		"",
		m.st.text.Render(message),
		m.st.text.Render("按 r 重试恢复并退出；按 f 明确强制退出（需要自行恢复网络设置）。"),
	}
	canvas := make([]string, 0, m.screenHeight())
	for _, line := range lines {
		canvas = append(canvas, m.padLine(m.spaces(metrics.BrandPadX), line))
	}
	for len(canvas) < m.screenHeight() {
		canvas = append(canvas, m.blankLine())
	}
	if len(canvas) > m.screenHeight() {
		canvas = canvas[:m.screenHeight()]
	}
	return strings.Join(canvas, "\n")
}
