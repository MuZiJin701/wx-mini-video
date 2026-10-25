package tui

import (
	"fmt"

	"wx_channel/internal/tui/ui"
	"wx_channel/pkg/certificate"
)

// render_target.go 负责统一目标选择区（输入 + 发现列表）与目标条。

// renderTargetArea 渲染展开状态的目标选择区：AppID 输入、启动/发现状态与持续更新的
// 已发现列表同时存在，确认后由调用方收起。
func (m model) renderTargetArea(startY, height int, metrics ui.Metrics) []string {
	left := metrics.BrandPadX + metrics.ContentLeft
	width := metrics.ContentWidth
	if width < 24 {
		width = 24
	}
	compact := metrics.Width < 80 || metrics.Height < 24

	heading := m.st.accent.Render("目标小程序")
	hint := m.st.muted.Render(ui.Truncate("输入 AppID，或从下方已发现列表选择；启动即自动发现，发现不会自动选定目标。", width))
	label := m.st.entryLabel.Render("AppID")
	inputWidth := width
	if inputWidth > 64 {
		inputWidth = 64
	}
	if inputWidth < 24 {
		inputWidth = 24
	}
	inputLines := m.inputFieldLines(m.appIDInput, m.inputCursor, inputWidth, m.focus == focusInput)
	status := m.targetStatusLine(width)
	listLabel := m.st.muted.Render(fmt.Sprintf("已发现的小程序（%d）", len(m.programs)))
	tail := m.st.muted.Render(ui.Truncate("Tab/Shift+Tab 切换焦点 · Enter 确认 · Esc 取消/退出", width))

	buttons := m.targetActionButtons()
	buttonRows, buttonHits := m.renderButtons(buttons, left, width, compact)

	fixed := 1 + 1 + len(inputLines) + 1 + len(buttonRows) // 标题、标签、输入框、状态、按钮
	showTail := height >= fixed+1
	if showTail {
		fixed++
	}
	showHint := height >= fixed+2
	if showHint {
		fixed++
	}
	showListLabel := height >= fixed+1
	if showListLabel {
		fixed++
	}
	listBudget := height - fixed
	if listBudget < 0 {
		listBudget = 0
	}

	rows := make([]string, 0, height)
	rows = append(rows, m.padLine(m.spaces(left), heading))
	if showHint {
		rows = append(rows, m.padLine(m.spaces(left), hint))
	}
	rows = append(rows, m.padLine(m.spaces(left), label))
	inputY := len(rows)
	for _, line := range inputLines {
		rows = append(rows, m.padLine(m.spaces(left), line))
	}
	rows = append(rows, m.padLine(m.spaces(left), status))
	buttonTop := len(rows)
	for _, line := range buttonRows {
		rows = append(rows, line)
	}
	if showListLabel {
		rows = append(rows, m.padLine(m.spaces(left), listLabel))
	}
	listTop := len(rows)
	listRows, listHits := m.discoveryRows(listBudget, width)
	for _, line := range listRows {
		rows = append(rows, m.padLine(m.spaces(left), line))
	}
	if showTail {
		rows = append(rows, m.padLine(m.spaces(left), tail))
	}
	for len(rows) < height {
		rows = append(rows, m.blankLine())
	}
	if len(rows) > height {
		rows = rows[:height]
	}

	// 命中区域：输入框、发现列表、按钮。行号相对主区起点，必须加上 startY
	// 才能与画布上实际显示的行对齐。
	m.hits.add(left, startY+inputY, inputWidth, len(inputLines), hitTargetInput, 0)
	for _, hit := range listHits {
		m.hits.add(left+hit.x, startY+listTop+hit.y, hit.w, hit.h, hit.action, hit.index)
	}
	for _, hit := range buttonHits {
		m.hits.add(hit.x, startY+buttonTop+hit.y, hit.w, hit.h, hit.action, hit.index)
	}
	return rows
}

