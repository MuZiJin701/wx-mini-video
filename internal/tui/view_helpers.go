package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/tui/ui"
)

// view_helpers.go 提供逐行拼装、按钮、输入框与覆盖层等共享的渲染原语。

func (m model) screenWidth() int {
	if m.width > 0 {
		return m.width
	}
	return 80
}

func (m model) screenHeight() int {
	if m.height > 0 {
		return m.height
	}
	return 24
}

func (m model) metricsNow() ui.Metrics {
	return ui.ComputeMetrics(m.screenWidth(), m.screenHeight())
}

func (m model) blankLine() string {
	return m.st.screen.Render(strings.Repeat(" ", m.screenWidth()))
}

func (m model) spaces(count int) string {
	if count <= 0 {
		return ""
	}
	return m.st.screen.Render(strings.Repeat(" ", count))
}

// padLine 把若干已渲染片段拼成一行并补齐到屏幕宽度。
func (m model) padLine(segments ...string) string {
	joined := strings.Join(segments, "")
	current := ui.Width(joined)
	if current < m.screenWidth() {
		return joined + m.spaces(m.screenWidth()-current)
	}
	return joined
}

// fillTo 把文本补齐到指定显示宽度（用基础背景样式填空格）。
func (m model) fillTo(text string, width int) string {
	current := ui.Width(text)
	if current < width {
		return text + m.spaces(width-current)
	}
	return text
}

// buttonKind 区分主按钮、次按钮的视觉状态。
type buttonKind int

const (
	buttonSecondary buttonKind = iota
	buttonPrimary
)

// buttonLines 渲染一个带 1 格实线边框的按钮（高 3 行）。width 是包含边框与
// 内边距的总宽度；除颜色外，各状态尺寸完全一致。
func (m model) buttonLines(label string, kind buttonKind, focused, hovered bool, width int) []string {
	var style lipgloss.Style
	if kind == buttonPrimary {
		style = m.st.primaryButton
		if hovered {
			style = m.st.primaryHover
		}
		if focused {
			style = m.st.primaryFocus
		}
	} else {
		style = m.st.button
		if hovered {
			style = m.st.buttonHover
		}
		if focused {
			style = m.st.buttonFocused
		}
	}
	return renderFramedButton(style, label, width)
}

// renderFramedButton 用一致的内宽渲染 3 行按钮，保证显示宽度等于 width。
func renderFramedButton(style lipgloss.Style, label string, width int) []string {
	content := width - 4 // 2 列边框 + 左右各 1 列内边距
	if content < 1 {
		content = 1
	}
	label = strings.Repeat(" ", max(0, (content-ui.Width(label))/2)) + label
	rendered := style.Render(ui.Fit(label, content))
	return strings.Split(rendered, "\n")
}

// buttonWidth 返回按钮的总宽度：文字 + 边框 + 内边距，且不小于 minWidth。
func buttonWidth(label string, minWidth int) int {
	width := ui.Width(label) + 4
	if width < minWidth {
		width = minWidth
	}
	return width
}

// joinButtons 把多个按钮按行横向拼接，按钮之间留 1 列间距。
func joinButtons(groups ...[]string) []string {
	height := 0
	for _, group := range groups {
		if len(group) > height {
			height = len(group)
		}
	}
	out := make([]string, height)
	for row := 0; row < height; row++ {
		var parts []string
		for _, group := range groups {
			if row < len(group) {
				parts = append(parts, group[row])
			} else {
				parts = append(parts, "")
			}
		}
		out[row] = strings.Join(parts, " ")
	}
	return out
}

// actionButton 是媒体视图与目标选择区里的可点击动作按钮。
type actionButton struct {
	label   string
	action  hitAction
	primary bool
	enabled bool
	focused bool
}

// compact 用单行 chip 形态渲染，避免小窗口被按钮占满。
func (m model) buttonWidthFor(button actionButton, compact bool) int {
	if compact {
		return ui.Width(button.label) + 2
	}
	return buttonWidth(button.label, 16)
}

func (m model) buttonStyleFor(button actionButton) lipgloss.Style {
	if !button.enabled {
		return m.st.buttonDisabled
	}
	if button.primary {
		if button.focused {
			return m.st.primaryFocus
		}
		if m.isHover(button.action, 0) {
			return m.st.primaryHover
		}
		return m.st.primaryButton
	}
	if button.focused {
		return m.st.buttonFocused
	}
	if m.isHover(button.action, 0) {
		return m.st.buttonHover
	}
	return m.st.button
}

