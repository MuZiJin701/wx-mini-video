package app

import "testing"

func TestConfiguredFFmpegPathKeepsExplicitConfig(t *testing.T) {
	got := configuredFFmpegPath(`D:\tools\ffmpeg.exe`)
	if got != `D:\tools\ffmpeg.exe` {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestConfiguredFFmpegPathLeavesEmptyConfigUnmanaged(t *testing.T) {
	got := configuredFFmpegPath("")
	if got != "" {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestConfiguredFFmpegPathTreatsLegacyDefaultAsUnmanaged(t *testing.T) {
	if got := configuredFFmpegPath("ffmpeg"); got != "" {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}
