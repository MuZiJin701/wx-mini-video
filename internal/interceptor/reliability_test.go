package interceptor

import (
	"net/http"
	"os"
	"testing"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

func TestPlaylistCaptureKeepsRelativeURLsForAuthenticatedReplay(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	body := []byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.key\"\n#EXTINF:1,\nseg.ts\n#EXT-X-ENDLIST\n")
	plugin.OnResponse(&testProxyContext{
		req:     &proxy.ContextReq{URL: &proxy.ContextURL{String: "https://cdn.example.com/media.m3u8?auth=signature"}, Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}}},
		headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}}, body: body,
	})
	items := store.List("wx123")
	if len(items) != 1 {
		t.Fatal("playlist missing")
	}
	cached, err := os.ReadFile(items[0].CachedPath)
	if err != nil || string(cached) != string(body) {
		t.Fatal("capture rewrote relative references before authenticated replay")
	}
}

func TestJSONCandidatesDoNotInheritForeignCredentialsAndResponseRefreshesHeaders(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	plugin.OnResponse(&testProxyContext{
		req: &proxy.ContextReq{URL: &proxy.ContextURL{String: "https://api.example.com/feed"}, Header: http.Header{
			"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}, "Authorization": {"api-secret"}, "Cookie": {"api-session"}, "X-API-Key": {"api-key"},
		}}, headers: http.Header{"Content-Type": {"application/json"}},
		body: []byte(`{"videoUrl":"https://cdn.example.com/video.mp4","ownUrl":"https://api.example.com/own.mp4"}`),
	})
	for _, candidate := range store.List("wx123") {
		if candidate.URL == "https://api.example.com/own.mp4" {
			if candidate.Headers["Authorization"] != "api-secret" {
				t.Fatal("lost same-origin authentication")
			}
			continue
		}
		for _, name := range []string{"Authorization", "Cookie", "X-Api-Key", "X-API-Key"} {
			if candidate.Headers[name] != "" {
				t.Fatalf("cross-origin JSON candidate leaked %s", name)
			}
		}
	}
	plugin.OnResponse(&testProxyContext{
		req: &proxy.ContextReq{URL: &proxy.ContextURL{String: "https://cdn.example.com/video.mp4"}, Header: http.Header{
			"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}, "Authorization": {"media-secret"},
		}}, headers: http.Header{"Content-Type": {"video/mp4"}, "Content-Length": {"3"}, "Content-Range": {"bytes 0-2/6"}},
	})
	candidate, ok := store.Get(miniprogram.CandidateID("wx123", "https://cdn.example.com/video.mp4"))
	if !ok || candidate.Headers["Authorization"] != "media-secret" || candidate.ContentType != "video/mp4" || candidate.ContentLength != 6 {
		t.Fatalf("real response did not replace provisional metadata: %#v", candidate)
	}
}

func TestClearRejectsInFlightPlaylistAndPreservesTarget(t *testing.T) {
	store := miniprogram.NewStore()
	plugin := CreateMiniProgramInterceptorPlugin(miniprogram.Target{AppID: "wx123"}, store, t.TempDir())
	ctx := &testProxyContext{
		req:     &proxy.ContextReq{URL: &proxy.ContextURL{String: "https://cdn.example.com/live.m3u8"}, Header: http.Header{"Referer": {"https://servicewechat.com/wx123/1/page-frame.html"}}},
		headers: http.Header{"Content-Type": {"application/vnd.apple.mpegurl"}}, body: []byte("#EXTM3U\n#EXT-X-ENDLIST\n"),
		onRead: func() { store.Clear("wx123") },
	}
	plugin.OnResponse(ctx)
	if len(store.List("wx123")) != 0 {
		t.Fatal("cleared in-flight response returned to candidates")
	}
	ctx.onRead = nil
	plugin.OnResponse(ctx)
	if len(store.List("wx123")) != 1 {
		t.Fatal("clear removed the target or rejected a newly loaded response")
	}
}
