package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette 是一套外观模式下的语义色 token，沿用参考项目 channels-light/channels-dark 的绿系色值。
type Palette struct {
	Primary       lipgloss.Color
	Accent        lipgloss.Color
	Foreground    lipgloss.Color
	Background    lipgloss.Color
	Surface       lipgloss.Color
	Panel         lipgloss.Color
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	Muted         lipgloss.Color
	Line          lipgloss.Color
	EntryBG       lipgloss.Color
	ButtonFG      lipgloss.Color
	GradientStart lipgloss.Color
	GradientEnd   lipgloss.Color
}

// RGB 把 lipgloss 颜色解析为 RGB 三元组，供终端图片背景使用。
func RGB(color lipgloss.Color) [3]uint8 {
	hex := strings.TrimPrefix(string(color), "#")
	var out [3]uint8
	if len(hex) != 6 {
		return out
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return out
	}
	return [3]uint8{uint8(r), uint8(g), uint8(b)}
}

// PaletteFor 返回对应明暗模式的色板。
func PaletteFor(dark bool) Palette {
	if dark {
		return Palette{
			Primary:       lipgloss.Color("#8ce5a8"),
			Accent:        lipgloss.Color("#b6f0cb"),
			Foreground:    lipgloss.Color("#dee0e3"),
			Background:    lipgloss.Color("#283e2c"),
			Surface:       lipgloss.Color("#1c3021"),
			Panel:         lipgloss.Color("#35543c"),
			Success:       lipgloss.Color("#afedba"),
			Warning:       lipgloss.Color("#ffd38a"),
			Error:         lipgloss.Color("#ffadc0"),
			Muted:         lipgloss.Color("#afc9b5"),
			Line:          lipgloss.Color("#608069"),
			EntryBG:       lipgloss.Color("#192e20"),
			ButtonFG:      lipgloss.Color("#142b1b"),
			GradientStart: lipgloss.Color("#4cc06b"),
			GradientEnd:   lipgloss.Color("#b6f0cb"),
		}
	}
	return Palette{
		Primary:       lipgloss.Color("#285633"),
		Accent:        lipgloss.Color("#164b2b"),
		Foreground:    lipgloss.Color("#0c0d0e"),
		Background:    lipgloss.Color("#88a28c"),
		Surface:       lipgloss.Color("#a8bfae"),
		Panel:         lipgloss.Color("#77977d"),
		Success:       lipgloss.Color("#164d28"),
		Warning:       lipgloss.Color("#583500"),
		Error:         lipgloss.Color("#7d1731"),
		Muted:         lipgloss.Color("#2d4934"),
		Line:          lipgloss.Color("#55775c"),
		EntryBG:       lipgloss.Color("#cbdccf"),
		ButtonFG:      lipgloss.Color("#ffffff"),
		GradientStart: lipgloss.Color("#285633"),
		GradientEnd:   lipgloss.Color("#b6f0cb"),
	}
}
