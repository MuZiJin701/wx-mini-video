//go:build windows

package logo

import (
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// queryTerminal writes the probe query to the Windows console, saves and
// restores both console modes, and polls the input buffer until the deadline.
//
// Console input is read with ReadFile rather than a blocking os.Stdin read:
// WaitForSingleObject tells us when a console input record is available, so a
// timed-out probe can never leave a goroutine blocked on stdin and steal keys
// from Bubble Tea later.
func queryTerminal(query string, timeout time.Duration) ([]byte, error) {
	input := windows.Handle(os.Stdin.Fd())
	output := windows.Handle(os.Stdout.Fd())

	var inputMode, outputMode uint32
	if err := windows.GetConsoleMode(input, &inputMode); err != nil {
		return nil, err
	}
	if err := windows.GetConsoleMode(output, &outputMode); err != nil {
		return nil, err
	}
	defer func() {
		_ = windows.SetConsoleMode(input, inputMode)
		_ = windows.SetConsoleMode(output, outputMode)
	}()

	if err := windows.SetConsoleMode(output, outputMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return nil, err
	}

	// Raw VT input is required for terminal replies. Some older conhost
	// versions reject the flag; fall back to a non-canonical, no-echo mode.
	if err := windows.SetConsoleMode(input, windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
		fallback := inputMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
		if fallbackErr := windows.SetConsoleMode(input, fallback); fallbackErr != nil {
			return nil, err
		}
	}

	var written uint32
	if err := windows.WriteFile(output, []byte(query), &written, nil); err != nil {
		return nil, err
	}
	if int(written) != len(query) {
		return nil, io.ErrShortWrite
	}

	deadline := time.Now().Add(timeout)
	reply := make([]byte, 0, 256)
	buffer := make([]byte, 512)
	for len(reply) < maxProbeReplySize {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		wait := uint32(remaining / time.Millisecond)
		if wait == 0 {
			wait = 1
		}
		if wait > 20 {
			wait = 20
		}
		event, err := windows.WaitForSingleObject(input, wait)
		if err != nil {
			return reply, err
		}
		if event != windows.WAIT_OBJECT_0 {
			continue
		}
		var read uint32
		if err := windows.ReadFile(input, buffer, &read, nil); err != nil {
			return reply, err
		}
		if read == 0 {
			// A non-character input event can signal the handle without
			// yielding bytes; avoid a busy poll for the rest of the deadline.
			time.Sleep(2 * time.Millisecond)
			continue
		}
		reply = append(reply, buffer[:read]...)
	}
	return reply, nil
}
