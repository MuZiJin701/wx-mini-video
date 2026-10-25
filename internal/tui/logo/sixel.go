package logo

import (
	"image"
	"strconv"
)

// encodeSixel encodes an opaque image as a Sixel stream: a 64-color adaptive
// palette, per-band color planes, and run-length compression for repeated
// sixel characters. The stream has no absolute cursor movement.
func encodeSixel(img *image.RGBA) ([]byte, error) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidSize
	}
	palette, indexed := quantize64(img)

	out := make([]byte, 0, width*height/4+1024)
	out = append(out, "\x1bP0;1q\"1;1;"...)
	out = strconv.AppendInt(out, int64(width), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(height), 10)

	for index, entry := range palette {
		out = append(out, '#')
		out = strconv.AppendInt(out, int64(index), 10)
		out = append(out, ";2;"...)
		out = strconv.AppendInt(out, int64(sixelPercent(entry.r)), 10)
		out = append(out, ';')
		out = strconv.AppendInt(out, int64(sixelPercent(entry.g)), 10)
		out = append(out, ';')
		out = strconv.AppendInt(out, int64(sixelPercent(entry.b)), 10)
	}

	bandCount := (height + 5) / 6
	for band := 0; band < bandCount; band++ {
		top := band * 6
		rows := height - top
		if rows > 6 {
			rows = 6
		}

		// masks[x][color] has one bit per row in this band. Building it once
		// per column avoids rescanning the band for every palette entry.
		masks := make([][maxSixelColors]uint8, width)
		used := make([]bool, len(palette))
		for x := 0; x < width; x++ {
			for y := 0; y < rows; y++ {
				index := indexed[(top+y)*width+x]
				masks[x][index] |= 1 << uint(y)
				used[index] = true
			}
		}

		row := make([]byte, width)
		for index := range palette {
			if !used[index] {
				continue
			}
			for x := 0; x < width; x++ {
				row[x] = '?' + masks[x][index]
			}
			end := len(row)
			for end > 0 && row[end-1] == '?' {
				end--
			}
			if end == 0 {
				continue
			}

			out = append(out, '#')
			out = strconv.AppendInt(out, int64(index), 10)
			out = appendSixelRunLength(out, row[:end])
			out = append(out, '$')
		}
		if band < bandCount-1 {
			out = append(out, '-')
		}
	}

	out = append(out, "\x1b\\"...)
	return out, nil
}

func sixelPercent(value uint8) int {
	return (int(value)*100 + 127) / 255
}

// appendSixelRunLength compresses runs of four or more identical characters
// with the Sixel !count<char> repeat form.
func appendSixelRunLength(dst []byte, values []byte) []byte {
	for start := 0; start < len(values); {
		end := start + 1
		for end < len(values) && values[end] == values[start] {
			end++
		}
		if end-start >= 4 {
			dst = append(dst, '!')
			dst = strconv.AppendInt(dst, int64(end-start), 10)
			dst = append(dst, values[start])
		} else {
			dst = append(dst, values[start:end]...)
		}
		start = end
	}
	return dst
}
