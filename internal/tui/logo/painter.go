package logo

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// DefaultBackground is the dark panel colour from the design tokens. Callers
// that follow the active theme should call SetBackground.
var DefaultBackground = [3]uint8{0x28, 0x3e, 0x2c}

// Painter is an io.Writer for tea.WithOutput. It forwards every renderer
// write unchanged and then appends the logo image sequence, if a bitmap
// placement is enabled and configured. All state is mutex-protected because
// Bubble Tea's render goroutine and the model goroutine share the Painter.
//
// Painter also exposes Fd, Read and Close so it satisfies the terminal file
// interface that Bubble Tea uses for its output. Without Fd, tea.WithOutput
// would treat the painter as a non-TTY and silently stop sending
// WindowSizeMsg and stop enabling VT processing on Windows.
//
// Painter has no knowledge of overlays: the image is appended after the whole
// frame, so a caller whose overlay covers the logo rectangle must disable
// drawing (or move the placement) while that overlay is visible. Sixel has no
// delete command, so a moved or shrunken Sixel placement relies on the next
// full repaint or on leaving the alternate screen. As a safety net, Write
// disables drawing and deletes a kitty placement when it sees the alt-screen
// exit sequence, so a forgotten Close cannot leave a placement behind.
type Painter struct {
	mu         sync.Mutex
	out        io.Writer
	capability Capability

	enabled      bool
	hasPlacement bool
	row          int
	col          int
	cols         int
	rows         int
	background   [3]uint8

	pendingClear bool
	cacheReady   bool
	cacheKey     paintKey
	cached       []byte
}

type paintKey struct {
	cols       int
	rows       int
	background [3]uint8
	protocol   Protocol
	cellWidth  int
	cellHeight int
}

// NewPainter wraps out. Drawing is disabled until SetEnabled(true); a caller
// should only enable it when capability.Available() is true.
func NewPainter(out io.Writer, capability Capability) *Painter {
	return &Painter{
		out:        out,
		capability: capability,
		background: DefaultBackground,
	}
}

// SetEnabled toggles bitmap output. Disabling a kitty placement deletes it
// immediately; Sixel has no deletion command, so its pixels disappear when
// the caller repaints or leaves the alternate screen.
func (p *Painter) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled == enabled {
		return
	}
	if !enabled && p.capability.Protocol == ProtocolKitty {
		p.writeClearLocked()
	}
	p.enabled = enabled
	p.pendingClear = false
}

// SetPlacement sets the zero-based top-left cell and the cell extent of the
// logo. Non-positive extents or negative coordinates hide it.
func (p *Painter) SetPlacement(row, col, cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if row < 0 || col < 0 || cols <= 0 || rows <= 0 {
		if p.hasPlacement && p.enabled && p.capability.Protocol == ProtocolKitty {
			p.writeClearLocked()
		}
		p.hasPlacement = false
		p.pendingClear = false
		return
	}

	hadPlacement := p.hasPlacement
	changed := !hadPlacement || p.row != row || p.col != col || p.cols != cols || p.rows != rows
	p.row, p.col, p.cols, p.rows = row, col, cols, rows
	p.hasPlacement = true
	if changed && hadPlacement && p.enabled && p.capability.Protocol == ProtocolKitty {
		// A new placement may leave the previous image at the old coordinates.
		p.pendingClear = true
	}
}

// SetBackground updates the colour used for the area around the icon and
// invalidates the cached encoding.
func (p *Painter) SetBackground(background [3]uint8) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.background == background {
		return
	}
	p.background = background
	p.invalidateCacheLocked()
}

// Write forwards a renderer frame and then appends the image sequence. Image
// errors are swallowed: a terminal that stops accepting graphics must not take
// down the text UI. The frame write result is returned unchanged.
func (p *Painter) Write(frame []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.out == nil {
		return len(frame), nil
	}
	if p.pendingClear {
		p.writeClearLocked()
		p.pendingClear = false
	}

	// Bubble Tea writes the alt-screen exit sequence through this writer. If
	// the caller forgot Close, stop drawing here and delete a kitty placement
	// before the alt screen (and its image) goes away.
	if isAltScreenExit(frame) {
		p.writeClearLocked()
		p.enabled = false
		p.hasPlacement = false
		p.pendingClear = false
	}

	written, err := p.out.Write(frame)
	if err != nil {
		return written, err
	}
	if written != len(frame) {
		return written, io.ErrShortWrite
	}
	if !p.canDrawLocked() {
		return written, nil
	}

	logoBytes := p.encodedLocked()
	if len(logoBytes) == 0 {
		return written, nil
	}
	_, _ = p.out.Write(drawSequence(p.row, p.col, logoBytes))
	return written, nil
}

// Close removes a kitty placement and disables drawing. It is idempotent and
// is safe to call from the model before tea.Quit and again from main after the
// program returns.
func (p *Painter) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.enabled && p.capability.Protocol == ProtocolKitty {
		p.writeClearLocked()
	}
	p.enabled = false
	p.hasPlacement = false
	p.pendingClear = false
	p.invalidateCacheLocked()
	return nil
}

// Fd lets Bubble Tea treat the painter's underlying output as a terminal. It
// returns an invalid descriptor when the wrapped writer is not terminal-like.
func (p *Painter) Fd() uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	if file, ok := p.out.(interface{ Fd() uintptr }); ok {
		return file.Fd()
	}
	return ^uintptr(0)
}

// Read exists only to satisfy the term.File interface. Bubble Tea never reads
// from its output; a wrapped reader is forwarded for completeness.
func (p *Painter) Read(buffer []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if reader, ok := p.out.(io.Reader); ok {
		return reader.Read(buffer)
	}
	return 0, errors.New("logo: painter output is not readable")
}

func (p *Painter) canDrawLocked() bool {
	return p.enabled && p.hasPlacement && p.capability.Available()
}

func (p *Painter) encodedLocked() []byte {
	key := paintKey{
		cols:       p.cols,
		rows:       p.rows,
		background: p.background,
		protocol:   p.capability.Protocol,
		cellWidth:  p.capability.CellWidth,
		cellHeight: p.capability.CellHeight,
	}
	if p.cacheReady && p.cacheKey == key {
		return p.cached
	}

	p.cacheReady = true
	p.cacheKey = key
	p.cached = nil
	encoded, err := p.capability.EncodeLogo(p.cols, p.rows, p.background)
	if err != nil {
		return nil
	}
	p.cached = encoded
	return p.cached
}

func (p *Painter) invalidateCacheLocked() {
	p.cacheReady = false
	p.cacheKey = paintKey{}
	p.cached = nil
}

func (p *Painter) writeClearLocked() {
	if p.out == nil {
		return
	}
	clear := p.capability.ClearBytes()
	if len(clear) == 0 {
		return
	}
	_, _ = p.out.Write(clear)
}

func isAltScreenExit(frame []byte) bool {
	return bytes.Contains(frame, []byte("\x1b[?1049l")) || bytes.Contains(frame, []byte("\x1b[?1047l"))
}

var (
	_ io.Writer          = (*Painter)(nil)
	_ io.ReadWriteCloser = (*Painter)(nil)
	_ interface {
		io.ReadWriteCloser
		Fd() uintptr
	} = (*Painter)(nil)
)
