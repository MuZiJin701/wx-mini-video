package interceptor

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

type testProxyContext struct {
	req        *proxy.ContextReq
	headers    http.Header
	body       []byte
	reads      int
	statusCode int
	onRead     func()
}

func TestMiniProgramPluginSendsOtherAppIDMediaToUnattributed(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx456/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	}
	plugin.OnResponse(ctx)
	if got := len(store.List("")); got != 0 {
		t.Fatalf("other AppID candidates leaked into target list: %d", got)
	}
	unattributed := store.Unattributed()
	if len(unattributed) != 1 {
		t.Fatalf("unattributed candidates = %d, want 1", len(unattributed))
	}
	got := unattributed[0]
	if got.Kind != "video" || got.SourceAppID != "wx456" || got.Attribution != miniprogram.AttributionUnattributed {
		t.Fatalf("unattributed candidate = %#v, want video from wx456", got)
	}
}

func TestMiniProgramPluginCanDisableUnattributedCapture(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir(), false)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	}
	plugin.OnResponse(ctx)
	if got := len(store.Unattributed()); got != 0 {
		t.Fatalf("unattributed candidates = %d, want 0 when capture is disabled", got)
	}
}

func (c *testProxyContext) Req() *proxy.ContextReq { return c.req }

func (c *testProxyContext) Res() *proxy.ContextRes {
	status := c.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &proxy.ContextRes{StatusCode: status, Header: c.headers}
}

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
	if len(store.List("")) != 0 || len(store.Unattributed()) != 0 || len(files) != 0 || ctx.reads != 0 {
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

func TestMiniProgramPluginSendsUnknownSourcesToUnattributedWithoutSideEffects(t *testing.T) {
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
			unattributed := store.Unattributed()
			if len(store.List("")) != 0 || len(unattributed) != 1 || unattributed[0].Kind != "m3u8" {
				t.Fatalf("unknown source handling = target %#v, unattributed %#v", store.List(""), unattributed)
			}
			if unattributed[0].CachedPath != "" {
				t.Fatal("unattributed m3u8 must not be cached")
			}
			if ctx.reads != 0 || len(files) != 0 || fetches.Load() != 0 {
				t.Fatal("rejected source read a body, wrote a cache or fetched a playlist")
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

func TestMiniProgramPluginAdmitsH5VideoToUnattributedWithRealHeaders(t *testing.T) {
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123", Name: "课程小程序"}, store, dir)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://course.example.com/media/lesson1.mp4?sign=1"},
			Header: http.Header{
				"Referer": {"https://course.example.com/learn/1"},
				"Cookie":  {"sid=real-session"},
			},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}, "Content-Length": {"1048576"}},
	}
	plugin.OnResponse(ctx)

	if got := len(store.List("")); got != 0 {
		t.Fatalf("H5 video leaked into target list: %d", got)
	}
	got := store.Unattributed()
	if len(got) != 1 {
		t.Fatalf("unattributed candidates = %d, want 1", len(got))
	}
	candidate := got[0]
	if candidate.Attribution != miniprogram.AttributionUnattributed || candidate.Kind != "video" || candidate.Suffix != ".mp4" {
		t.Fatalf("candidate attribution/kind = %#v", candidate)
	}
	if candidate.SourceAppID != "" {
		t.Fatalf("H5 referer must not invent an AppID, got %q", candidate.SourceAppID)
	}
	if candidate.Headers["Referer"] != "https://course.example.com/learn/1" || candidate.Headers["Cookie"] != "sid=real-session" {
		t.Fatalf("unattributed candidate lost replay headers: %#v", candidate.Headers)
	}
	if candidate.ContentLength != 1048576 {
		t.Fatalf("ContentLength = %d, want response length", candidate.ContentLength)
	}
	if ctx.reads != 0 {
		t.Fatal("direct video response should not be buffered")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("direct video should not write cache files: %v, %v", files, err)
	}
}

