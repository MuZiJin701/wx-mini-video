package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWheelScrollsViewportWithoutChangingSelection(t *testing.T) {
	m := mediaTestModel(t)
	m.width, m.height = 110, 40
	m.candidates = testCandidates(80)
	m.selected = 0
	m.selectedID = m.candidates[0].ID
	_ = m.View()
	row, ok := findHitIndex(m, hitRow, 0)
	if !ok {
		t.Fatal("first candidate not visible")
	}
	wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: row.x + 1, Y: row.y}
	for range 3 {
		next, _ := m.Update(wheel)
		m = next.(model)
	}
	_ = m.View()
	if m.selected != 0 || m.selectedID != m.candidates[0].ID || m.mediaOffset != 9 {
		t.Fatalf("wheel changed selection or lost repeated events: selected=%d offset=%d", m.selected, m.mediaOffset)
	}
	if _, ok := findHitIndex(m, hitRow, 9); !ok {
		t.Fatal("viewport did not scroll to candidate 9")
	}
	if _, ok := findHitIndex(m, hitRow, 0); ok {
		t.Fatal("viewport still follows the offscreen selection")
	}
	// Wheel events over the footer and an appearance dropdown must not scroll media.
	footer, _ := findHit(m, hitActionQuit)
	wheel.X, wheel.Y = footer.x, footer.y
	next, _ := m.Update(wheel)
	m = next.(model)
	if m.mediaOffset != 9 {
		t.Fatal("wheel outside the list scrolled media")
	}
	m.appearanceOpen = true
	wheel.X, wheel.Y = row.x+1, row.y
	next, _ = m.Update(wheel)
	m = next.(model)
	if m.mediaOffset != 9 {
		t.Fatal("wheel behind appearance dropdown scrolled media")
	}
	m.appearanceOpen = false
	m = m.scrollBy(-1000)
	if m.mediaOffset != 0 {
		t.Fatal("scrolling above the start was not clamped")
	}
	m = m.scrollBy(1000)
	_ = m.View()
	if _, ok := findHitIndex(m, hitRow, len(m.candidates)-1); !ok {
		t.Fatal("cannot scroll to the last candidate")
	}
	// Keyboard navigation returns to the selected item; category changes reset scrolling.
	m.moveSelection(1)
	_ = m.View()
	if _, ok := findHitIndex(m, hitRow, 1); !ok || m.selected != 1 {
		t.Fatal("keyboard navigation did not reveal the selected candidate")
	}
	m.mediaScrolled = true
	m.setCategory(categoryAll)
	if m.mediaScrolled {
		t.Fatal("category switch retained the old scroll state")
	}
}