func (m model) chipStyleFor(button actionButton) lipgloss.Style {
	if !button.enabled {
		return m.st.chipDisabled
	}
	if button.primary {
		if m.isHover(button.action, 0) {
			return m.st.chipHover
		}
		return m.st.chipActive
	}
	if button.focused {
		return m.st.chipFocused
	}
	if m.isHover(button.action, 0) {
		return m.st.chipHover
	}
	return m.st.chip
}

// renderButtons 把按钮按宽度自动换行。compact 为真时使用单行 chip 形态，
// 否则使用 3 行带边框按钮；返回的命中区域 y 以返回行数组的起点为 0。
func (m model) renderButtons(buttons []actionButton, left, width int, compact bool) ([]string, []paneHit) {
	var lines []string
	var hits []paneHit
	if width <= 0 {
		return lines, hits
	}

	type placedButton struct {
		actionButton
		width int
	}
	var row []placedButton
	rowWidth := 0

	flush := func() {
		if len(row) == 0 {
			return
		}
		top := len(lines)
		if compact {
			var parts []string
			x := left
			for _, item := range row {
				style := m.chipStyleFor(item.actionButton)
				rendered := style.Render(item.label)
				parts = append(parts, rendered)
				if item.enabled {
					hits = append(hits, paneHit{x: x, y: top, w: item.width, h: 1, action: item.action, index: 0})
				}
				x += item.width + 1
			}
			lines = append(lines, m.padLine(m.spaces(left), strings.Join(parts, " ")))
		} else {
			var groups [][]string
			x := left
			for _, item := range row {
				style := m.buttonStyleFor(item.actionButton)
				groups = append(groups, renderFramedButton(style, item.label, item.width))
				if item.enabled {
					hits = append(hits, paneHit{x: x, y: top, w: item.width, h: 3, action: item.action, index: 0})
				}
				x += item.width + 1
			}
			rows := joinButtons(groups...)
			for _, line := range rows {
				lines = append(lines, m.padLine(m.spaces(left), line))
			}
		}
		row = nil
		rowWidth = 0
	}

	for _, button := range buttons {
		w := m.buttonWidthFor(button, compact)
		if rowWidth > 0 && rowWidth+1+w > width {
			flush()
		}
		row = append(row, placedButton{actionButton: button, width: w})
		rowWidth += w
		if len(row) > 1 {
			rowWidth++
		}
	}
	flush()
	return lines, hits
}

// chipText 渲染单个单行 chip（目标条更换按钮、外观下拉控件）。
func (m model) chipText(label string, action hitAction, active, enabled bool) string {
	button := actionButton{label: label, action: action, enabled: enabled, primary: active}
	return m.chipStyleFor(button).Render(label)
}

// inputFieldLines 渲染 3 行输入框。totalWidth 包含边框与内边距；光标以 ▌ 显示，
// 文本超过可视宽度时围绕光标截取窗口。
func (m model) inputFieldLines(text string, cursor int, totalWidth int, focused bool) []string {
	if totalWidth < 8 {
		totalWidth = 8
	}
	content := totalWidth - 4
	if content < 4 {
		content = 4
	}
	style := m.st.input
	if focused {
		style = m.st.inputFocused
	}
	display := inputDisplay(text, cursor, content)
	rendered := style.Render(ui.Fit(display, content))
	lines := strings.Split(rendered, "\n")
	for len(lines) < 3 {
		lines = append(lines, "")
	}
	return lines[:3]
}

// inputDisplay 生成带光标的单行文本窗口，显示宽度不超过 width。
func inputDisplay(text string, cursor, width int) string {
	runes := []rune(text)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	before := string(runes[:cursor])
	after := string(runes[cursor:])
	display := before + "▌" + after
	if ui.Width(display) <= width {
		return display
	}
	// 光标左侧超宽时丢弃左侧字符，保证光标可见。
	overflow := ui.Width(display) - width
	cut := 0
	for cut < cursor {
		if ui.Width(string(runes[:cut+1])) > overflow {
			break
		}
		cut++
	}
	display = string(runes[cut:])
	if ui.Width(display) > width {
		display = ui.Truncate(display, width)
	}
	return display
}

