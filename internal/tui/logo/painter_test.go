package logo

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestPainterAppendsImageAfterFrame(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20})
	painter.SetEnabled(true)
	painter.SetPlacement(1, 2, 3, 2)

	written, err := painter.Write([]byte("frame"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != len("frame") {
		t.Fatalf("Write() = %d, want %d", written, len("frame"))
	}
	got := output.String()
	if !strings.HasPrefix(got, "frame\x1b7\x1b[2;3H") {
		t.Fatalf("output does not append drawing after the frame: %q", got[:min(len(got), 32)])
	}
	if !strings.HasSuffix(got, "\x1b8") {
		t.Fatal("output does not restore the cursor after drawing")
	}
	if !strings.Contains(got, "\x1b_Ga=T,f=100") {
		t.Fatal("output is missing the kitty image")
	}
}

func TestPainterDisabledWritesFrameOnly(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20})
	painter.SetPlacement(0, 0, 4, 2)
	if _, err := painter.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := output.String(); got != "frame" {
		t.Fatalf("disabled painter wrote %q, want only the frame", got)
	}
}

func TestPainterUnavailableWritesFrameOnly(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 4, 2)
	if _, err := painter.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := output.String(); got != "frame" {
		t.Fatalf("unavailable painter wrote %q, want only the frame", got)
	}
}

func TestPainterCloseClearsKittyPlacement(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 4, 2)
	if _, err := painter.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	output.Reset()

	if err := painter.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := output.String(); !strings.HasPrefix(got, "\x1b_Ga=d,d=I,i=7320") {
		t.Fatalf("Close() wrote %q, want a kitty delete", got)
	}

	output.Reset()
	if err := painter.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("second Close() wrote %q, want nothing", output.String())
	}
}

func TestPainterPlacementChangeClearsBeforeFrame(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 4, 2)
	if _, err := painter.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	output.Reset()

	painter.SetPlacement(2, 3, 4, 2)
	if _, err := painter.Write([]byte("next")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got := output.String()
	if !strings.HasPrefix(got, "\x1b_Ga=d,d=I,i=7320") {
		t.Fatalf("placement change did not clear the old image: %q", got[:min(len(got), 32)])
	}
	if !strings.Contains(got, "next\x1b7\x1b[3;4H") {
		t.Fatal("placement change did not draw the new image after the frame")
	}
}

func TestPainterSetBackgroundInvalidatesCache(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolSixel, CellWidth: 8, CellHeight: 16})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 2, 2)
	if _, err := painter.Write([]byte("same")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	first := output.String()

	output.Reset()
	painter.SetBackground([3]uint8{0xff, 0xff, 0xff})
	if _, err := painter.Write([]byte("same")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	second := output.String()
	if !strings.Contains(first, "\x1bP0;1q") || !strings.Contains(second, "\x1bP0;1q") {
		t.Fatal("sixel output missing in one of the frames")
	}
	if first == second {
		t.Fatal("background change did not produce a new encoding")
	}
}

func TestPainterAltScreenExitStopsDrawing(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 4, 2)
	if _, err := painter.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	output.Reset()

	if _, err := painter.Write([]byte("\x1b[?1049l")); err != nil {
		t.Fatalf("exit Write() error = %v", err)
	}
	got := output.String()
	if !strings.HasPrefix(got, "\x1b_Ga=d,d=I,i=7320") {
		t.Fatalf("exit did not delete the kitty image first: %q", got)
	}
	if strings.Contains(got, "\x1b_Ga=T") {
		t.Fatal("exit frame must not redraw the image")
	}

	output.Reset()
	if _, err := painter.Write([]byte("after")); err != nil {
		t.Fatalf("Write() after exit error = %v", err)
	}
	if got := output.String(); got != "after" {
		t.Fatalf("drawing continued after alt-screen exit: %q", got)
	}
}

func TestPainterConcurrentAccess(t *testing.T) {
	var output bytes.Buffer
	painter := NewPainter(&output, Capability{Protocol: ProtocolKitty, CellWidth: 8, CellHeight: 16})
	painter.SetEnabled(true)
	painter.SetPlacement(0, 0, 4, 2)

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for iteration := 0; iteration < 10; iteration++ {
				_, _ = painter.Write([]byte("frame"))
				if worker == 0 {
					painter.SetBackground([3]uint8{uint8(iteration), 2, 3})
				}
				if worker == 1 {
					painter.SetPlacement(iteration%2, 0, 4, 2)
				}
			}
		}(worker)
	}
	wait.Wait()
	if err := painter.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
