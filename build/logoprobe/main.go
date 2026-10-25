// Command logoprobe is the standalone prototype for the terminal bitmap logo.
//
//	go run ./build/logoprobe
//
// It probes the terminal before Bubble Tea starts, then renders the embedded
// PNG in the top five rows while a counter, scrolling list, spinner and theme
// switch generate continuous redraw pressure. Run it in a real terminal (the
// validation target is Windows Terminal); q exits and removes a kitty image.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/tui/logo"
)

const (
	logoRows   = 5
	brandLeft  = 4
	tickPeriod = 80 * time.Millisecond
	listSize   = 40
)

type tickMsg time.Time

type theme struct {
	name       string
	background [3]uint8
	text       lipgloss.Color
	muted      lipgloss.Color
	accent     lipgloss.Color
	buttonText lipgloss.Color
}

var themes = []theme{
	{
		name:       "深色",
		background: [3]uint8{0x28, 0x3e, 0x2c},
		text:       lipgloss.Color("#dee0e3"),
		muted:      lipgloss.Color("#afc9b5"),
		accent:     lipgloss.Color("#b6f0cb"),
		buttonText: lipgloss.Color("#142b1b"),
	},
	{
		name:       "浅色",
		background: [3]uint8{0x88, 0xa2, 0x8c},
		text:       lipgloss.Color("#0c0d0e"),
		muted:      lipgloss.Color("#2d4934"),
		accent:     lipgloss.Color("#164b2b"),
		buttonText: lipgloss.Color("#ffffff"),
	},
}

type placement struct {
	row  int
	col  int
	cols int
	rows int
}

type model struct {
	capability logo.Capability
	painter    *logo.Painter

	width  int
	height int
	theme  int
	ticks  int

	selected int
	items    []string
	mouse    string
	lastKey  string

	placement placement
	closed    bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "logoprobe:", err)
		os.Exit(1)
	}
}

func run() error {
	capability := logo.Probe()
	painter := logo.NewPainter(os.Stdout, capability)
	if capability.Available() {
		painter.SetEnabled(true)
		painter.SetBackground(themes[0].background)
	}
	defer func() {
		_ = painter.Close()
	}()

	program := tea.NewProgram(
		newModel(capability, painter),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithOutput(painter),
	)
	_, err := program.Run()
	return err
}

func newModel(capability logo.Capability, painter *logo.Painter) model {
	items := make([]string, listSize)
	kinds := []string{"图片", "视频", "音频", "m3u8"}
	for index := range items {
		items[index] = fmt.Sprintf("候选 %02d · %s · 可用于观察表格重绘", index+1, kinds[index%len(kinds)])
	}
	model := model{
		capability: capability,
		painter:    painter,
		width:      100,
		height:     32,
		items:      items,
		mouse:      "等待鼠标事件",
	}
	model.layout()
	return model
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tick(), tea.WindowSize())
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		m.layout()
		m.applyPlacement()
		return m, nil

	case tickMsg:
		m.ticks++
		return m, tick()

	case tea.KeyMsg:
		m.lastKey = message.String()
		switch message.String() {
		case "q", "ctrl+c":
			m.close()
			return m, tea.Quit
		case "t", "left", "right":
			m.toggleTheme()
		case "j", "down":
			m.selected = (m.selected + 1) % len(m.items)
		case "k", "up":
			m.selected = (m.selected - 1 + len(m.items)) % len(m.items)
		case "pgdown":
			m.selected = (m.selected + 10) % len(m.items)
		case "pgup":
			m.selected = (m.selected - 10 + len(m.items)) % len(m.items)
		case "home":
			m.selected = 0
		case "end":
			m.selected = len(m.items) - 1
		}
		return m, nil

	case tea.MouseMsg:
		m.handleMouse(message)
		return m, nil
	}
	return m, nil
}

func (m *model) handleMouse(message tea.MouseMsg) {
	m.mouse = fmt.Sprintf("%s @ %d,%d", message.String(), message.X, message.Y)
	switch {
	case message.Button == tea.MouseButtonWheelUp:
		m.selected = (m.selected - 1 + len(m.items)) % len(m.items)
	case message.Button == tea.MouseButtonWheelDown:
		m.selected = (m.selected + 1) % len(m.items)
	case message.Action == tea.MouseActionPress && message.Button == tea.MouseButtonLeft &&
		message.Y < 5 && message.X >= m.width-24:
		m.toggleTheme()
	}
}

func (m *model) toggleTheme() {
	m.theme = (m.theme + 1) % len(themes)
	m.painter.SetBackground(themes[m.theme].background)
}

func (m *model) close() {
	if m.closed {
		return
	}
	m.closed = true
	_ = m.painter.Close()
}

