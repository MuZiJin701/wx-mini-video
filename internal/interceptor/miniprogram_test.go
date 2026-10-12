package interceptor

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

type testProxyContext struct {
	req     *proxy.ContextReq
	headers http.Header
	body    []byte
	reads   int
	onRead  func()
}

func TestMiniProgramPluginRejectsOtherAppID(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx456/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	}
	plugin.OnResponse(ctx)
	if got := len(store.List("")); got != 0 {
		t.Fatalf("other AppID candidates = %d, want 0", got)
	}
}

func (c *testProxyContext) Req() *proxy.ContextReq { return c.req }

func (c *testProxyContext) Res() *proxy.ContextRes { return nil }

func (c *testProxyContext) Mock(int, map[string]string, string) {}

func (c *testProxyContext) GetResponseHeader(key string) string { return c.headers.Get(key) }

func (c *testProxyContext) SetResponseHeader(key, value string) { c.headers.Set(key, value) }

func (c *testProxyContext) SetResponseBody(body string) { c.body = []byte(body) }

func (c *testProxyContext) GetResponseBody() ([]byte, error) {
	c.reads++
	if c.onRead != nil {
		c.onRead()
	}
	return c.body, nil
}

func (c *testProxyContext) SetStatusCode(int) {}

func TestMiniProgramPluginDoesNotWriteStdout(t *testing.T) {
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

	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	plugin.OnResponse(&testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://example.com/video.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	})

	_ = writer.Close()
	var out bytes.Buffer
	_, _ = io.Copy(&out, reader)
	if strings.Contains(out.String(), "[MINIPROGRAM]") || out.Len() > 0 {
		t.Fatalf("media plugin wrote stdout: %q", out.String())
	}
}

func TestMiniProgramPluginSkipsM3U8CandidateFromJSON(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://mini.example.com/api/feed"},
			Header: http.Header{"User-Agent": {"wx-test"}, "Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"application/json"}},
		body:    []byte(`{"playUrl":"https://cdn.example.com/live/index.m3u8"}`),
	}

	plugin.OnResponse(ctx)
	got := store.List("wx123")
	if len(got) != 0 {
		t.Fatalf("JSON m3u8 candidates = %#v, want none", got)
	}
}

func TestMiniProgramPluginAddsDirectM3U8Response(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/live/index.m3u8"},
			Header: http.Header{"User-Agent": {"wx-test"}, "Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}},
		body:    []byte("#EXTM3U\n#EXT-X-ENDLIST\n"),
	}

	plugin.OnResponse(ctx)
	got := store.List("wx123")
	if len(got) != 1 || got[0].Kind != "m3u8" || got[0].Source != "response" {
		t.Fatalf("direct m3u8 candidates = %#v, want one response candidate", got)
	}
	if got[0].CachedPath == "" {
		t.Fatalf("candidate = %#v, want cached playlist", got[0])
	}
}

func TestMiniProgramDiscoveryDoesNotCollectMedia(t *testing.T) {
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{}, store, dir)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/live.m3u8"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}},
		body:    []byte("#EXTM3U\n#EXT-X-ENDLIST\n"),
	}
	plugin.OnResponse(ctx)
	plugin.OnResponse(ctx)
	programs := store.Programs()
	if len(programs) != 1 || programs[0].AppID != "wx123" || programs[0].Name != "" {
		t.Fatalf("discovered programs = %#v, want one unnamed wx123", programs)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List("")) != 0 || len(files) != 0 || ctx.reads != 0 {
		t.Fatal("discovery collected media, cached a playlist or read the response body")
	}
}

func TestMiniProgramSwitchRejectsInFlightResponse(t *testing.T) {
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/old.m3u8"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}},
		body:    []byte("#EXTM3U\n#EXT-X-ENDLIST\n"),
		onRead:  func() { store.SetTarget(miniprogram.Target{AppID: "wx456"}) },
	}
	plugin.OnResponse(ctx)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.List("")) != 0 || len(files) != 0 {
		t.Fatal("in-flight old target response restored a candidate or wrote a cache")
	}
	ctx = &testProxyContext{
		req: &proxy.ContextReq{
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/new.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx456/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	}
	plugin.OnResponse(ctx)
	if got := store.List(""); len(got) != 1 || got[0].AppID != "wx456" {
		t.Fatalf("candidates after switch = %#v, want only wx456", got)
	}
}

