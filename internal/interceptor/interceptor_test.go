package interceptor

import (
	"testing"

	"wx_channel/pkg/system"
)

func TestIsOwnProxySnapshot(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "plain", raw: "127.0.0.1:2023", want: true},
		{name: "per scheme", raw: "http=127.0.0.1:2023;https=127.0.0.1:2023", want: true},
		{name: "other proxy", raw: "proxy.example.com:8080", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isOwnProxySnapshot(&system.ProxySnapshot{
				Enabled:   true,
				HasServer: true,
				Server:    tc.raw,
			}, "127.0.0.1", "2023")
			if got != tc.want {
				t.Fatalf("isOwnProxySnapshot(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
