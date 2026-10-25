package logo

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
)

// Protocol is a terminal image protocol that the logo renderer can speak.
type Protocol int

const (
	// ProtocolNone means no bitmap protocol was positively detected.
	ProtocolNone Protocol = iota
	ProtocolSixel
	ProtocolKitty
)

// String returns the lower-case protocol name used in diagnostics.
func (p Protocol) String() string {
	switch p {
	case ProtocolSixel:
		return "sixel"
	case ProtocolKitty:
		return "kitty"
	default:
		return "none"
	}
}

// Capability is the result of a terminal probe. CellWidth and CellHeight are
// the pixel size of one character cell, as reported by CSI 16t.
type Capability struct {
	Protocol   Protocol
	CellWidth  int
	CellHeight int
}

// Available reports whether a bitmap can be drawn. Both a positive protocol
// reply and a sane pixel cell size are required.
func (c Capability) Available() bool {
	return c.Protocol != ProtocolNone && validCellSize(c.CellWidth) && validCellSize(c.CellHeight)
}

func validCellSize(size int) bool {
	return size >= 1 && size <= maxCellPixels
}

const (
	maxCellPixels     = 256
	maxProbeReplySize = 8192
	probeTimeout      = 300 * time.Millisecond
)

// probeQuery asks kitty for an acknowledgement, asks the terminal for its
// pixel cell size (CSI 16t), and asks for DA1 so Sixel support can be read from
// the primary device attributes.
const probeQuery = "\x1b_Gi=7319,a=q,t=d,f=24,s=1,v=1;AAAA\x1b\\" + "\x1b[16t" + "\x1b[c"

var (
	cellReplyPattern  = regexp.MustCompile(`\x1b\[6;([0-9]{1,4});([0-9]{1,4})t`)
	da1ReplyPattern   = regexp.MustCompile(`\x1b\[\?([0-9;]*)c`)
	kittyReplyPattern = regexp.MustCompile(`\x1b_Gi=7319;([^\x1b]*)\x1b\\`)
)

// Probe detects terminal bitmap support. It must run before Bubble Tea takes
// ownership of stdin. It never guesses from the operating system name: a
// missing or malformed positive reply always yields the zero Capability.
//
// The query reads until its 300ms deadline rather than returning as soon as
// the first reply arrives, so reply bytes cannot leak into Bubble Tea's input
// reader as fake key presses. The cost is up to 300ms of extra startup time.
func Probe() Capability {
	if unsupportedEnvironment(os.LookupEnv) || !stdioIsTerminal() {
		return Capability{}
	}
	reply, err := queryTerminal(probeQuery, probeTimeout)
	if err != nil {
		return Capability{}
	}
	capability, pending := parseProbeReply(reply)
	// Reply bytes are intentionally consumed here. They must not be handed to
	// Bubble Tea's input reader, which would interpret them as key presses.
	_ = pending
	return capability
}

// unsupportedEnvironment applies the conservative negative checks. It is a
// pure function so the checks can be tested without a terminal.
func unsupportedEnvironment(lookup func(string) (string, bool)) bool {
	if _, ok := lookup("NO_COLOR"); ok {
		return true
	}
	term, _ := lookup("TERM")
	if term == "dumb" || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return true
	}
	if program, _ := lookup("TERM_PROGRAM"); program == "Apple_Terminal" {
		return true
	}
	if _, ok := lookup("TMUX"); ok {
		return true
	}
	if _, ok := lookup("STY"); ok {
		return true
	}
	return false
}

func stdioIsTerminal() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// parseProbeReply extracts the protocol and pixel cell size from the bytes
// read during the probe. The second result contains bytes that were not part
// of a recognised reply, so a caller can restore or discard them explicitly.
func parseProbeReply(reply []byte) (Capability, []byte) {
	cell := cellReplyPattern.FindSubmatch(reply)
	if cell == nil {
		return Capability{}, reply
	}
	height, errHeight := strconv.Atoi(string(cell[1]))
	width, errWidth := strconv.Atoi(string(cell[2]))
	if errHeight != nil || errWidth != nil || !validCellSize(width) || !validCellSize(height) {
		return Capability{}, reply
	}

	protocol := ProtocolNone
	if match := kittyReplyPattern.FindSubmatch(reply); match != nil {
		if strings.HasPrefix(strings.TrimSpace(string(match[1])), "OK") {
			protocol = ProtocolKitty
		}
	}
	if protocol == ProtocolNone {
		if match := da1ReplyPattern.FindSubmatch(reply); match != nil {
			for _, parameter := range strings.Split(string(match[1]), ";") {
				if parameter == "4" {
					protocol = ProtocolSixel
					break
				}
			}
		}
	}
	if protocol == ProtocolNone {
		return Capability{}, stripProbeReplies(reply)
	}
	return Capability{
		Protocol:   protocol,
		CellWidth:  width,
		CellHeight: height,
	}, stripProbeReplies(reply)
}

// stripProbeReplies removes the sequences we recognise so callers can decide
// what to do with everything else (normally just discard it).
func stripProbeReplies(reply []byte) []byte {
	pending := cellReplyPattern.ReplaceAll(reply, nil)
	pending = da1ReplyPattern.ReplaceAll(pending, nil)
	pending = kittyReplyPattern.ReplaceAll(pending, nil)
	return pending
}
