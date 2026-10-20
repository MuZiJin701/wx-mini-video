//go:build !windows && !darwin

package system

import (
	"fmt"
	"os"
)

func PrepareConsole() (bool, error) { return false, nil }

func ShowStartupError(err error) { fmt.Fprintln(os.Stderr, err) }
