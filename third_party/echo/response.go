package echo

import (
	"compress/flate"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type zstdReadCloser struct{ dec *zstd.Decoder }

func (z zstdReadCloser) Read(p []byte) (int, error) { return z.dec.Read(p) }
func (z zstdReadCloser) Close() error               { z.dec.Close(); return nil }

// DecompressBody returns a reader that decompresses the response body if needed
func DecompressBody(res *http.Response) (io.ReadCloser, error) {
	encoding := res.Header.Get("Content-Encoding")

	switch encoding {
	case "gzip":
		return gzip.NewReader(res.Body)
	case "deflate":
		return flate.NewReader(res.Body), nil
	case "br":
		return io.NopCloser(brotli.NewReader(res.Body)), nil
	case "zstd", "zstandard", "x-zstd":
		dec, err := zstd.NewReader(res.Body)
		if err != nil {
			return nil, err
		}
		return zstdReadCloser{dec: dec}, nil
	default:
		return res.Body, nil
	}
}

// CopyHeader copies headers from source to destination
func CopyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// DelHopHeaders removes hop-by-hop headers
func DelHopHeaders(header http.Header) {
	hopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te", // canonicalized version of "TE"
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, h := range hopHeaders {
		header.Del(h)
	}
}

// IsWebSocketRequest checks if the request is a WebSocket upgrade
func IsWebSocketRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
