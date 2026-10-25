package ui

import (
	"fmt"
	"strings"
)

// GradientLine 生成一条左右插值的真彩渐变色带，用于品牌区分隔线。
// 色带由每个单元格独立设置背景色，末尾重置样式。
func GradientLine(width int, p Palette, dark bool) string {
	if width <= 0 {
		return ""
	}
	start := hexRGB(string(p.GradientStart))
	end := hexRGB(string(p.GradientEnd))
	var b strings.Builder
	for i := 0; i < width; i++ {
		t := 0.0
		if width > 1 {
			t = float64(i) / float64(width-1)
		}
		r := int(float64(start[0]) + (float64(end[0])-float64(start[0]))*t)
		g := int(float64(start[1]) + (float64(end[1])-float64(start[1]))*t)
		bl := int(float64(start[2]) + (float64(end[2])-float64(start[2]))*t)
		fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm ", r, g, bl)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func hexRGB(hex string) [3]int {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return [3]int{}
	}
	var out [3]int
	fmt.Sscanf(hex, "%02x%02x%02x", &out[0], &out[1], &out[2])
	return out
}