func TestMiniProgramPluginRejectsUnknownSourcesWithoutSideEffects(t *testing.T) {
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
	}))
	defer server.Close()
	for _, referer := range []string{
		"", "not a URL", "http://servicewechat.com/wx123/1/page-frame.html",
		"https://servicewechat.com.evil.example/wx123/1/page-frame.html",
		"https://servicewechat.com@evil.example/wx123/1/page-frame.html",
		"https://user@servicewechat.com/wx123/1/page-frame.html",
		"https://servicewechat.com:444/wx123/1/page-frame.html",
		"https://servicewechat.com/wx1234/1/page-frame.html",
		"https://servicewechat.com/wx123//page-frame.html",
		"https://servicewechat.com/wx123/1/other.html",
		"https://servicewechat.com/wx123/1/page-frame.html?appid=wx123",
		"https://servicewechat.com/wx123/1/page-frame.html#wx123",
		"https://servicewechat.com/%77x123/1/page-frame.html",
	} {
		t.Run(referer, func(t *testing.T) {
			store := miniprogram.NewStore()
			dir := t.TempDir()
			plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
			ctx := &testProxyContext{
				req:     &proxy.ContextReq{URL: &proxy.ContextURL{String: server.URL + "/media.m3u8"}, Header: http.Header{"Referer": {referer}}},
				headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}},
			}
			plugin.OnResponse(ctx)
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(store.List("")) != 0 || ctx.reads != 0 || len(files) != 0 || fetches.Load() != 0 {
				t.Fatal("rejected source collected media, read a body, wrote a cache or fetched a playlist")
			}
		})
	}
}

func TestMiniProgramPluginCollectsOnlyMatchedJSONMedia(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123", Name: "测试小程序"}, store, t.TempDir())
	body := []byte(`{"title":"媒体标题","cover":"https://cdn.example.com/cover.jpg","video":"https://cdn.example.com/video.mp4","audio":"https://cdn.example.com/audio.m4a","playlist":"https://cdn.example.com/live.m3u8"}`)
	for _, appID := range []string{"wx456", "wx123"} {
		ctx := &testProxyContext{
			req:     &proxy.ContextReq{URL: &proxy.ContextURL{String: "https://api.example.com/feed"}, Header: http.Header{"Referer": {"https://servicewechat.com/" + appID + "/devtools/page-frame.html"}}},
			headers: http.Header{"Content-Type": {"application/json"}},
			body:    append([]byte(nil), body...),
		}
		plugin.OnResponse(ctx)
		if string(ctx.body) != string(body) {
			t.Fatal("filter changed the forwarded response body")
		}
		if appID == "wx456" && (ctx.reads != 0 || len(store.List("")) != 0) {
			t.Fatal("other AppID JSON was read or extracted")
		}
	}
	got := store.List("")
	if len(got) != 3 {
		t.Fatalf("matched JSON candidates = %d, want image, video and audio", len(got))
	}
	for _, candidate := range got {
		if candidate.AppID != "wx123" || candidate.AppName != "测试小程序" || candidate.Title != "媒体标题" || candidate.SourceURL != "https://api.example.com/feed" || candidate.Kind == "m3u8" {
			t.Fatalf("matched JSON candidate has wrong ownership or source: %#v", candidate)
		}
	}
}

func TestMiniProgramPluginFiltersDirectAudio(t *testing.T) {
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
	for _, appID := range []string{"", "wx456", "wx123"} {
		ctx := &testProxyContext{
			req: &proxy.ContextReq{
				URL:    &proxy.ContextURL{String: "https://cdn.example.com/play"},
				Header: http.Header{"Referer": {"https://servicewechat.com/" + appID + "/1/page-frame.html"}},
			},
			headers: http.Header{"Content-Type": {"audio/mpeg"}},
			body:    []byte("ID3 audio bytes"),
		}
		plugin.OnResponse(ctx)
		if ctx.reads != 0 || string(ctx.body) != "ID3 audio bytes" {
			t.Fatal("direct audio sniffing should leave the forwarded body untouched")
		}
		if appID != "wx123" && len(store.List("")) != 0 {
			t.Fatal("unknown or other AppID audio was admitted")
		}
	}
	got := store.List("wx123")
	if len(got) != 1 || got[0].Kind != "audio" || got[0].Suffix != ".mp3" || got[0].Source != "response" {
		t.Fatalf("matched direct audio candidates = %#v", got)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("audio sniffing should not create a media cache: %v, %v", files, err)
	}
}