// overlayAt 把面板覆盖到画布指定坐标；左右两侧保留原有内容，越界部分被裁掉。
func (m model) overlayAt(canvas []string, pane []string, x, y int) ([]string, int, int) {
	if len(pane) == 0 || len(canvas) == 0 {
		return canvas, x, y
	}
	paneWidth := 0
	for _, line := range pane {
		if w := ui.Width(line); w > paneWidth {
			paneWidth = w
		}
	}
	if x < 0 {
		x = 0
	}
	if x+paneWidth > m.screenWidth() {
		x = m.screenWidth() - paneWidth
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if y+len(pane) > len(canvas) {
		y = len(canvas) - len(pane)
	}
	if y < 0 {
		y = 0
	}
	for i, line := range pane {
		row := y + i
		if row < 0 || row >= len(canvas) {
			continue
		}
		canvas[row] = composeOverlayRow(canvas[row], line, x, paneWidth, m.screenWidth())
	}
	return canvas, x, y
}

// ansiTokenRegexp 匹配 SGR/CSI 与 OSC 转义序列，用于按显示宽度横向合成覆盖层。
var ansiTokenRegexp = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\].*?(?:\x07|\x1b\\)`)

type ansiToken struct {
	escape bool
	text   string
	width  int
}

func ansiTokens(line string) []ansiToken {
	tokens := make([]ansiToken, 0, len(line)/2)
	for len(line) > 0 {
		if loc := ansiTokenRegexp.FindStringIndex(line); loc != nil && loc[0] == 0 {
			tokens = append(tokens, ansiToken{escape: true, text: line[:loc[1]]})
			line = line[loc[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(line)
		tokens = append(tokens, ansiToken{text: line[:size], width: ui.Width(string(r))})
		line = line[size:]
	}
	return tokens
}

// composeOverlayRow 把 pane 横向嵌入 base 的 [x, x+paneWidth) 列，保留两侧内容。
func composeOverlayRow(base, pane string, x, paneWidth, screenWidth int) string {
	tokens := ansiTokens(base)
	var out strings.Builder
	col := 0
	for _, token := range tokens {
		if token.escape {
			if col < x {
				out.WriteString(token.text)
			}
			continue
		}
		if col+token.width <= x {
			out.WriteString(token.text)
			col += token.width
			continue
		}
		if col < x {
			out.WriteString(strings.Repeat(" ", x-col))
			col = x
		}
		break
	}
	if col < x {
		out.WriteString(strings.Repeat(" ", x-col))
	}
	out.WriteString("\x1b[0m")
	out.WriteString(pane)
	out.WriteString("\x1b[0m")

	col = 0
	var suffix strings.Builder
	for _, token := range tokens {
		if token.escape {
			if col >= x+paneWidth {
				suffix.WriteString(token.text)
			}
			continue
		}
		if col >= x+paneWidth {
			suffix.WriteString(token.text)
		} else if col+token.width > x+paneWidth {
			suffix.WriteString(strings.Repeat(" ", x+paneWidth-col))
		}
		col += token.width
	}
	out.WriteString(suffix.String())
	width := ui.Width(out.String())
	if width < screenWidth {
		out.WriteString(strings.Repeat(" ", screenWidth-width))
	}
	return out.String()
}

// panelFrame 生成带边框与标题的覆盖层面板；interior 每行必须正好是内部宽度。
func (m model) panelFrame(title string, interior []string, footer string, width int) []string {
	if width < 24 {
		width = 24
	}
	maxWidth := m.screenWidth() - 2
	if maxWidth < 24 {
		maxWidth = m.screenWidth()
	}
	if width > maxWidth {
		width = maxWidth
	}
	inner := width - 2
	frame := m.st.frameLine
	bar := func(left, fill, right string) string {
		return frame.Render(left + strings.Repeat(fill, inner) + right)
	}
	rows := []string{bar("┌", "─", "┐")}
	if title != "" {
		rows = append(rows, frame.Render("│")+m.st.panelTitle.Render(ui.Fit(" "+title, inner))+frame.Render("│"))
		rows = append(rows, bar("├", "─", "┤"))
	}
	for _, line := range interior {
		rows = append(rows, frame.Render("│")+line+frame.Render("│"))
	}
	if footer != "" {
		rows = append(rows, bar("├", "─", "┤"))
		rows = append(rows, frame.Render("│")+m.st.muted.Render(ui.Fit(" "+footer, inner))+frame.Render("│"))
	}
	rows = append(rows, bar("└", "─", "┘"))
	return rows
}

// interiorLine 把纯文本按内部宽度截断/补齐并应用样式。
func (m model) interiorLine(plain string, width int, style lipgloss.Style) string {
	return style.Render(ui.Fit(plain, width))
}

// horizontalRule 返回一条细横线（用于抽屉/分区）。
func (m model) horizontalRule(width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	return style.Render(strings.Repeat("─", width))
}
