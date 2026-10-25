package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Width 返回字符串在终端中的显示宽度。
func Width(value string) int {
	return lipgloss.Width(value)
}

// PadRight 用空格把文本补齐到 width 列；已超出时原样返回。
func PadRight(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if current := Width(value); current < width {
		return value + strings.Repeat(" ", width-current)
	}
	return value
}

// Truncate 按显示宽度截断文本并以省略号结尾。
func Truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	limit := width - 1
	var b strings.Builder
	for _, r := range value {
		next := b.String() + string(r)
		if Width(next) > limit {
			break
		}
		b.WriteString(string(r))
	}
	return b.String() + "…"
}

// Fit 按显示宽度截断并补齐到 width 列，保证返回行的显示宽度恰好等于 width。
func Fit(value string, width int) string {
	if width <= 0 {
		return ""
	}
	value = Truncate(value, width)
	return PadRight(value, width)
}