func TestMiniProgramPluginMarksCanonicalRefererAsConfirmed(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123", Name: "目标"}, store, t.TempDir())
	plugin.OnResponse(&testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/media/lesson.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/42/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	})
	got := store.List("wx123")
	if len(got) != 1 {
		t.Fatalf("target candidates = %d, want 1", len(got))
	}
	if got[0].Attribution != miniprogram.AttributionConfirmed || got[0].SourceAppID != "wx123" || got[0].AppID != "wx123" {
		t.Fatalf("confirmed candidate = %#v", got[0])
	}
	if len(store.Unattributed()) != 0 {
		t.Fatal("confirmed media must not appear in unattributed list")
	}
}

func TestMiniProgramPluginUnknownBinaryFallback(t *testing.T) {
	tests := []struct {
		name               string
		status             int
		contentType        string
		contentLength      string
		contentRange       string
		contentDisposition string
		wantKind           string
		wantSuffix         string
		wantCount          int
	}{
		{name: "octet-stream large", status: http.StatusOK, contentType: "application/octet-stream", contentLength: "314572800", contentDisposition: `attachment; filename="lesson.pdf"`, wantKind: "file", wantSuffix: ".pdf", wantCount: 1},
		{name: "binary type large", status: http.StatusOK, contentType: "binary/octet-stream", contentLength: "262144", wantKind: "file", wantSuffix: ".bin", wantCount: 1},
		{name: "unrecognized type large", status: http.StatusOK, contentType: "application/x-course-data", contentLength: "524288", wantKind: "file", wantSuffix: ".bin", wantCount: 1},
		{name: "range total large", status: http.StatusPartialContent, contentType: "application/octet-stream", contentRange: "bytes 0-1023/524288", wantKind: "file", wantSuffix: ".bin", wantCount: 1},
		{name: "small binary dropped", status: http.StatusOK, contentType: "application/octet-stream", contentLength: "1024", wantCount: 0},
		{name: "small range dropped", status: http.StatusPartialContent, contentType: "application/octet-stream", contentRange: "bytes 0-1023/131072", wantCount: 0},
		{name: "html excluded", status: http.StatusOK, contentType: "text/html; charset=utf-8", contentLength: "524288", wantCount: 0},
		{name: "wasm excluded", status: http.StatusOK, contentType: "application/wasm", contentLength: "524288", wantCount: 0},
		{name: "font excluded", status: http.StatusOK, contentType: "font/woff2", contentLength: "524288", wantCount: 0},
		{name: "svg excluded", status: http.StatusOK, contentType: "image/svg+xml", contentLength: "524288", wantCount: 0},
		{name: "non-200 status dropped", status: http.StatusNotFound, contentType: "application/octet-stream", contentLength: "524288", wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := miniprogram.NewStore()
			dir := t.TempDir()
			plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
			headers := http.Header{"Content-Type": {tt.contentType}}
			if tt.contentLength != "" {
				headers.Set("Content-Length", tt.contentLength)
			}
			if tt.contentRange != "" {
				headers.Set("Content-Range", tt.contentRange)
			}
			if tt.contentDisposition != "" {
				headers.Set("Content-Disposition", tt.contentDisposition)
			}
			ctx := &testProxyContext{
				statusCode: tt.status,
				req: &proxy.ContextReq{
					Method: http.MethodGet,
					URL:    &proxy.ContextURL{String: "https://cdn.example.com/download/asset"},
					Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
				},
				headers: headers,
			}
			plugin.OnResponse(ctx)
			got := store.List("wx123")
			if len(got) != tt.wantCount {
				t.Fatalf("candidates = %#v, want %d", got, tt.wantCount)
			}
			if tt.wantCount == 0 {
				// HTML/JS 页面需要读取正文做内联媒体扫描，其余被丢弃类型不应读取正文。
				if ctx.reads != 0 && !strings.HasPrefix(tt.contentType, "text/html") {
					t.Fatal("dropped unknown binary should not be buffered")
				}
				return
			}
			if got[0].Kind != tt.wantKind || got[0].Suffix != tt.wantSuffix || got[0].Attribution != miniprogram.AttributionConfirmed {
				t.Fatalf("file candidate = %#v", got[0])
			}
			if got[0].CachedPath != "" {
				t.Fatal("file candidate must not be cached")
			}
		})
	}
}

