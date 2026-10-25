//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package logo

import "golang.org/x/sys/unix"

func tcget(fd int) (*unix.Termios, error) {
	return unix.IoctlGetTermios(fd, unix.TIOCGETA)
}

func tcset(fd int, state *unix.Termios) error {
	return unix.IoctlSetTermios(fd, unix.TIOCSETA, state)
}
