package minidownload

import "testing"

func TestCanAutoInstallFFmpegOnlyOnWindows(t *testing.T) {
	if !canAutoInstallFFmpeg("windows") {
		t.Fatal("windows should support automatic ffmpeg install")
	}
	if canAutoInstallFFmpeg("darwin") {
		t.Fatal("darwin should not download windows ffmpeg.exe")
	}
	if canAutoInstallFFmpeg("linux") {
		t.Fatal("linux should not download windows ffmpeg.exe")
	}
}
