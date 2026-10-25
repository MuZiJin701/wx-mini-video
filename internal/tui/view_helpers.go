package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/tui/ui"
)

// view_helpers.go 提供逐行拼装、按钮与覆盖层等共享的渲染原语。

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

// buttonLines 渲染一个带 1 格实线边框的按钮（高 3 行）。
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
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	content := ui.Fit(label, inner)
	rendered := style.Width(inner).Render(content)
	return strings.Split(rendered, "\n")
}

// buttonWidth 返回按钮文字的显示宽度 + 边框 + 内边距。
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

// placeOverlay 把面板逐行覆盖到画布中央；pane 每行宽度应一致。
func (m model) placeOverlay(canvas []string, pane []string) []string {
	if len(pane) == 0 || len(canvas) == 0 {
		return canvas
	}
	paneWidth := 0
	for _, line := range pane {
		if w := ui.Width(line); w > paneWidth {
			paneWidth = w
		}
	}
	totalWidth := m.screenWidth()
	x := (totalWidth - paneWidth) / 2
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
		left := m.spaces(x)
		rightWidth := totalWidth - x - ui.Width(line)
		if rightWidth < 0 {
			rightWidth = 0
		}
		canvas[row] = m.padLine(left, line, m.spaces(rightWidth))
	}
	return canvas
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
