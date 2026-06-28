package app

import "testing"

func TestConfiguredFFmpegPathKeepsExplicitConfig(t *testing.T) {
	got := configuredFFmpegPath(`D:\tools\ffmpeg.exe`)
	if got != `D:\tools\ffmpeg.exe` {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}

func TestConfiguredFFmpegPathDefaultsToFFmpeg(t *testing.T) {
	got := configuredFFmpegPath("")
	if got != "ffmpeg" {
		t.Fatalf("configuredFFmpegPath() = %q", got)
	}
}
