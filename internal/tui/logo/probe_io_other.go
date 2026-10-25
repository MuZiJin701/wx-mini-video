//go:build !windows && !linux && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package logo

import (
	"errors"
	"time"
)

var errUnsupportedPlatform = errors.New("logo: terminal probing is not implemented on this platform")

func queryTerminal(string, time.Duration) ([]byte, error) {
	return nil, errUnsupportedPlatform
}