// targetActionButtons 返回目标选择区的确认/取消/重试按钮。
func (m model) targetActionButtons() []actionButton {
	buttons := make([]actionButton, 0, 4)
	if m.startErr != nil && !m.starting {
		buttons = append(buttons, actionButton{label: "重试 · Ctrl+R", action: hitTargetRetry, enabled: true})
	} else {
		buttons = append(buttons, actionButton{label: "刷新 · Ctrl+R", action: hitActionRefresh, enabled: true})
	}
	buttons = append(buttons, actionButton{label: "目录 · Ctrl+O", action: hitActionOpenDir, enabled: true})
	buttons = append(buttons, actionButton{
		label:   "确认目标 · Enter",
		action:  hitTargetConfirm,
		primary: true,
		enabled: true,
		focused: m.focus == focusConfirm,
	})
	if m.targetReady && m.targetExpanded {
		buttons = append(buttons, actionButton{
			label:   "取消 · Esc",
			action:  hitTargetCancel,
			enabled: true,
			focused: m.focus == focusCancel,
		})
	}
	return buttons
}

// targetStatusLine 渲染启动状态与发现状态。
func (m model) targetStatusLine(width int) string {
	var text string
	style := m.st.muted
	switch {
	case m.startErr != nil:
		style = m.st.errStyle
		text = "代理启动失败：" + m.startErr.Error() + "（点击重试或按 Ctrl+R）"
	case m.starting:
		text = "正在启动代理，自动发现小程序…"
	case m.started:
		style = m.st.success
		text = fmt.Sprintf("代理运行中 %s · 已发现 %d 个小程序 · 请在 PC 微信打开目标小程序", m.runtime.ProxyAddr(), len(m.programs))
	default:
		text = "等待代理启动…"
	}
	if m.legacyWarning != "" {
		text = m.legacyWarning
		style = m.st.warning
	}
	return style.Render(ui.Truncate(text, width))
}

// discoveryRows 渲染发现列表窗口，maxRows 行；返回相对行首的命中区域。
func (m model) discoveryRows(maxRows, width int) ([]string, []paneHit) {
	rows := make([]string, 0, maxRows)
	var hits []paneHit
	if maxRows <= 0 {
		return rows, hits
	}
	if len(m.programs) == 0 {
		placeholder := "尚未发现小程序：请在 PC 微信打开目标小程序；仍可直接输入 AppID。"
		rows = append(rows, m.st.muted.Render(ui.Truncate(placeholder, width)))
		for len(rows) < maxRows {
			rows = append(rows, "")
		}
		return rows, hits
	}
	selected := m.currentProgramIndex()
	if selected < 0 {
		selected = 0
	}
	start := selected - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > len(m.programs) {
		start = len(m.programs) - maxRows
	}
	if start < 0 {
		start = 0
	}
	for index := start; index < len(m.programs) && len(rows) < maxRows; index++ {
		program := m.programs[index]
		prefix := "  "
		if index == selected && m.focus == focusPrograms {
			prefix = "› "
		}
		marker := "  "
		if m.targetReady && program.AppID == m.runtime.Settings.Target.AppID {
			marker = "● "
		}
		text := fmt.Sprintf("%s%s%-24s  最近出现 %s", prefix, marker, program.AppID, program.LastSeen.Format("15:04:05"))
		style := m.st.text
		switch {
		case index == selected:
			style = m.st.selected
		case m.isHover(hitTargetProgram, index):
			style = m.st.rowHover
		}
		rows = append(rows, style.Render(ui.Fit(text, width)))
		hits = append(hits, paneHit{x: 0, y: len(rows) - 1, w: width, h: 1, action: hitTargetProgram, index: index})
	}
	for len(rows) < maxRows {
		rows = append(rows, "")
	}
	return rows, hits
}

// renderTargetBar 渲染收起后的当前目标条与更换目标按钮。
func (m model) renderTargetBar(y int, metrics ui.Metrics) string {
	left := metrics.BrandPadX + metrics.ContentLeft
	target := m.runtime.Settings.Target.AppID
	ffmpeg := "ffmpeg 下载分段媒体时准备"
	if m.runtime.HasFFmpeg() {
		ffmpeg = "ffmpeg 就绪"
	}
	switchLabel := "更换目标 · s"
	switchChip := m.chipText(switchLabel, hitActionTargetSwitch, false, !m.downloading)
	switchWidth := ui.Width(switchChip)
	prefix := m.st.title.Render("目标 ") + m.st.emphasis.Render(target)
	prefixWidth := ui.Width(prefix)
	infoRoom := metrics.ContentWidth - prefixWidth - switchWidth - 4
	info := ""
	if infoRoom > 10 {
		info = m.st.muted.Render(ui.Truncate("  · 代理 "+m.runtime.ProxyAddr()+" · "+ffmpeg, infoRoom))
	}
	used := prefixWidth + ui.Width(info)
	gap := metrics.Width - left - used - switchWidth - metrics.BrandPadX
	if gap < 0 {
		gap = 0
	}
	line := m.padLine(m.spaces(left), prefix, info, m.spaces(gap), switchChip, m.spaces(metrics.BrandPadX))
	if !m.downloading {
		m.hits.add(metrics.Width-metrics.BrandPadX-switchWidth, y, switchWidth, 1, hitActionTargetSwitch, 0)
	}
	return line
}

