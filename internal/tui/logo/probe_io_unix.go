//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package logo

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// queryTerminal puts stdin into non-canonical mode (echo off), writes the
// query, and polls with select until the deadline. The original termios is
// always restored.
func queryTerminal(query string, timeout time.Duration) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	original, err := tcget(fd)
	if err != nil {
		return nil, err
	}

	raw := *original
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Iflag &^= unix.IXON | unix.ICRNL
	// select() gates each read; VMIN=1 keeps the read from returning a
	// zero-length result between the readiness check and the read itself.
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := tcset(fd, &raw); err != nil {
		return nil, err
	}
	defer func() {
		_ = tcset(fd, original)
	}()

	if _, err := os.Stdout.WriteString(query); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	reply := make([]byte, 0, 256)
	buffer := make([]byte, 512)
	for len(reply) < maxProbeReplySize {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		var readSet unix.FdSet
		readSet.Set(fd)
		wait := unix.NsecToTimeval(remaining.Nanoseconds())
		ready, err := unix.Select(fd+1, &readSet, nil, nil, &wait)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return reply, err
		}
		if ready == 0 {
			break
		}

		n, err := unix.Read(fd, buffer)
		if err != nil {
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			return reply, err
		}
		if n == 0 {
			break
		}
		reply = append(reply, buffer[:n]...)
	}
	return reply, nil
}
