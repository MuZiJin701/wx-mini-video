package echo_test

import (
	"net/http"
	"testing"

	"github.com/ltaoo/echo"
)

func assertMatch(t *testing.T, hostname, pattern string, expected bool) {
	t.Helper()
	got := echo.IsMatch(hostname, pattern)
	if got != expected {
		t.Fatalf("isMatch\n  pattern:   %q\n  hostname:  %q\n  expected:  %v\n  received:  %v", pattern, hostname, expected, got)
	}
}

func TestIsMatch(t *testing.T) {
	cases := []struct {
		name     string
		hostname string
		pattern  string
		expected bool
	}{
		{"exact host", "example.com", "example.com", true},
		{"subdomain vs exact host (substring)", "sub.example.com", "example.com", true},
		{"wildcard single-level", "a.example.com", "*.example.com", true},
		{"wildcard multi-level", "a.b.example.com", "*.example.com", true},
		{"wildcard has suffix slash", "https://www.baidu.com/", "*.baidu.com/*", true},
		{"wildcard not root", "example.com", "*.example.com", false},
		{"substring host positive", "test.example.com", "example", true},
		{"substring host different TLD", "myexample.net", "example", true},
		{"substring host negative", "samples.com", "example", false},
		{"any star matches anything", "anything.com", "*", true},
		{"url exact", "https://api.example.com/index.html", "https://api.example.com/index.html", true},
		{"url wildcard domain and path", "https://api.example.com/index.html", "https://*.example.com/*", true},
		{"url wildcard negative", "https://api.other.com/index.html", "https://*.example.com/*", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertMatch(t, c.hostname, c.pattern, c.expected)
		})
	}
}

func TestPluginLoaderSkipsDisabledPlugins(t *testing.T) {
	disabled := &echo.Plugin{
		Match:    "example.com",
		Disabled: true,
	}
	enabled := &echo.Plugin{
		Match: "example.com",
	}
	loader, err := echo.NewPluginLoader([]*echo.Plugin{disabled, enabled})
	if err != nil {
		t.Fatal(err)
	}

	if got := loader.MatchPlugin("example.com"); got != enabled {
		t.Fatalf("expected enabled plugin, got %#v", got)
	}

	matches := loader.MatchPlugins("example.com")
	if len(matches) != 1 || matches[0] != enabled {
		t.Fatalf("expected only enabled plugin, got %#v", matches)
	}
}

func TestPluginLoaderSkipsDisabledRequestPlugins(t *testing.T) {
	disabled := &echo.Plugin{
		Match:    "https://example.com/api/*",
		Disabled: true,
	}
	enabled := &echo.Plugin{
		Match: "https://example.com/api/*",
	}
	loader, err := echo.NewPluginLoader([]*echo.Plugin{disabled, enabled})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/api/users", nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := loader.MatchPluginForRequest(req); got != enabled {
		t.Fatalf("expected enabled plugin, got %#v", got)
	}

	matches := loader.MatchPluginsForRequest(req)
	if len(matches) != 1 || matches[0] != enabled {
		t.Fatalf("expected only enabled plugin, got %#v", matches)
	}
}
