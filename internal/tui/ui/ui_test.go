package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAppearanceRoundTripAndFallback(t *testing.T) {
	dir := t.TempDir()
	if got := LoadAppearance(dir); got != AppearanceSystem {
		t.Fatalf("missing ui.json should load system, got %q", got)
	}
	for _, mode := range []AppearanceMode{AppearanceSystem, AppearanceLight, AppearanceDark} {
		if err := SaveAppearance(dir, mode); err != nil {
			t.Fatalf("save %q: %v", mode, err)
		}
		if got := LoadAppearance(dir); got != mode {
			t.Fatalf("load after save %q = %q", mode, got)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, AppearanceFileName), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadAppearance(dir); got != AppearanceSystem {
		t.Fatalf("invalid ui.json should fall back to system, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, AppearanceFileName), []byte(`{"appearance":"rainbow"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadAppearance(dir); got != AppearanceSystem {
		t.Fatalf("unknown appearance should fall back to system, got %q", got)
	}
}

func TestAppearanceFileIsJSON(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAppearance(dir, AppearanceDark); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, AppearanceFileName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("ui.json is not valid JSON: %v", err)
	}
	if decoded["appearance"] != "dark" {
		t.Fatalf("unexpected ui.json content: %s", data)
	}
}

func TestNextAppearanceCycles(t *testing.T) {
	modes := []AppearanceMode{AppearanceSystem, AppearanceLight, AppearanceDark}
	for i, mode := range modes {
		if got := NextAppearance(mode); got != modes[(i+1)%len(modes)] {
			t.Fatalf("NextAppearance(%q) = %q", mode, got)
		}
	}
	if !AppearanceSystem.Valid() || !AppearanceLight.Valid() || !AppearanceDark.Valid() {
		t.Fatal("valid modes rejected")
	}
	if AppearanceMode("nope").Valid() {
		t.Fatal("invalid mode accepted")
	}
}

func TestComputeMetricsBreakpoints(t *testing.T) {
	normal := ComputeMetrics(110, 40)
	if normal.Compact || normal.Narrow || normal.Short {
		t.Fatalf("110x40 should be the normal layout: %#v", normal)
	}
	if normal.ContentLeft != ContentMarginX+ContentPadX {
		t.Fatalf("content left = %d", normal.ContentLeft)
	}
	if normal.ContentWidth != 110-2*(ContentMarginX+ContentPadX) {
		t.Fatalf("content width = %d", normal.ContentWidth)
	}
	compact := ComputeMetrics(70, 40)
	if !compact.Compact || compact.Narrow {
		t.Fatalf("70x40 should be compact: %#v", compact)
	}
	narrow := ComputeMetrics(50, 40)
	if !narrow.Narrow {
		t.Fatalf("50x40 should be narrow: %#v", narrow)
	}
	short := ComputeMetrics(110, 30)
	if !short.Short {
		t.Fatalf("110x30 should be short: %#v", short)
	}
	tiny := ComputeMetrics(10, 10)
	if tiny.ContentWidth < MinContentWidth {
		t.Fatalf("content width should clamp, got %d", tiny.ContentWidth)
	}
}

func TestFitAndTruncateUseDisplayWidth(t *testing.T) {
	if got := Width("图片abc"); got != 7 {
		t.Fatalf("CJK width = %d", got)
	}
	if got := Fit("图片abc", 4); Width(got) != 4 {
		t.Fatalf("Fit produced width %d: %q", Width(got), got)
	}
	if got := Truncate("图片图片图片", 5); Width(got) > 5 {
		t.Fatalf("Truncate overflowed: %q", got)
	}
	if got := Fit("", 6); got != "      " {
		t.Fatalf("Fit empty = %q", got)
	}
	if got := PadRight("a", 3); got != "a  " {
		t.Fatalf("PadRight = %q", got)
	}
}

func TestGradientLineFillsWidth(t *testing.T) {
	p := PaletteFor(true)
	line := GradientLine(10, p, true)
	if line == "" {
		t.Fatal("empty gradient")
	}
	if got := Width(line); got != 10 {
		t.Fatalf("gradient display width = %d", got)
	}
	for _, dark := range []bool{true, false} {
		palette := PaletteFor(dark)
		if palette.Background == "" || palette.Primary == "" || palette.Error == "" {
			t.Fatalf("palette missing tokens (dark=%v)", dark)
		}
	}
}

func TestBrandMarkWidth(t *testing.T) {
	lines := BrandMark(PaletteFor(false))
	if len(lines) != 3 {
		t.Fatalf("brand mark lines = %d", len(lines))
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got != BrandMarkWidth {
			t.Fatalf("brand mark line width = %d, want %d: %q", got, BrandMarkWidth, line)
		}
	}
}
