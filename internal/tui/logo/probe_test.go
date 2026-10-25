package logo

import (
	"strings"
	"testing"
)

func TestUnsupportedEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "normal windows terminal", env: map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "Windows_Terminal"}, want: false},
		{name: "no color present even empty", env: map[string]string{"NO_COLOR": ""}, want: true},
		{name: "dumb term", env: map[string]string{"TERM": "dumb"}, want: true},
		{name: "screen", env: map[string]string{"TERM": "screen-256color"}, want: true},
		{name: "tmux", env: map[string]string{"TERM": "tmux-256color"}, want: true},
		{name: "apple terminal", env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, want: true},
		{name: "tmux variable", env: map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"}, want: true},
		{name: "screen variable", env: map[string]string{"STY": "1234.pts-0.host"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			}
			if got := unsupportedEnvironment(lookup); got != test.want {
				t.Fatalf("unsupportedEnvironment() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseProbeReply(t *testing.T) {
	const cell = "\x1b[6;20;10t"
	const sixelDA = "\x1b[?62;4;6;22c"
	const plainDA = "\x1b[?62;1;2c"
	const kittyOK = "\x1b_Gi=7319;OK\x1b\\"
	const kittyError = "\x1b_Gi=7319;ENOENT\x1b\\"

	tests := []struct {
		name         string
		reply        string
		wantProtocol Protocol
		wantWidth    int
		wantHeight   int
		wantPending  string
	}{
		{
			name:         "kitty wins over sixel",
			reply:        kittyOK + cell + sixelDA,
			wantProtocol: ProtocolKitty,
			wantWidth:    10,
			wantHeight:   20,
		},
		{
			name:         "sixel from DA1",
			reply:        cell + sixelDA,
			wantProtocol: ProtocolSixel,
			wantWidth:    10,
			wantHeight:   20,
		},
		{
			name:         "DA1 without parameter 4",
			reply:        cell + plainDA,
			wantProtocol: ProtocolNone,
		},
		{
			name:         "cell without protocol reply",
			reply:        cell,
			wantProtocol: ProtocolNone,
		},
		{
			name:         "kitty without cell size",
			reply:        kittyOK + sixelDA,
			wantProtocol: ProtocolNone,
		},
		{
			name:         "kitty error is not positive",
			reply:        kittyError + cell + plainDA,
			wantProtocol: ProtocolNone,
		},
		{
			name:         "DA1 with malformed dimensions",
			reply:        "\x1b[6;0;10t" + sixelDA,
			wantProtocol: ProtocolNone,
			wantPending:  "\x1b[6;0;10t" + sixelDA,
		},
		{
			name:         "DA1 with oversized cell",
			reply:        "\x1b[6;20;300t" + sixelDA,
			wantProtocol: ProtocolNone,
			wantPending:  "\x1b[6;20;300t" + sixelDA,
		},
		{
			name:         "minimal sixel reply",
			reply:        "\x1b[?4c" + "\x1b[6;1;1t",
			wantProtocol: ProtocolSixel,
			wantWidth:    1,
			wantHeight:   1,
		},
		{
			name:         "unrecognised bytes are returned",
			reply:        cell + sixelDA + "typed",
			wantProtocol: ProtocolSixel,
			wantWidth:    10,
			wantHeight:   20,
			wantPending:  "typed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capability, pending := parseProbeReply([]byte(test.reply))
			if capability.Protocol != test.wantProtocol {
				t.Fatalf("protocol = %v, want %v", capability.Protocol, test.wantProtocol)
			}
			if capability.CellWidth != test.wantWidth || capability.CellHeight != test.wantHeight {
				t.Fatalf("cell = %dx%d, want %dx%d", capability.CellWidth, capability.CellHeight, test.wantWidth, test.wantHeight)
			}
			if test.wantPending != "" && string(pending) != test.wantPending {
				t.Fatalf("pending = %q, want %q", pending, test.wantPending)
			}
			if test.wantProtocol != ProtocolNone && string(pending) != test.wantPending {
				t.Fatalf("pending = %q, want %q", pending, test.wantPending)
			}
			if test.wantProtocol != ProtocolNone && !capability.Available() {
				t.Fatal("parsed capability should be available")
			}
		})
	}
}

func TestStripProbeReplies(t *testing.T) {
	reply := "\x1b[6;20;10t\x1b[?62;4;6;22c\x1b_Gi=7319;OK\x1b\\abc"
	if got := string(stripProbeReplies([]byte(reply))); got != "abc" {
		t.Fatalf("stripProbeReplies() = %q, want %q", got, "abc")
	}
}

func TestParseProbeReplyNoControlBytes(t *testing.T) {
	capability, pending := parseProbeReply([]byte("kitty says OK"))
	if capability.Available() {
		t.Fatal("plain text must not be interpreted as a capability")
	}
	if !strings.Contains(string(pending), "kitty") {
		t.Fatalf("unexpected pending bytes: %q", pending)
	}
}
