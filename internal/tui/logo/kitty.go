package logo

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strconv"
)

// encodeKitty encodes the canvas as a PNG and transfers it with the kitty
// graphics protocol. Payloads are split into 4096-byte base64 chunks; the
// command header is present only on the first chunk.
func encodeKitty(img *image.RGBA, cols, rows int) ([]byte, error) {
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, img); err != nil {
		return nil, err
	}
	payload := base64.StdEncoding.EncodeToString(pngBuffer.Bytes())
	return encodeKittyPayload(payload, cols, rows), nil
}

func encodeKittyPayload(payload string, cols, rows int) []byte {
	out := make([]byte, 0, len(payload)+64)
	for offset := 0; offset < len(payload); offset += kittyChunkLen {
		end := offset + kittyChunkLen
		if end > len(payload) {
			end = len(payload)
		}
		more := 0
		if end < len(payload) {
			more = 1
		}

		out = append(out, "\x1b_G"...)
		if offset == 0 {
			out = append(out, "a=T,f=100,t=d,q=2,i="...)
			out = strconv.AppendInt(out, kittyImageID, 10)
			out = append(out, ",c="...)
			out = strconv.AppendInt(out, int64(cols), 10)
			out = append(out, ",r="...)
			out = strconv.AppendInt(out, int64(rows), 10)
			out = append(out, ",m="...)
		} else {
			out = append(out, "m="...)
		}
		out = strconv.AppendInt(out, int64(more), 10)
		out = append(out, ';')
		out = append(out, payload[offset:end]...)
		out = append(out, "\x1b\\"...)
	}
	return out
}
