package interceptor

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

type testProxyContext struct {
	req     *proxy.ContextReq
	headers http.Header
	body    []byte
}

func (c *testProxyContext) Req() *proxy.ContextReq { return c.req }

func (c *testProxyContext) Res() *proxy.ContextRes { return nil }

func (c *testProxyContext) Mock(int, map[string]string, string) {}

func (c *testProxyContext) GetResponseHeader(key string) string { return c.headers.Get(key) }

func (c *testProxyContext) SetResponseHeader(key, value string) { c.headers.Set(key, value) }

func (c *testProxyContext) SetResponseBody(body string) { c.body = []byte(body) }

func (c *testProxyContext) GetResponseBody() ([]byte, error) { return c.body, nil }

func (c *testProxyContext) SetStatusCode(int) {}

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

func TestMiniProgramPluginAddsM3U8CandidateFromJSON(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://mini.example.com/api/feed"},
			Header: http.Header{"User-Agent": {"wx-test"}},
		},
		headers: http.Header{"Content-Type": {"application/json"}},
		body:    []byte(`{"playUrl":"https://cdn.example.com/live/index.m3u8"}`),
	}

	plugin.OnResponse(ctx)
	got := store.List("wx123")
	if len(got) != 1 {
		t.Fatalf("JSON m3u8 candidates = %#v, want one", got)
	}
	if got[0].Kind != "m3u8" || got[0].Source != "json" {
		t.Fatalf("candidate = %#v, want JSON m3u8", got[0])
	}
	if got[0].Headers["User-Agent"] != "wx-test" {
		t.Fatalf("candidate headers = %#v, want session header", got[0].Headers)
	}
}
