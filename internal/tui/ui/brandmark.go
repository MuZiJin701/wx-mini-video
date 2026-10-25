package ui

import "github.com/charmbracelet/lipgloss"

// BrandMark 返回块字符绘制的品牌图标（真图不可用时的降级方案），
// 宽度 10 列、高度 3 行。调用方负责按需垂直居中。
func BrandMark(p Palette) []string {
	border := lipgloss.NewStyle().Foreground(p.Primary)
	glyph := lipgloss.NewStyle().Foreground(p.ButtonFG).Bold(true)
	line0 := border.Render(" ▄▄▄▄▄▄▄▄ ")
	line1 := border.Render("▐") + glyph.Render("  ▶  ↓  ") + border.Render("▌")
	line2 := border.Render(" ▀▀▀▀▀▀▀▀ ")
	return []string{line0, line1, line2}
}

// BrandMarkWidth 是 BrandMark 的显示宽度。
const BrandMarkWidth = 10
