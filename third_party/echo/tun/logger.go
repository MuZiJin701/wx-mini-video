package tun

import (
	"fmt"
	"io"
	"os"
)

var logWriter io.Writer = os.Stderr

func SetLogEnabled(enabled bool) {
	if enabled {
		logWriter = os.Stderr
	} else {
		logWriter = io.Discard
	}
}

type stdLogger struct{}

func (l stdLogger) Trace(args ...any) {
	fmt.Fprint(logWriter, "[trace] ")
	fmt.Fprintln(logWriter, args...)
}
func (l stdLogger) Debug(args ...any) {
	fmt.Fprint(logWriter, "[debug] ")
	fmt.Fprintln(logWriter, args...)
}
func (l stdLogger) Info(args ...any) {
	fmt.Fprint(logWriter, "[info] ")
	fmt.Fprintln(logWriter, args...)
}
func (l stdLogger) Warn(args ...any) {
	fmt.Fprint(logWriter, "[warn] ")
	fmt.Fprintln(logWriter, args...)
}
func (l stdLogger) Error(args ...any) {
	fmt.Fprint(logWriter, "[error] ")
	fmt.Fprintln(logWriter, args...)
}
func (l stdLogger) Fatal(args ...any) {
	fmt.Fprint(logWriter, "[fatal] ")
	fmt.Fprintln(logWriter, args...)
	os.Exit(1)
}
func (l stdLogger) Panic(args ...any) { msg := fmt.Sprintln(args...); panic(msg) }
