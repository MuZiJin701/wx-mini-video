package tui

import (
	"fmt"
	"strings"

	"wx_channel/internal/tui/ui"
	"wx_channel/pkg/certificate"
)

// render_target.go 负责目标选择卡与目标选择覆盖层。

func (m model) renderTargetCard(startY, height int, metrics ui.Metrics) []string {
	padX := metrics.BrandPadX
	left := padX + metrics.ContentLeft
	innerWidth := metrics.ContentWidth - 2
	if innerWidth < 24 {
		innerWidth = 24
	}
	inputWidth := innerWidth
	if inputWidth > 60 {
		inputWidth = 60
	}
	inputInner := inputWidth - 2

	startLabel := "开始嗅探 · Enter"
	discoverLabel := "自动发现 · Ctrl+D"
	startWidth := buttonWidth(startLabel, 18)
	discoverWidth := buttonWidth(discoverLabel, 20)

	block := make([]string, 0, 14)
	block = append(block, m.st.title.Render("目标小程序"))
	block = append(block, m.st.muted.Render(ui.Truncate("输入目标 AppID，或按 Ctrl+D 自动发现小程序。", innerWidth)))
	block = append(block, "")
	block = append(block, m.st.entryLabel.Render("AppID"))
	inputText := m.appIDInput + "▌"
	if m.appIDInput == "" {
		inputText = "▌"
	}
	inputRendered := m.st.inputFocused.Width(inputInner).Render(ui.Truncate(inputText, inputInner))
	block = append(block, strings.Split(inputRendered, "\n")...)
	switch {
	case m.inputErr != "":
		block = append(block, m.st.errStyle.Render(ui.Truncate(m.inputErr, innerWidth)))
	case m.startErr != nil:
		block = append(block, m.st.errStyle.Render(ui.Truncate("代理启动失败: "+m.startErr.Error()+"；按 r 重试", innerWidth)))
	case m.starting:
		block = append(block, m.st.muted.Render("正在启动代理..."))
	default:
		block = append(block, "")
	}
	block = append(block, "")
	startBtn := m.buttonLines(startLabel, buttonPrimary, true, m.isHover(hitStart, 0), startWidth)
	discoverBtn := m.buttonLines(discoverLabel, buttonSecondary, false, m.isHover(hitDiscover, 0), discoverWidth)
	buttonRow := len(block)
	block = append(block, joinButtons(startBtn, discoverBtn)...)
	block = append(block, "")
	hint := "提示：AppID 可在微信「更多资料」中查看，通常以 wx 开头；来源未确认的媒体会以 ? 标记按类型显示。"
	if m.legacyWarning != "" {
		block = append(block, m.st.warning.Render(ui.Truncate(m.legacyWarning, innerWidth)))
	} else {
		block = append(block, m.st.muted.Render(ui.Truncate(hint, innerWidth)))
	}

	top := startY
	if height > len(block) {
		top = startY + (height-len(block))/2
	}
	rows := make([]string, height)
	for i := range rows {
		rows[i] = m.blankLine()
	}
	for i, line := range block {
		row := top - startY + i
		if row < 0 || row >= height {
			continue
		}
		rows[row] = m.padLine(m.spaces(left), line)
	}

	btnY := top + buttonRow
	m.hits.add(left, btnY, startWidth, 3, hitStart, 0)
	m.hits.add(left+startWidth+1, btnY, discoverWidth, 3, hitDiscover, 0)
	if m.startErr != nil {
		m.hits.add(left, top+7, innerWidth, 1, hitRetryProxy, 0)
	}
	return rows
}

func (m model) targetOverlayPane(metrics ui.Metrics) ([]string, []paneHit) {
	width := metrics.ContentWidth
	if width > 72 {
		width = 72
	}
	if width < 40 {
		width = min(metrics.Width-2, 40)
	}
	inner := width - 2
	titleRows := 2 // 标题行 + 分隔行
	maxRows := m.screenHeight() - 9
	if maxRows < 3 {
		maxRows = 3
	}
	visible := len(m.programs)
	if visible > maxRows {
		visible = maxRows
	}
	start := 0
	if m.programIdx >= visible {
		start = m.programIdx - visible + 1
	}

	var interior []string
	var hits []paneHit
	if len(m.programs) == 0 {
		interior = append(interior, m.interiorLine("还没有发现小程序。请在 PC 微信打开目标小程序，", inner, m.st.muted))
		interior = append(interior, m.interiorLine("或按 Ctrl+D 回到手动输入 AppID。", inner, m.st.muted))
	} else {
		for i := start; i < start+visible && i < len(m.programs); i++ {
			program := m.programs[i]
			marker := "  "
			if program.AppID == m.runtime.Settings.Target.AppID && m.targetReady {
				marker = "● "
			}
			text := fmt.Sprintf("%s%-20s 最近出现 %s", marker, program.AppID, program.LastSeen.Format("15:04:05"))
			style := m.st.text
			if i == m.programIdx {
				style = m.st.selected
			} else if m.isHover(hitOverlayProgram, i) {
				style = m.st.buttonHover
			}
			interior = append(interior, m.interiorLine(" "+ui.Truncate(text, inner-1), inner, style))
			hits = append(hits, paneHit{x: 1, y: titleRows + (i - start), w: inner, h: 1, action: hitOverlayProgram, index: i})
		}
	}
	pane := m.panelFrame("选择小程序", interior, "↑/↓ 选择 · Enter 确认 · Esc 取消 · r 刷新", width)
	return pane, hits
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
