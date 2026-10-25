package tui

import (
	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/tui/ui"
)

// styles 是按当前色板构建的一组 lipgloss 样式；外观切换时整体重建。
type styles struct {
	palette ui.Palette

	screen   lipgloss.Style
	text     lipgloss.Style
	muted    lipgloss.Style
	title    lipgloss.Style
	kicker   lipgloss.Style
	note     lipgloss.Style
	success  lipgloss.Style
	warning  lipgloss.Style
	errStyle lipgloss.Style
	accent   lipgloss.Style
	emphasis lipgloss.Style

	selected    lipgloss.Style
	selectedDim lipgloss.Style

	rowEven lipgloss.Style
	rowOdd  lipgloss.Style

	tabActive   lipgloss.Style
	tabInactive lipgloss.Style

	button        lipgloss.Style
	buttonFocused lipgloss.Style
	buttonHover   lipgloss.Style
	primaryButton lipgloss.Style
	primaryFocus  lipgloss.Style
	primaryHover  lipgloss.Style

	frame        lipgloss.Style
	frameFocused lipgloss.Style
	frameLine    lipgloss.Style

	input        lipgloss.Style
	inputFocused lipgloss.Style

	entryLabel lipgloss.Style

	panelTitle lipgloss.Style

	link        lipgloss.Style
	linkFocused lipgloss.Style
	footerHint  lipgloss.Style

	progressBar  lipgloss.Style
	progressRest lipgloss.Style
}

func newStyles(p ui.Palette) styles {
	border := lipgloss.NormalBorder()
	base := lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Background)
	return styles{
		palette: p,
		screen:  base,
		text:    base,
		muted:   base.Foreground(p.Muted),
		title:   base.Foreground(p.Foreground).Bold(true),
		kicker:  base.Foreground(p.Muted).Bold(true),
		note:    base.Foreground(p.Muted),
		success: base.Foreground(p.Success),
		warning: base.Foreground(p.Warning),
		errStyle: base.
			Foreground(p.Error),
		accent:   base.Foreground(p.Accent).Bold(true),
		emphasis: base.Bold(true),

		selected:    lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary),
		selectedDim: lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Foreground(p.Muted),

		rowEven: lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Background),
		rowOdd:  lipgloss.NewStyle().Foreground(p.Foreground).Background(p.Surface),

		tabActive:   lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true),
		tabInactive: lipgloss.NewStyle().Foreground(p.Muted).Background(p.Background),

		button: base.
			Foreground(p.Foreground).
			Border(border, true, true, true, true).
			BorderForeground(p.Line).
			Padding(0, 1),
		buttonFocused: base.
			Foreground(p.Foreground).
			Border(border, true, true, true, true).
			BorderForeground(p.Foreground).
			Bold(true).
			Padding(0, 1),
		buttonHover: base.
			Foreground(p.Foreground).
			Background(p.Surface).
			Border(border, true, true, true, true).
			BorderForeground(p.Accent).
			Padding(0, 1),
		primaryButton: lipgloss.NewStyle().
			Foreground(p.ButtonFG).
			Background(p.Primary).
			Border(border, true, true, true, true).
			BorderForeground(p.Accent).
			Bold(true).
			Padding(0, 1),
		primaryFocus: lipgloss.NewStyle().
			Foreground(p.ButtonFG).
			Background(p.Primary).
			Border(border, true, true, true, true).
			BorderForeground(p.Foreground).
			Bold(true).
			Padding(0, 1),
		primaryHover: lipgloss.NewStyle().
			Foreground(p.Background).
			Background(p.Accent).
			Border(border, true, true, true, true).
			BorderForeground(p.Foreground).
			Bold(true).
			Padding(0, 1),

		frame: base.
			Border(border, true, true, true, true).
			BorderForeground(p.Line).
			Background(p.Background),
		frameLine: base.Foreground(p.Accent),
		frameFocused: base.
			Border(border, true, true, true, true).
			BorderForeground(p.Accent),

		input: base.
			Foreground(p.Foreground).
			Background(p.EntryBG).
			Border(border, true, true, true, true).
			BorderForeground(p.Line).
			Padding(0, 1),
		inputFocused: base.
			Foreground(p.Foreground).
			Background(p.EntryBG).
			Border(border, true, true, true, true).
			BorderForeground(p.Accent).
			Padding(0, 1),

		entryLabel: base.Foreground(p.Muted),

		panelTitle: base.Bold(true),

		link:        base.Foreground(p.Muted).Underline(true),
		linkFocused: lipgloss.NewStyle().Foreground(p.ButtonFG).Background(p.Primary).Bold(true).Underline(true),
		footerHint:  base.Foreground(p.Muted),

		progressBar:  base.Foreground(p.Accent),
		progressRest: base.Foreground(p.Line),
	}
}