// layout keeps the bitmap placement aligned with the cell canvas. The logo is
// kept at a fixed five-row placement across the acceptance window sizes
// (44x20 and larger); only an extremely narrow terminal clamps it. Keeping the
// rectangle stable avoids confusing Sixel residue with a placement bug.
func (m *model) layout() {
	rows := logoRows
	cols := logoColumns(m.capability, rows)
	if cols < 1 {
		cols = 1
	}
	col := brandLeft
	if m.width > 0 && m.width <= col {
		col = 0
	}
	if m.width > 0 {
		available := m.width - col - 1
		if available < 1 {
			available = 1
		}
		if cols > available {
			cols = available
		}
	}
	m.placement = placement{row: 0, col: col, cols: cols, rows: rows}
}

// applyPlacement is called only after the first size message, so the very
// first renderer writes (alt screen, hide cursor, mouse enable) do not each
// append an image before any frame has been laid out.
func (m *model) applyPlacement() {
	if !m.capability.Available() {
		return
	}
	m.painter.SetPlacement(m.placement.row, m.placement.col, m.placement.cols, m.placement.rows)
}

func logoColumns(capability logo.Capability, rows int) int {
	if !capability.Available() || rows <= 0 {
		return 0
	}
	cols := (rows*capability.CellHeight + capability.CellWidth/2) / capability.CellWidth
	if cols < 1 {
		cols = 1
	}
	return cols
}

func (m model) View() string {
	width := m.width
	if width < 20 {
		width = 20
	}
	height := m.height
	if height < 12 {
		height = 12
	}
	current := themes[m.theme%len(themes)]

	title := lipgloss.NewStyle().Foreground(current.text).Bold(true)
	muted := lipgloss.NewStyle().Foreground(current.muted)
	accent := lipgloss.NewStyle().Foreground(current.accent)
	logoArea := lipgloss.NewStyle().Background(hexColor(current.background))
	button := lipgloss.NewStyle().Background(current.accent).Foreground(current.buttonText).Bold(true).Padding(0, 1)

	lines := make([]string, 0, height)
	lines = append(lines, m.brandLines(title, muted, accent, logoArea)...)
	lines = append(lines, "")

	status := "未探测到图形协议 · 已静默降级（不输出图片控制序列）"
	if m.capability.Available() {
		status = fmt.Sprintf("真图已启用 · %s · 终端回复的单元格 = %d×%d px", m.capability.Protocol, m.capability.CellWidth, m.capability.CellHeight)
	}
	lines = append(lines, "终端探测  "+status)
	lines = append(lines, "探测环境  "+environmentSummary())
	imageInfo := "图片区域  未启用（已回退为块字符占位）"
	if m.capability.Available() {
		imageInfo = fmt.Sprintf("图片区域  %d×%d 单元格（%d×%d px）· 位置 row=%d col=%d",
			m.placement.cols, m.placement.rows,
			m.placement.cols*cellWidth(m.capability), m.placement.rows*cellHeight(m.capability),
			m.placement.row, m.placement.col)
	}
	lines = append(lines, imageInfo)
	lines = append(lines, "")

	bar := progressBar(m.ticks%25, 24)
	lines = append(lines, fmt.Sprintf("重绘压力  帧 #%d · 每 %s 一帧 · %s", m.ticks, tickPeriod, accent.Render(bar)))
	lines = append(lines, "主题  "+button.Render(current.name)+"  t / ← / → 或点击右上角切换")
	lines = append(lines, "")

	visible := height - len(lines) - 5
	if visible < 3 {
		visible = 3
	}
	if visible > 12 {
		visible = 12
	}
	lines = append(lines, "滚动列表  ↑/↓、PgUp/PgDn、Home/End、鼠标滚轮")
	lines = append(lines, listLines(m, visible, current, accent)...)
	lines = append(lines, "")
	lines = append(lines, muted.Render("鼠标  "+m.mouse+" · 最近按键 "+keyLabel(m.lastKey)))
	lines = append(lines, accent.Render("q 退出并清理图片 · 改窗口观察重绘/残影 · 打开覆盖层不在本原型范围"))

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	rendered := make([]string, len(lines))
	for index, line := range lines {
		if line == "" {
			rendered[index] = strings.Repeat(" ", width)
			continue
		}
		rendered[index] = fit(line, width)
	}
	return strings.Join(rendered, "\n")
}

