package logo

import (
	"image"
	"sort"
)

const maxSixelColors = 64

type rgbColor struct {
	r uint8
	g uint8
	b uint8
}

type colorSample struct {
	color rgbColor
	count int
}

type colorBox struct {
	samples []colorSample
	count   int
}

// quantize64 builds an adaptive palette of at most 64 colors with a weighted
// median cut, then maps every pixel to the nearest palette entry. It is a
// deliberately small replacement for a general image-quantization package.
func quantize64(img *image.RGBA) ([]rgbColor, []uint8) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	histogram := make(map[uint32]int, width*height/4+1)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := img.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
			histogram[colorKey(pixel.R, pixel.G, pixel.B)]++
		}
	}

	samples := make([]colorSample, 0, len(histogram))
	for key, count := range histogram {
		samples = append(samples, colorSample{
			color: rgbColor{
				r: uint8(key >> 16),
				g: uint8(key >> 8),
				b: uint8(key),
			},
			count: count,
		})
	}
	// Deterministic input makes the generated palette reproducible.
	sort.Slice(samples, func(i, j int) bool {
		left, right := samples[i].color, samples[j].color
		if left.r != right.r {
			return left.r < right.r
		}
		if left.g != right.g {
			return left.g < right.g
		}
		return left.b < right.b
	})

	palette := medianCutPalette(samples, maxSixelColors)
	if len(palette) == 0 {
		palette = []rgbColor{{}}
	}

	nearest := make(map[uint32]uint8, len(samples))
	for _, sample := range samples {
		nearest[colorKey(sample.color.r, sample.color.g, sample.color.b)] = uint8(nearestColor(palette, sample.color))
	}

	indexed := make([]uint8, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := img.RGBAAt(bounds.Min.X+x, bounds.Min.Y+y)
			indexed[y*width+x] = nearest[colorKey(pixel.R, pixel.G, pixel.B)]
		}
	}
	return palette, indexed
}

func colorKey(red, green, blue uint8) uint32 {
	return uint32(red)<<16 | uint32(green)<<8 | uint32(blue)
}

func medianCutPalette(samples []colorSample, limit int) []rgbColor {
	if len(samples) == 0 || limit <= 0 {
		return nil
	}
	boxes := []colorBox{newColorBox(samples)}
	for len(boxes) < limit {
		bestIndex := -1
		bestScore := 0
		for index, box := range boxes {
			if len(box.samples) < 2 {
				continue
			}
			_, span := box.longestChannel()
			score := span * box.count
			if score > bestScore {
				bestScore = score
				bestIndex = index
			}
		}
		if bestIndex < 0 {
			break
		}

		left, right, ok := splitColorBox(boxes[bestIndex])
		if !ok {
			break
		}
		boxes[bestIndex] = left
		boxes = append(boxes, right)
	}

	palette := make([]rgbColor, 0, len(boxes))
	for _, box := range boxes {
		palette = append(palette, box.average())
	}
	sort.Slice(palette, func(i, j int) bool {
		if palette[i].r != palette[j].r {
			return palette[i].r < palette[j].r
		}
		if palette[i].g != palette[j].g {
			return palette[i].g < palette[j].g
		}
		return palette[i].b < palette[j].b
	})
	return palette
}

func newColorBox(samples []colorSample) colorBox {
	box := colorBox{samples: samples}
	for _, sample := range samples {
		box.count += sample.count
	}
	return box
}

func (b colorBox) average() rgbColor {
	if b.count == 0 {
		return rgbColor{}
	}
	var red, green, blue uint64
	for _, sample := range b.samples {
		red += uint64(sample.color.r) * uint64(sample.count)
		green += uint64(sample.color.g) * uint64(sample.count)
		blue += uint64(sample.color.b) * uint64(sample.count)
	}
	total := uint64(b.count)
	return rgbColor{
		r: uint8((red + total/2) / total),
		g: uint8((green + total/2) / total),
		b: uint8((blue + total/2) / total),
	}
}

// longestChannel returns the channel index (0=red, 1=green, 2=blue) and the
// range of that channel inside the box.
func (b colorBox) longestChannel() (int, int) {
	if len(b.samples) == 0 {
		return 0, 0
	}
	minRed, maxRed := b.samples[0].color.r, b.samples[0].color.r
	minGreen, maxGreen := b.samples[0].color.g, b.samples[0].color.g
	minBlue, maxBlue := b.samples[0].color.b, b.samples[0].color.b
	for _, sample := range b.samples[1:] {
		if sample.color.r < minRed {
			minRed = sample.color.r
		}
		if sample.color.r > maxRed {
			maxRed = sample.color.r
		}
		if sample.color.g < minGreen {
			minGreen = sample.color.g
		}
		if sample.color.g > maxGreen {
			maxGreen = sample.color.g
		}
		if sample.color.b < minBlue {
			minBlue = sample.color.b
		}
		if sample.color.b > maxBlue {
			maxBlue = sample.color.b
		}
	}
	redSpan := int(maxRed) - int(minRed)
	greenSpan := int(maxGreen) - int(minGreen)
	blueSpan := int(maxBlue) - int(minBlue)
	switch {
	case redSpan >= greenSpan && redSpan >= blueSpan:
		return 0, redSpan
	case greenSpan >= blueSpan:
		return 1, greenSpan
	default:
		return 2, blueSpan
	}
}

func splitColorBox(box colorBox) (colorBox, colorBox, bool) {
	if len(box.samples) < 2 {
		return colorBox{}, colorBox{}, false
	}
	channel, _ := box.longestChannel()
	sort.SliceStable(box.samples, func(i, j int) bool {
		left, right := channelValue(box.samples[i].color, channel), channelValue(box.samples[j].color, channel)
		if left != right {
			return left < right
		}
		leftColor, rightColor := box.samples[i].color, box.samples[j].color
		if leftColor.r != rightColor.r {
			return leftColor.r < rightColor.r
		}
		if leftColor.g != rightColor.g {
			return leftColor.g < rightColor.g
		}
		return leftColor.b < rightColor.b
	})

	half := box.count / 2
	accumulated := 0
	cut := 1
	for index, sample := range box.samples {
		accumulated += sample.count
		if accumulated >= half && index+1 < len(box.samples) {
			cut = index + 1
			break
		}
	}
	if cut <= 0 {
		cut = 1
	}
	if cut >= len(box.samples) {
		cut = len(box.samples) - 1
	}
	left := newColorBox(box.samples[:cut])
	right := newColorBox(box.samples[cut:])
	return left, right, true
}

func channelValue(color rgbColor, channel int) uint8 {
	switch channel {
	case 0:
		return color.r
	case 1:
		return color.g
	default:
		return color.b
	}
}

func nearestColor(palette []rgbColor, target rgbColor) int {
	bestIndex := 0
	bestDistance := int64(1) << 62
	for index, candidate := range palette {
		dr := int64(target.r) - int64(candidate.r)
		dg := int64(target.g) - int64(candidate.g)
		db := int64(target.b) - int64(candidate.b)
		distance := dr*dr + dg*dg + db*db
		if distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	return bestIndex
}
