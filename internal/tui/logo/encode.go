package logo

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
)

const (
	// kittyImageID identifies the single logo placement owned by this package.
	// ClearBytes deletes exactly this image.
	kittyImageID  = 7320
	kittyChunkLen = 4096

	// Keep a malformed placement or a very large terminal from allocating an
	// unbounded bitmap. The real brand area is a handful of cells.
	maxCanvasSide   = 4096
	maxCanvasPixels = 4 * 1024 * 1024
)

var (
	// ErrUnavailable is returned when a bitmap protocol was not detected.
	ErrUnavailable = errors.New("logo: terminal graphics unavailable")
	// ErrInvalidSize is returned for a non-positive or unreasonable canvas.
	ErrInvalidSize = errors.New("logo: invalid logo placement")
)

// EncodeLogo renders the embedded brand mark into a cols x rows cell canvas
// and encodes it with this capability's protocol. The canvas is filled with
// bgRGB; the icon is scaled to the largest square that fits and centred.
//
// The returned bytes do not contain an absolute cursor position. DrawBytes
// wraps them with cursor save/restore and positioning.
func (c Capability) EncodeLogo(cols, rows int, bgRGB [3]uint8) ([]byte, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	if cols <= 0 || rows <= 0 {
		return nil, ErrInvalidSize
	}

	width64 := int64(cols) * int64(c.CellWidth)
	height64 := int64(rows) * int64(c.CellHeight)
	if width64 <= 0 || height64 <= 0 ||
		width64 > maxCanvasSide || height64 > maxCanvasSide ||
		width64*height64 > maxCanvasPixels {
		return nil, ErrInvalidSize
	}
	width, height := int(width64), int(height64)

	source, err := decodeLogo()
	if err != nil {
		return nil, err
	}
	canvas := renderLogoCanvas(source, width, height, bgRGB)

	switch c.Protocol {
	case ProtocolSixel:
		return encodeSixel(canvas)
	case ProtocolKitty:
		return encodeKitty(canvas, cols, rows)
	default:
		return nil, ErrUnavailable
	}
}

// DrawBytes wraps an encoded logo in DECSC/DECRC and an absolute cursor
// position. row and col are zero-based terminal cells.
func (c Capability) DrawBytes(row, col, cols, rows int, bgRGB [3]uint8) ([]byte, error) {
	if row < 0 || col < 0 {
		return nil, ErrInvalidSize
	}
	imageBytes, err := c.EncodeLogo(cols, rows, bgRGB)
	if err != nil {
		return nil, err
	}
	return drawSequence(row, col, imageBytes), nil
}

// ClearBytes returns the protocol-specific cleanup sequence. Kitty deletes
// both the image data and its placements; Sixel has no equivalent deletion
// command, so it returns an empty slice.
func (c Capability) ClearBytes() []byte {
	if c.Protocol != ProtocolKitty {
		return nil
	}
	return []byte("\x1b_Ga=d,d=I,i=" + strconv.Itoa(kittyImageID) + ",q=2\x1b\\")
}

func drawSequence(row, col int, encoded []byte) []byte {
	sequence := make([]byte, 0, len(encoded)+32)
	sequence = append(sequence, "\x1b7\x1b["...)
	sequence = strconv.AppendInt(sequence, int64(row+1), 10)
	sequence = append(sequence, ';')
	sequence = strconv.AppendInt(sequence, int64(col+1), 10)
	sequence = append(sequence, 'H')
	sequence = append(sequence, encoded...)
	sequence = append(sequence, "\x1b8"...)
	return sequence
}

// renderLogoCanvas builds an opaque canvas and composites the scaled icon
// onto it. Gaps left by the square icon keep the requested background.
func renderLogoCanvas(source *image.RGBA, width, height int, bgRGB [3]uint8) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	background := color.RGBA{R: bgRGB[0], G: bgRGB[1], B: bgRGB[2], A: 255}
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)

	side := width
	if height < side {
		side = height
	}
	if side <= 0 {
		return canvas
	}
	drawScaledLogo(canvas, source, (width-side)/2, (height-side)/2, side)
	return canvas
}

// drawScaledLogo performs a bilinear resize directly into the destination.
// image.RGBA stores premultiplied alpha, so the four samples can be averaged
// per channel without dark fringes at the transparent rounded corners.
func drawScaledLogo(dst *image.RGBA, source *image.RGBA, left, top, side int) {
	sourceBounds := source.Bounds()
	sourceWidth := sourceBounds.Dx()
	sourceHeight := sourceBounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 || side <= 0 {
		return
	}

	scaleX := float64(sourceWidth) / float64(side)
	scaleY := float64(sourceHeight) / float64(side)
	for y := 0; y < side; y++ {
		sourceY := (float64(y)+0.5)*scaleY - 0.5
		y0 := int(math.Floor(sourceY))
		fy := sourceY - float64(y0)
		y1 := y0 + 1
		if y0 < 0 {
			y0 = 0
		}
		if y1 >= sourceHeight {
			y1 = sourceHeight - 1
		}

		for x := 0; x < side; x++ {
			sourceX := (float64(x)+0.5)*scaleX - 0.5
			x0 := int(math.Floor(sourceX))
			fx := sourceX - float64(x0)
			x1 := x0 + 1
			if x0 < 0 {
				x0 = 0
			}
			if x1 >= sourceWidth {
				x1 = sourceWidth - 1
			}

			topLeft := source.RGBAAt(sourceBounds.Min.X+x0, sourceBounds.Min.Y+y0)
			topRight := source.RGBAAt(sourceBounds.Min.X+x1, sourceBounds.Min.Y+y0)
			bottomLeft := source.RGBAAt(sourceBounds.Min.X+x0, sourceBounds.Min.Y+y1)
			bottomRight := source.RGBAAt(sourceBounds.Min.X+x1, sourceBounds.Min.Y+y1)

			weights := [4]float64{
				(1 - fx) * (1 - fy),
				fx * (1 - fy),
				(1 - fx) * fy,
				fx * fy,
			}
			samples := [4]color.RGBA{topLeft, topRight, bottomLeft, bottomRight}
			var red, green, blue, alpha float64
			for index, sample := range samples {
				weight := weights[index]
				red += float64(sample.R) * weight
				green += float64(sample.G) * weight
				blue += float64(sample.B) * weight
				alpha += float64(sample.A) * weight
			}

			// Premultiplied source over an opaque destination.
			background := dst.RGBAAt(left+x, top+y)
			inverse := 1 - alpha/255
			dst.SetRGBA(left+x, top+y, color.RGBA{
				R: clampByte(red + float64(background.R)*inverse),
				G: clampByte(green + float64(background.G)*inverse),
				B: clampByte(blue + float64(background.B)*inverse),
				A: 255,
			})
		}
	}
}

func clampByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(value + 0.5)
}
