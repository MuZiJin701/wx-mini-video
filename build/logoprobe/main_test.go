package main

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wx_channel/internal/tui/logo"
)

func TestViewHeadless(t *testing.T) {
	capability := logo.Capability{Protocol: logo.ProtocolSixel, CellWidth: 10, CellHeight: 20}
	var output bytes.Buffer
	painter := logo.NewPainter(&output, capability)
	model := newModel(capability, painter)
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 32}, {Width: 44, Height: 20}, {Width: 20, Height: 12}} {
		model.width = size.Width
		model.height = size.Height
		model.layout()
		view := model.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size.Height {
			t.Fatalf("%dx%d: got %d lines", size.Width, size.Height, len(lines))
		}
		for index, line := range lines {
			if got := lipgloss.Width(line); got > size.Width {
				t.Fatalf("%dx%d line %d width = %d", size.Width, size.Height, index, got)
			}
		}
	}
}
