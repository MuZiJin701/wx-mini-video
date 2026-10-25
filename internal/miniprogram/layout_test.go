package miniprogram

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaKind(t *testing.T) {
	tests := []struct {
		name      string
		candidate Candidate
		want      string
	}{
		{name: "image", candidate: Candidate{Kind: "image", URL: "https://cdn.example.com/a.jpg"}, want: "image"},
		{name: "video", candidate: Candidate{Kind: "video", URL: "https://cdn.example.com/a.mp4"}, want: "video"},
		{name: "audio", candidate: Candidate{Kind: "audio", URL: "https://cdn.example.com/a.m4a"}, want: "audio"},
		{name: "m3u8 kind", candidate: Candidate{Kind: "m3u8", URL: "https://cdn.example.com/live"}, want: "m3u8"},
		{name: "m3u8 extension wins", candidate: Candidate{Kind: "video", URL: "https://cdn.example.com/live/index.m3u8?token=1"}, want: "m3u8"},
		{name: "live wins over video kind", candidate: Candidate{Kind: "video", Live: true, URL: "https://cdn.example.com/live/7559_esd.flv"}, want: "live"},
		{name: "live wins over suffixless video", candidate: Candidate{Kind: "video", Live: true, URL: "https://cdn.example.com/media/live-stream"}, want: "live"},
		{name: "finite flv stays video", candidate: Candidate{Kind: "video", URL: "https://cdn.example.com/clip.flv"}, want: "video"},
		{name: "file kind", candidate: Candidate{Kind: "file", URL: "https://cdn.example.com/a.bin"}, want: "file"},
		{name: "unknown kind", candidate: Candidate{Kind: "", URL: "https://cdn.example.com/a"}, want: "file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MediaKind(tt.candidate); got != tt.want {
				t.Fatalf("MediaKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsM3U8(t *testing.T) {
	tests := []struct {
		name      string
		candidate Candidate
		want      bool
	}{
		{name: "m3u8 kind", candidate: Candidate{Kind: "m3u8", URL: "https://cdn.example.com/play"}, want: true},
		{name: "m3u8 extension", candidate: Candidate{Kind: "video", URL: "https://cdn.example.com/index.m3u8?token=1"}, want: true},
		{name: "live flag does not reroute m3u8", candidate: Candidate{Kind: "m3u8", Live: true, URL: "https://cdn.example.com/index.m3u8"}, want: true},
		{name: "flv video", candidate: Candidate{Kind: "video", Live: true, URL: "https://cdn.example.com/a.flv"}, want: false},
		{name: "plain video", candidate: Candidate{Kind: "video", URL: "https://cdn.example.com/a.mp4"}, want: false},
		{name: "image", candidate: Candidate{Kind: "image", URL: "https://cdn.example.com/a.jpg"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsM3U8(tt.candidate); got != tt.want {
				t.Fatalf("IsM3U8() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypeDir(t *testing.T) {
	for kind, want := range map[string]string{
		"image": DirImages,
		"video": DirVideos,
		"audio": DirAudios,
		"live":  DirLive,
		"m3u8":  DirSegments,
		"file":  DirOther,
		"":      DirOther,
	} {
		if got := TypeDir(kind); got != want {
			t.Fatalf("TypeDir(%q) = %q, want %q", kind, got, want)
		}
	}
	for kind, want := range map[string]string{
		"image": "图片",
		"video": "视频",
		"audio": "音频",
		"live":  "直播",
		"m3u8":  "分段媒体",
		"file":  "其他",
	} {
		if got := TypeDir(kind); got != want {
			t.Fatalf("TypeDir(%q) = %q, want literal %q", kind, got, want)
		}
	}
}

func TestSanitizeAppIDDir(t *testing.T) {
	tests := []struct {
		name  string
		appID string
		want  string
	}{
		{name: "normal keeps case", appID: "wxAbC123", want: "wxAbC123"},
		{name: "trim spaces", appID: "  wx123  ", want: "wx123"},
		{name: "trim trailing dots", appID: "wx123...", want: "wx123"},
		{name: "trim space before dot", appID: "wx123 . ", want: "wx123"},
		{name: "illegal characters", appID: `wx<>:"/\|?*1`, want: "wx---------1"},
		{name: "control characters", appID: "wx\x001", want: "wx-1"},
		{name: "empty", appID: "", want: "_unknown"},
		{name: "spaces only", appID: "   ", want: "_unknown"},
		{name: "dot", appID: ".", want: "_unknown"},
		{name: "dot dot", appID: "..", want: "_unknown"},
		{name: "reserved con", appID: "CON", want: "_CON"},
		{name: "reserved con with extension", appID: "con.txt", want: "_con.txt"},
		{name: "reserved com1", appID: "COM1", want: "_COM1"},
		{name: "reserved lpt9", appID: "lpt9.log", want: "_lpt9.log"},
		{name: "com0 is not reserved", appID: "COM0", want: "COM0"},
		{name: "reserved with trailing dot stripped", appID: "NUL.", want: "_NUL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeAppIDDir(tt.appID); got != tt.want {
				t.Fatalf("SanitizeAppIDDir(%q) = %q, want %q", tt.appID, got, tt.want)
			}
		})
	}
}

func TestNormalizeTargetAppID(t *testing.T) {
	for _, raw := range []string{"wx123", "  wxAbC  ", "app-id_1", "非常规AppID"} {
		got, err := NormalizeTargetAppID(raw)
		if err != nil {
			t.Fatalf("NormalizeTargetAppID(%q) error = %v", raw, err)
		}
		if got != strings.TrimSpace(raw) {
			t.Fatalf("NormalizeTargetAppID(%q) = %q, want trimmed input", raw, got)
		}
	}

	for _, raw := range []string{"", "   ", ".", "..", "a/b", `a\b`, "a:b", "a<b", "a>b", `a"b`, "a|b", "a?b", "a*b", "a\x001"} {
		if got, err := NormalizeTargetAppID(raw); err == nil {
			t.Fatalf("NormalizeTargetAppID(%q) = %q, want error", raw, got)
		}
	}
}

func TestMediaSubdir(t *testing.T) {
	tests := []struct {
		name        string
		candidate   Candidate
		targetAppID string
		want        string
	}{
		{
			name:      "confirmed candidate uses its own app id",
			candidate: Candidate{AppID: "wxCandidate", Kind: "image"},
			want:      filepath.Join("wxCandidate", DirImages),
		},
		{
			name:        "unattributed uses current target",
			candidate:   Candidate{Kind: "video"},
			targetAppID: "wxTarget",
			want:        filepath.Join("wxTarget", DirVideos),
		},
		{
			name:        "confirmed app id wins over target",
			candidate:   Candidate{AppID: "wxCandidate", Kind: "audio"},
			targetAppID: "wxTarget",
			want:        filepath.Join("wxCandidate", DirAudios),
		},
		{
			name:        "m3u8 by extension goes to segments",
			candidate:   Candidate{Kind: "video", URL: "https://cdn.example.com/live/index.m3u8"},
			targetAppID: "wxTarget",
			want:        filepath.Join("wxTarget", DirSegments),
		},
		{
			name:        "live candidate goes to live dir",
			candidate:   Candidate{Kind: "video", Live: true, URL: "https://cdn.example.com/7559_esd.flv"},
			targetAppID: "wxTarget",
			want:        filepath.Join("wxTarget", DirLive),
		},
		{
			name:        "finite flv stays in videos",
			candidate:   Candidate{Kind: "video", URL: "https://cdn.example.com/clip.flv"},
			targetAppID: "wxTarget",
			want:        filepath.Join("wxTarget", DirVideos),
		},
		{
			name:      "file goes to other",
			candidate: Candidate{AppID: "wxCandidate", Kind: "file"},
			want:      filepath.Join("wxCandidate", DirOther),
		},
		{
			name:        "missing app id falls back to unknown",
			candidate:   Candidate{Kind: "image"},
			targetAppID: "..",
			want:        filepath.Join("_unknown", DirImages),
		},
		{
			name:      "reserved app id is prefixed",
			candidate: Candidate{AppID: "CON", Kind: "image"},
			want:      filepath.Join("_CON", DirImages),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MediaSubdir(tt.candidate, tt.targetAppID); got != tt.want {
				t.Fatalf("MediaSubdir() = %q, want %q", got, tt.want)
			}
		})
	}
}
