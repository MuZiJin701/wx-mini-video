package logo

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	"image/png"
	"sync"
)

// logoPNG is embedded from the shared brand asset (reused from the reference
// project). It stays inside the binary and is not copied into release
// directories.
//
//go:embed assets/logo.png
var logoPNG []byte

var (
	logoOnce  sync.Once
	logoImage *image.RGBA
	logoErr   error
)

// decodeLogo decodes the embedded 512x512 RGBA brand mark once. The returned
// image uses image.RGBA's premultiplied alpha representation.
func decodeLogo() (*image.RGBA, error) {
	logoOnce.Do(func() {
		decoded, err := png.Decode(bytes.NewReader(logoPNG))
		if err != nil {
			logoErr = err
			return
		}
		if rgba, ok := decoded.(*image.RGBA); ok {
			logoImage = rgba
			return
		}
		bounds := decoded.Bounds()
		rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.Draw(rgba, rgba.Bounds(), decoded, bounds.Min, draw.Src)
		logoImage = rgba
	})
	return logoImage, logoErr
}

// LogoSize reports the dimensions of the embedded brand asset. It is useful
// for diagnostics and tests; the asset is expected to be 512x512.
func LogoSize() (width, height int, err error) {
	img, err := decodeLogo()
	if err != nil {
		return 0, 0, err
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}
