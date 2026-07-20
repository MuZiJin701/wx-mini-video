package interceptor

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"wx_channel/internal/miniprogram"
)

func TestAddMiniProgramCandidateDoesNotWriteStdout(t *testing.T) {
	store := miniprogram.NewStore()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
		_ = reader.Close()
	}()

	addMiniProgramCandidate(store, miniprogram.Target{AppID: "wx123"}, miniprogram.Candidate{
		Kind:          "video",
		URL:           "https://example.com/video.mp4?token=secret",
		ContentLength: 1024,
	})

	_ = writer.Close()
	var out bytes.Buffer
	_, _ = io.Copy(&out, reader)
	if strings.Contains(out.String(), "[MINIPROGRAM]") || out.Len() > 0 {
		t.Fatalf("addMiniProgramCandidate wrote stdout: %q", out.String())
	}
}
