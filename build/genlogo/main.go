// Command genlogo regenerates the embedded 512x512 brand icon using only the
// Go standard library.
//
//	go run ./build/genlogo
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	finalSize = 512
	superSize = finalSize * 4
	assetPath = "internal/tui/logo/assets/logo.png"
)

var (
	gradientTop    = color.RGBA{R: 0x1f, G: 0x6b, B: 0x40, A: 0xff}
	gradientBottom = color.RGBA{R: 0x77, G: 0xd5, B: 0x9b, A: 0xff}
	white          = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

func main() {
	highResolution := render(superSize)
	icon := downsample(highResolution, superSize/finalSize)

	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		fatal(err)
	}
	file, err := os.Create(assetPath)
	if err != nil {
		fatal(err)
	}
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(file, icon); err != nil {
		_ = file.Close()
		fatal(err)
	}
	if err := file.Close(); err != nil {
		fatal(err)
	}
	info, err := os.Stat(assetPath)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%dx%d, %d bytes)\n", assetPath, finalSize, finalSize, info.Size())
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genlogo:", err)
	os.Exit(1)
}

func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / finalSize
	unit := func(value float64) float64 { return value * scale }

	// Rounded gradient tile.
	fillRoundedRect(img, unit(0), unit(0), unit(512), unit(512), unit(112), func(_, y float64) color.RGBA {
		position := math.Max(0, math.Min(1, y/float64(size)))
		return color.RGBA{
			R: lerp(gradientTop.R, gradientBottom.R, position),
			G: lerp(gradientTop.G, gradientBottom.G, position),
			B: lerp(gradientTop.B, gradientBottom.B, position),
			A: 0xff,
		}
	})

	// White play mark.
	fillTriangle(img,
		unit(184), unit(86),
		unit(184), unit(252),
		unit(342), unit(169),
		white,
	)

	// Download arrow stem and head.
	fillRoundedRect(img, unit(241), unit(258), unit(281), unit(344), unit(3), func(_, _ float64) color.RGBA {
		return white
	})
	fillTriangle(img,
		unit(200), unit(306),
		unit(322), unit(306),
		unit(261), unit(390),
		white,
	)

	// Download tray: two posts and a rounded base, matching the reference
	// mark's open-top "inbox" silhouette.
	fillRoundedRect(img, unit(116), unit(362), unit(142), unit(446), unit(10), func(_, _ float64) color.RGBA {
		return white
	})
	fillRoundedRect(img, unit(370), unit(362), unit(396), unit(446), unit(10), func(_, _ float64) color.RGBA {
		return white
	})
	fillRoundedRect(img, unit(116), unit(420), unit(396), unit(446), unit(12), func(_, _ float64) color.RGBA {
		return white
	})

	return img
}

func lerp(from, to uint8, position float64) uint8 {
	value := float64(from) + (float64(to)-float64(from))*position
	return uint8(math.Max(0, math.Min(255, value+0.5)))
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, radius float64, paint func(x, y float64) color.RGBA) {
	for y := int(math.Floor(y0)); y < int(math.Ceil(y1)); y++ {
		for x := int(math.Floor(x0)); x < int(math.Ceil(x1)); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if !insideRoundedRect(px, py, x0, y0, x1, y1, radius) {
				continue
			}
			img.SetRGBA(x, y, paint(px, py))
		}
	}
}

func insideRoundedRect(px, py, x0, y0, x1, y1, radius float64) bool {
	if px < x0 || px > x1 || py < y0 || py > y1 {
		return false
	}
	centerX := math.Max(x0+radius, math.Min(px, x1-radius))
	centerY := math.Max(y0+radius, math.Min(py, y1-radius))
	dx := px - centerX
	dy := py - centerY
	return dx*dx+dy*dy <= radius*radius
}

func fillTriangle(img *image.RGBA, ax, ay, bx, by, cx, cy float64, paint color.RGBA) {
	minX := int(math.Floor(math.Min(ax, math.Min(bx, cx))))
	maxX := int(math.Ceil(math.Max(ax, math.Max(bx, cx))))
	minY := int(math.Floor(math.Min(ay, math.Min(by, cy))))
	maxY := int(math.Ceil(math.Max(ay, math.Max(by, cy))))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if insideTriangle(px, py, ax, ay, bx, by, cx, cy) {
				img.SetRGBA(x, y, paint)
			}
		}
	}
}

func insideTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := edgeSign(px, py, ax, ay, bx, by)
	d2 := edgeSign(px, py, bx, by, cx, cy)
	d3 := edgeSign(px, py, cx, cy, ax, ay)
	hasNegative := d1 < 0 || d2 < 0 || d3 < 0
	hasPositive := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNegative && hasPositive)
}

func edgeSign(px, py, ax, ay, bx, by float64) float64 {
	return (px-bx)*(ay-by) - (ax-bx)*(py-by)
}

func downsample(source *image.RGBA, factor int) *image.RGBA {
	if factor < 1 {
		factor = 1
	}
	width := source.Bounds().Dx() / factor
	height := source.Bounds().Dy() / factor
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	samples := uint64(factor * factor)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var red, green, blue, alpha uint64
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					pixel := source.RGBAAt(x*factor+dx, y*factor+dy)
					red += uint64(pixel.R)
					green += uint64(pixel.G)
					blue += uint64(pixel.B)
					alpha += uint64(pixel.A)
				}
			}
			output.SetRGBA(x, y, color.RGBA{
				R: uint8((red + samples/2) / samples),
				G: uint8((green + samples/2) / samples),
				B: uint8((blue + samples/2) / samples),
				A: uint8((alpha + samples/2) / samples),
			})
		}
	}
	return output
}