func (m model) brandLines(title, muted, accent, logoArea lipgloss.Style) []string {
	captions := []struct {
		text  string
		style lipgloss.Style
	}{
		{text: "WX-MINI-VIDEO / 媒体嗅探", style: accent},
		{text: "微信小程序媒体下载", style: title},
		{text: "保存小程序里值得留下的图片与音视频", style: muted},
		{text: probeLine(m.capability), style: accent},
		{text: "原型：真图 logo + 重绘压力验证", style: muted},
	}
	if m.capability.Available() {
		captions[3].text = "真图 logo 已就位 · 观察导航/切主题/缩放/退出"
	}

	fallback := []string{
		"  ██╗    ██╗  ",
		"  ██║    ██║  ",
		"  ╚██╗  ██╔╝  ",
		"   ╚████╔╝   ",
		"    ╚═══╝    ",
	}
	if m.placement.rows < len(captions) {
		captions = captions[:m.placement.rows]
	}

	lines := make([]string, 0, len(captions))
	for row, caption := range captions {
		if m.capability.Available() {
			area := strings.Repeat(" ", m.placement.cols)
			prefix := strings.Repeat(" ", m.placement.col) + logoArea.Render(area) + "  "
			lines = append(lines, prefix+caption.style.Render(caption.text))
			continue
		}
		art := ""
		if row < len(fallback) {
			art = fallback[row]
		}
		prefix := strings.Repeat(" ", m.placement.col) + accent.Render(art) + "  "
		lines = append(lines, prefix+caption.style.Render(caption.text))
	}
	return lines
}

func probeLine(capability logo.Capability) string {
	if capability.Available() {
		return fmt.Sprintf("探测：%s · 单元格 %d×%d px", capability.Protocol, capability.CellWidth, capability.CellHeight)
	}
	return "探测：不可用 · 降级为块字符/纯文字"
}

func cellWidth(capability logo.Capability) int {
	if !capability.Available() {
		return 0
	}
	return capability.CellWidth
}

func cellHeight(capability logo.Capability) int {
	if !capability.Available() {
		return 0
	}
	return capability.CellHeight
}

func environmentSummary() string {
	keys := []string{"TERM", "TERM_PROGRAM", "WT_SESSION", "NO_COLOR", "TMUX", "STY"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if value == "" {
			value = "<空>"
		}
		parts = append(parts, key+"="+value)
	}
	if len(parts) == 0 {
		return "（无相关变量）"
	}
	return strings.Join(parts, " ")
}

func listLines(m model, visible int, current theme, accent lipgloss.Style) []string {
	if visible <= 0 {
		return nil
	}
	start := m.selected - visible/2
	if start < 0 {
		start = 0
	}
	if start+visible > len(m.items) {
		start = len(m.items) - visible
	}
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, visible)
	selectedStyle := lipgloss.NewStyle().Background(current.accent).Foreground(current.buttonText)
	for index := start; index < start+visible && index < len(m.items); index++ {
		marker := "  "
		if index == m.selected {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%02d  %s", marker, index+1, m.items[index])
		if index == m.selected {
			line = selectedStyle.Render(line)
		} else {
			line = accent.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func progressBar(filled, width int) string {
	if width <= 0 {
		return ""
	}
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func hexColor(rgb [3]uint8) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2]))
}

func keyLabel(key string) string {
	if key == "" {
		return "（无）"
	}
	return key
}

// fit clamps a styled line to the terminal width. Styling is applied before
// this point; visible width is measured with lipgloss so CJK text does not
// wrap the view and scroll the bitmap off its cell placement.
func fit(line string, width int) string {
	if width <= 0 || lipgloss.Width(line) <= width {
		return line
	}
	// Cut on a visible-width budget, skipping complete CSI/OSC sequences so a
	// truncation never leaves half an escape sequence behind.
	runes := []rune(line)
	visible := 0
	cut := 0
	for cut < len(runes) {
		r := runes[cut]
		if r == 0x1b && cut+1 < len(runes) {
			switch runes[cut+1] {
			case '[':
				cut += 2
				for cut < len(runes) && !(runes[cut] >= 0x40 && runes[cut] <= 0x7e) {
					cut++
				}
				if cut < len(runes) {
					cut++
				}
			case ']':
				cut += 2
				for cut < len(runes) {
					if runes[cut] == 0x07 {
						cut++
						break
					}
					if runes[cut] == 0x1b && cut+1 < len(runes) && runes[cut+1] == '\\' {
						cut += 2
						break
					}
					cut++
				}
			default:
				cut += 2
			}
			continue
		}
		next := visible + lipgloss.Width(string(r))
		if next > width {
			break
		}
		visible = next
		cut++
	}
	return string(runes[:cut]) + "\x1b[0m"
}

func tick() tea.Cmd {
	return tea.Tick(tickPeriod, func(at time.Time) tea.Msg {
		return tickMsg(at)
	})
}