func (m model) quitConfirmPane() ([]string, []paneHit) {
	width := 52
	if width > m.screenWidth()-2 {
		width = m.screenWidth() - 2
	}
	inner := width - 2
	message := "正在下载，退出会停止当前下载。"
	if m.downloadLive {
		message = "正在录制直播，退出会停止录制并保存已录内容。"
	}
	body := []string{
		m.interiorLine(message, inner, m.st.text),
		m.interiorLine("未完成部分会保留在下载目录。", inner, m.st.muted),
		m.interiorLine("", inner, m.st.text),
	}
	confirmLabel := "确认退出 · Enter"
	cancelLabel := "取消 · Esc"
	confirmWidth := buttonWidth(confirmLabel, 18)
	cancelWidth := buttonWidth(cancelLabel, 14)
	confirmBtn := m.buttonLines(confirmLabel, buttonPrimary, true, m.isHover(hitQuitConfirm, 0), confirmWidth)
	cancelBtn := m.buttonLines(cancelLabel, buttonSecondary, false, m.isHover(hitOverlayCancel, 0), cancelWidth)
	buttonRow := len(body)
	for _, row := range joinButtons(confirmBtn, cancelBtn) {
		body = append(body, m.interiorLine(" "+row, inner, m.st.text))
	}
	pane := m.panelFrame("退出确认", body, "", width)
	// 面板结构：边框、标题、分隔、body…、边框；按钮位于 body 起点的 buttonRow。
	top := 1 + 2 + buttonRow
	hits := []paneHit{
		{x: 2, y: top, w: confirmWidth, h: 3, action: hitQuitConfirm},
		{x: 2 + confirmWidth + 1, y: top, w: cancelWidth, h: 3, action: hitOverlayCancel},
	}
	return pane, hits
}

func (m model) legacyPane() ([]string, []paneHit) {
	width := 68
	if width > m.screenWidth()-2 {
		width = m.screenWidth() - 2
	}
	inner := width - 2
	body := []string{
		m.interiorLine("检测到旧版共享根证书（私钥公开）。", inner, m.st.warning),
		m.interiorLine("", inner, m.st.text),
		m.interiorLine("指纹: "+certificate.LegacyRootThumbprint, inner, m.st.muted),
		m.interiorLine("", inner, m.st.text),
		m.interiorLine("安装独立证书不会撤销旧证书的系统信任。", inner, m.st.text),
		m.interiorLine("将检查并清理本机与当前用户的根证书库。", inner, m.st.text),
		m.interiorLine("删除可能影响同样依赖此证书的其他代理工具。", inner, m.st.text),
		m.interiorLine("", inner, m.st.text),
	}
	cleanLabel := "清理旧证书 · y"
	keepLabel := "保留并继续 · n"
	cleanWidth := buttonWidth(cleanLabel, 18)
	keepWidth := buttonWidth(keepLabel, 18)
	cleanBtn := m.buttonLines(cleanLabel, buttonPrimary, true, m.isHover(hitLegacyClean, 0), cleanWidth)
	keepBtn := m.buttonLines(keepLabel, buttonSecondary, false, m.isHover(hitLegacyKeep, 0), keepWidth)
	buttonRow := len(body)
	for _, row := range joinButtons(cleanBtn, keepBtn) {
		body = append(body, m.interiorLine(" "+row, inner, m.st.text))
	}
	pane := m.panelFrame("旧共享根证书", body, "按 q 退出程序", width)
	top := 1 + 2 + buttonRow
	hits := []paneHit{
		{x: 2, y: top, w: cleanWidth, h: 3, action: hitLegacyClean},
		{x: 2 + cleanWidth + 1, y: top, w: keepWidth, h: 3, action: hitLegacyKeep},
	}
	return pane, hits
}
