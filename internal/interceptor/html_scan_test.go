package interceptor

import (
	"net/http"
	"testing"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

func htmlScanContext(body string, contentType string, pageURL string, referer string) *testProxyContext {
	header := http.Header{}
	if referer != "" {
		header.Set("Referer", referer)
	}
	header.Set("Cookie", "sid=abc")
	return &testProxyContext{
		req: &proxy.ContextReq{
			Method: http.MethodGet,
			URL:    &proxy.ContextURL{String: pageURL},
			Header: header,
		},
		headers: http.Header{"Content-Type": {contentType}},
		body:    []byte(body),
	}
}

func TestHTMLPageScanCapturesInlineVideo(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	pageURL := "https://h5.example.com/course_detail.php?id=1&course_id=10"
	ctx := htmlScanContext(
		`<script>wx.miniProgram.navigateTo({url:'/pages/index/index'});</script>`+
			`<script>var videoObject={video:'https://h5.example.com/uploadfiles/1001.mp4',poster:'https://h5.example.com/a.jpg'};</script>`,
		"text/html; charset=utf-8", pageURL, "https://h5.example.com/course_list.php?id=10")

	plugin.OnResponse(ctx)

	candidates := store.Unattributed()
	if len(candidates) != 1 {
		t.Fatalf("unattributed candidates = %d, want 1", len(candidates))
	}
	got := candidates[0]
	if got.URL != "https://h5.example.com/uploadfiles/1001.mp4" || got.Kind != "video" || got.Source != "html" {
		t.Fatalf("candidate = %#v, want html video", got)
	}
	if got.Headers["Referer"] != pageURL {
		t.Fatalf("candidate Referer = %q, want page URL %q", got.Headers["Referer"], pageURL)
	}
	if got.Headers["Cookie"] != "sid=abc" {
		t.Fatalf("candidate Cookie = %q, want preserved session cookie", got.Headers["Cookie"])
	}
	if len(ctx.body) == 0 {
		t.Fatal("response body should be preserved for the browser")
	}
}

func TestHTMLPageScanSkipsUnrelatedPage(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := htmlScanContext(
		`<html><body><a href="/files/report.mp4">report</a></body></html>`,
		"text/html; charset=utf-8", "https://intranet.example.com/index.html", "")

	plugin.OnResponse(ctx)

	if got := len(store.Unattributed()); got != 0 {
		t.Fatalf("unattributed candidates = %d, want 0 for unrelated page", got)
	}
}

func TestHTMLPageScanCanBeDisabled(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir(), false)
	ctx := htmlScanContext(
		`<script>wx.miniProgram.navigateTo({url:'/x'});var v='https://h5.example.com/a.mp4'</script>`,
		"text/html", "https://h5.example.com/detail", "")

	plugin.OnResponse(ctx)

	if got := len(store.Unattributed()); got != 0 {
		t.Fatalf("unattributed candidates = %d, want 0 when capture is disabled", got)
	}
}

func TestJavaScriptScanCapturesInlineM3U8(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	pageURL := "https://h5.example.com/player.js"
	ctx := htmlScanContext(
		`/* wx.miniProgram */ window.player={src:"https://cdn.example.com/live/index.m3u8?token=1"};`,
		"application/javascript", pageURL, "https://h5.example.com/detail")

	plugin.OnResponse(ctx)

	candidates := store.Unattributed()
	if len(candidates) != 1 || candidates[0].Kind != "m3u8" {
		t.Fatalf("unattributed candidates = %#v, want one m3u8 candidate", candidates)
	}
}