func TestMiniProgramPluginJSONJoinsTokenAndProvisionalField(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123", Name: "课程"}, store, t.TempDir())
	body := []byte(`{"data":{"url":"https://cdn.example.com/a/video.mp4","urlToken":"?auth=abc","videoAddress":"https://cdn.example.com/b/play","cover":"https://cdn.example.com/c/cover"}}`)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://api.example.com/feed"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"application/json"}},
		body:    append([]byte(nil), body...),
	}
	plugin.OnResponse(ctx)
	if string(ctx.body) != string(body) {
		t.Fatal("sniffing changed the forwarded JSON body")
	}
	got := store.List("wx123")
	if len(got) != 2 {
		t.Fatalf("JSON candidates = %#v, want joined video and provisional video", got)
	}
	var joined, provisional bool
	for _, candidate := range got {
		switch {
		case strings.Contains(candidate.URL, "?auth=abc"):
			joined = true
			if candidate.Kind != "video" || candidate.Provisional || !strings.Contains(candidate.URL, "video.mp4?auth=abc") {
				t.Fatalf("joined candidate = %#v", candidate)
			}
		case candidate.Provisional:
			provisional = true
			if candidate.Kind != "video" || candidate.Suffix != ".mp4" {
				t.Fatalf("provisional candidate = %#v", candidate)
			}
		}
	}
	if !joined || !provisional {
		t.Fatalf("joined=%v provisional=%v candidates=%#v", joined, provisional, got)
	}
}

func TestMiniProgramPluginDebugLogWhenEnabled(t *testing.T) {
	t.Setenv(debugEnvVar, "1")
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
	ctx := &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://course.example.com/media/lesson1.mp4?token=SECRETQUERY"},
			Header: http.Header{
				"Referer":       {"https://course.example.com/learn/1?session=SECRETSESSION"},
				"Cookie":        {"sid=COOKIESECRET"},
				"Authorization": {"Bearer AUTHSECRET"},
			},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	}
	plugin.OnResponse(ctx)

	data, err := os.ReadFile(filepath.Join(dir, debugLogFileName))
	if err != nil {
		t.Fatalf("debug log was not written: %v", err)
	}
	log := string(data)
	for _, want := range []string{
		"method=GET",
		"url=https://course.example.com/media/lesson1.mp4",
		"status=200",
		"content-type=video/mp4",
		"referer=https://course.example.com/learn/1",
		"target-app-id=wx123",
		"kind=video",
		"decision=admit-unattributed",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("debug log = %q, want %q", log, want)
		}
	}
	for _, secret := range []string{"SECRETQUERY", "SECRETSESSION", "COOKIESECRET", "AUTHSECRET"} {
		if strings.Contains(log, secret) {
			t.Fatalf("debug log leaked %q: %q", secret, log)
		}
	}
	if strings.Contains(log, "Cookie=") || strings.Contains(log, "Authorization=") {
		t.Fatalf("debug log must not print credential headers: %q", log)
	}
}

func TestMiniProgramPluginDebugLogDisabledByDefault(t *testing.T) {
	t.Setenv(debugEnvVar, "")
	store := miniprogram.NewStore()
	dir := t.TempDir()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, dir)
	plugin.OnResponse(&testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"},
			Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}},
		},
		headers: http.Header{"Content-Type": {"video/mp4"}},
	})
	if _, err := os.Stat(filepath.Join(dir, debugLogFileName)); !os.IsNotExist(err) {
		t.Fatalf("debug log should stay disabled by default: %v", err)
	}
}
