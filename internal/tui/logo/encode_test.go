package logo

import (
	"bytes"
	"encoding/base64"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestCapabilityAvailable(t *testing.T) {
	tests := []struct {
		name string
		cap  Capability
		want bool
	}{
		{name: "none", cap: Capability{}, want: false},
		{name: "sixel", cap: Capability{Protocol: ProtocolSixel, CellWidth: 10, CellHeight: 20}, want: true},
		{name: "kitty", cap: Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20}, want: true},
		{name: "zero cell", cap: Capability{Protocol: ProtocolSixel}, want: false},
		{name: "oversized cell", cap: Capability{Protocol: ProtocolSixel, CellWidth: 300, CellHeight: 20}, want: false},
		{name: "protocol none with cells", cap: Capability{CellWidth: 10, CellHeight: 20}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.cap.Available(); got != test.want {
				t.Fatalf("Available() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEncodeLogoRejectsUnavailableAndInvalidSizes(t *testing.T) {
	if _, err := (Capability{}).EncodeLogo(4, 3, [3]uint8{}); err != ErrUnavailable {
		t.Fatalf("unavailable EncodeLogo error = %v, want %v", err, ErrUnavailable)
	}
	sixel := Capability{Protocol: ProtocolSixel, CellWidth: 10, CellHeight: 20}
	if _, err := sixel.EncodeLogo(0, 3, [3]uint8{}); err != ErrInvalidSize {
		t.Fatalf("zero columns error = %v, want %v", err, ErrInvalidSize)
	}
	if _, err := sixel.EncodeLogo(4, -1, [3]uint8{}); err != ErrInvalidSize {
		t.Fatalf("negative rows error = %v, want %v", err, ErrInvalidSize)
	}
	if _, err := sixel.EncodeLogo(maxCanvasSide, 1, [3]uint8{}); err != ErrInvalidSize {
		t.Fatalf("oversized canvas error = %v, want %v", err, ErrInvalidSize)
	}
}

func TestEncodeSixel(t *testing.T) {
	capability := Capability{Protocol: ProtocolSixel, CellWidth: 8, CellHeight: 16}
	sequence, err := capability.EncodeLogo(4, 3, [3]uint8{0x28, 0x3e, 0x2c})
	if err != nil {
		t.Fatalf("EncodeLogo() error = %v", err)
	}
	if !bytes.HasPrefix(sequence, []byte("\x1bP0;1q\"1;1;32;48")) {
		t.Fatalf("sixel prefix = %q", sequence[:min(len(sequence), 24)])
	}
	if !bytes.HasSuffix(sequence, []byte("\x1b\\")) {
		t.Fatal("sixel stream is not terminated with ST")
	}
	if bytes.Contains(sequence, []byte("\x1b[")) {
		t.Fatal("sixel stream must not contain an absolute cursor position")
	}
	if !bytes.Contains(sequence, []byte("#0;2;")) {
		t.Fatal("sixel stream is missing palette definitions")
	}
}

func TestSixelRunLengthCompression(t *testing.T) {
	got := string(appendSixelRunLength(nil, []byte("????AAAAA?")))
	if got != "!4?!5A?" {
		t.Fatalf("appendSixelRunLength() = %q, want %q", got, "!4?!5A?")
	}
	if got := string(appendSixelRunLength(nil, []byte("???"))); got != "???" {
		t.Fatalf("short run should not be compressed, got %q", got)
	}
}

func TestEncodeKittyChunks(t *testing.T) {
	payload := strings.Repeat("A", 9000)
	sequence := encodeKittyPayload(payload, 4, 3)
	parts := bytes.Split(sequence, []byte("\x1b\\"))
	if len(parts) != 4 || len(parts[3]) != 0 {
		t.Fatalf("expected 3 chunks plus trailing ST, got %d parts", len(parts))
	}

	first := parts[0]
	if !bytes.HasPrefix(first, []byte("\x1b_Ga=T,f=100,t=d,q=2,i=7320,c=4,r=3,m=1;")) {
		t.Fatalf("first chunk header = %q", first)
	}
	if len(first)-bytes.IndexByte(first, ';')-1 != kittyChunkLen {
		t.Fatalf("first chunk payload length = %d, want %d", len(first)-bytes.IndexByte(first, ';')-1, kittyChunkLen)
	}
	if !bytes.HasPrefix(parts[1], []byte("\x1b_Gm=1;")) {
		t.Fatalf("second chunk header = %q", parts[1])
	}
	if bytes.HasPrefix(parts[1], []byte("\x1b_G,")) {
		t.Fatal("later chunks must not start with a comma")
	}
	if !bytes.HasPrefix(parts[2], []byte("\x1b_Gm=0;")) {
		t.Fatalf("last chunk header = %q", parts[2])
	}

	var joined strings.Builder
	for _, part := range parts[:3] {
		joined.Write(part[bytes.IndexByte(part, ';')+1:])
	}
	if joined.String() != payload {
		t.Fatal("chunk payloads do not reassemble to the original payload")
	}
}

func TestEncodeKittyProducesPNG(t *testing.T) {
	capability := Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20}
	sequence, err := capability.EncodeLogo(32, 16, [3]uint8{0x28, 0x3e, 0x2c})
	if err != nil {
		t.Fatalf("EncodeLogo() error = %v", err)
	}
	if !bytes.HasPrefix(sequence, []byte("\x1b_Ga=T,f=100,t=d,q=2,i=7320,c=32,r=16,m=")) {
		t.Fatalf("kitty prefix = %q", sequence[:min(len(sequence), 64)])
	}
	if !bytes.HasSuffix(sequence, []byte("\x1b\\")) {
		t.Fatal("kitty stream is not terminated with ST")
	}

	parts := bytes.Split(sequence, []byte("\x1b\\"))
	var payload strings.Builder
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if !bytes.HasPrefix(part, []byte("\x1b_G")) {
			t.Fatalf("chunk does not start with APC: %q", part[:min(len(part), 16)])
		}
		semicolon := bytes.IndexByte(part, ';')
		if semicolon < 0 {
			t.Fatal("chunk is missing the payload separator")
		}
		payload.Write(part[semicolon+1:])
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	image, err := png.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("payload is not PNG: %v", err)
	}
	if got := image.Bounds().Dx(); got != 320 {
		t.Fatalf("PNG width = %d, want 320", got)
	}
	if got := image.Bounds().Dy(); got != 320 {
		t.Fatalf("PNG height = %d, want 320", got)
	}
}

func TestDrawBytes(t *testing.T) {
	capability := Capability{Protocol: ProtocolSixel, CellWidth: 8, CellHeight: 16}
	sequence, err := capability.DrawBytes(1, 2, 2, 2, [3]uint8{})
	if err != nil {
		t.Fatalf("DrawBytes() error = %v", err)
	}
	if !bytes.HasPrefix(sequence, []byte("\x1b7\x1b[2;3H")) {
		t.Fatalf("draw prefix = %q", sequence[:min(len(sequence), 16)])
	}
	if !bytes.HasSuffix(sequence, []byte("\x1b8")) {
		t.Fatal("draw sequence must restore the cursor")
	}
	if !bytes.Contains(sequence, []byte("\x1bP0;1q")) {
		t.Fatal("draw sequence is missing the sixel payload")
	}
	if _, err := capability.DrawBytes(-1, 0, 2, 2, [3]uint8{}); err != ErrInvalidSize {
		t.Fatalf("negative row error = %v, want %v", err, ErrInvalidSize)
	}
}

func TestClearBytes(t *testing.T) {
	kitty := Capability{Protocol: ProtocolKitty, CellWidth: 10, CellHeight: 20}
	clear := kitty.ClearBytes()
	if !bytes.HasPrefix(clear, []byte("\x1b_Ga=d,d=I,i=7320")) {
		t.Fatalf("kitty clear = %q", clear)
	}
	if !bytes.HasSuffix(clear, []byte("\x1b\\")) {
		t.Fatal("kitty clear is not terminated with ST")
	}
	if got := (Capability{Protocol: ProtocolSixel, CellWidth: 10, CellHeight: 20}).ClearBytes(); len(got) != 0 {
		t.Fatalf("sixel clear = %q, want empty", got)
	}
}

func TestRenderCanvasBackgroundAndCentering(t *testing.T) {
	source, err := decodeLogo()
	if err != nil {
		t.Fatalf("decodeLogo() error = %v", err)
	}
	background := [3]uint8{0x11, 0x22, 0x33}
	canvas := renderLogoCanvas(source, 100, 50, background)
	if got := canvas.RGBAAt(0, 0); got != (color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}) {
		t.Fatalf("corner pixel = %#v, want background", got)
	}
	if got := canvas.RGBAAt(49, 24); got == (color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}) {
		t.Fatal("centre pixel should contain the icon, not background")
	}
	if canvas.Bounds().Dx() != 100 || canvas.Bounds().Dy() != 50 {
		t.Fatalf("canvas size = %v", canvas.Bounds())
	}
}

func TestLogoSize(t *testing.T) {
	width, height, err := LogoSize()
	if err != nil {
		t.Fatalf("LogoSize() error = %v", err)
	}
	if width != 512 || height != 512 {
		t.Fatalf("LogoSize() = %dx%d, want 512x512", width, height)
	}
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
