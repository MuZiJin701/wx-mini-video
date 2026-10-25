package tui

import (
	"bytes"
	"strings"
	"testing"

	"wx_channel/internal/tui/logo"
)

func TestLogoRemainsVisibleInCompactWindows(t *testing.T) {
	for _, size := range []struct{ width, height int }{{110, 40}, {110, 24}, {80, 24}, {60, 20}} {
		m := mediaTestModel(t)
		m.width, m.height = size.width, size.height
		m.logoCap = logo.Capability{Protocol: logo.ProtocolSixel, CellWidth: 10, CellHeight: 20}
		var output bytes.Buffer
		m.painter = logo.NewPainter(&output, m.logoCap)
		_, rows, ok := m.logoSize(m.metricsNow())
		if !ok || (size.height < 32 && rows != 3) {
			t.Fatalf("%dx%d: compact logo missing or too tall: rows=%d available=%v", size.width, size.height, rows, ok)
		}
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d: got %d lines", size.width, size.height, len(lines))
		}
		assertLineWidths(t, m, lines)
		if _, err := m.painter.Write([]byte(view)); err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(output.Bytes(), []byte("\x1bP")) {
			t.Fatalf("%dx%d: logo not drawn", size.width, size.height)
		}
	}
}
