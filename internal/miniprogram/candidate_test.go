package miniprogram

import (
	"net/http"
	"testing"
	"time"
)

func TestParseContentRangeValidatesCompleteEntityLength(t *testing.T) {
	for _, raw := range []string{"bytes 0-2/*", "bytes 3-2/6", "bytes 0-6/6", "bytes -1-2/6", "bytes 0-2/9999999999999999999999999", "garbage"} {
		if _, _, _, ok := ParseContentRange(raw); ok {
			t.Fatalf("accepted invalid range %q", raw)
		}
	}
	start, end, total, ok := ParseContentRange("bytes 3-5/6")
	if !ok || start != 3 || end != 5 || total != 6 {
		t.Fatal("valid range rejected")
	}
}

func TestClassifyDetectsMedia(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		rawURL      string
		wantKind    string
		wantSuffix  string
	}{
		{name: "jpeg content type", contentType: "image/jpeg", rawURL: "https://example.com/img", wantKind: "image", wantSuffix: ".jpg"},
		{name: "webp extension fallback", contentType: "application/octet-stream", rawURL: "https://example.com/cover.webp?token=1", wantKind: "image", wantSuffix: ".webp"},
		{name: "mp4 content type", contentType: "video/mp4; charset=utf-8", rawURL: "https://example.com/play", wantKind: "video", wantSuffix: ".mp4"},
		{name: "m3u8 content type", contentType: "application/vnd.apple.mpegurl", rawURL: "https://example.com/play", wantKind: "m3u8", wantSuffix: ".m3u8"},
		{name: "mp4 extension fallback", contentType: "application/octet-stream", rawURL: "https://example.com/video.mp4?token=1", wantKind: "video", wantSuffix: ".mp4"},
		{name: "mp3 content type without extension", contentType: "audio/mpeg", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".mp3"},
		{name: "audio mp4 before video extension", contentType: "audio/mp4; codecs=mp4a.40.2", rawURL: "https://example.com/audio.mp4", wantKind: "audio", wantSuffix: ".m4a"},
		{name: "aac content type", contentType: "audio/aac", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".aac"},
		{name: "wav content type", contentType: "audio/vnd.wave", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".wav"},
		{name: "ogg content type", contentType: "audio/ogg", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".ogg"},
		{name: "flac content type", contentType: "audio/flac", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".flac"},
		{name: "opus content type", contentType: "audio/opus", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".opus"},
		{name: "amr content type", contentType: "audio/amr", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".amr"},
		{name: "video ogg is not audio", contentType: "video/ogg", rawURL: "https://example.com/clip.ogg", wantKind: "", wantSuffix: ""},
		{name: "unknown", contentType: "text/html", rawURL: "https://example.com/page", wantKind: "", wantSuffix: ""},
	}
	for _, extension := range []string{"mp3", "m4a", "aac", "wav", "ogg", "flac", "opus", "amr"} {
		tests = append(tests, struct {
			name, contentType, rawURL, wantKind, wantSuffix string
		}{extension + " extension", "application/octet-stream", "https://example.com/track." + extension + "?token=1", "audio", "." + extension})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotSuffix := Classify(tt.contentType, tt.rawURL)
			if gotKind != tt.wantKind || gotSuffix != tt.wantSuffix {
				t.Fatalf("Classify() = (%q, %q), want (%q, %q)", gotKind, gotSuffix, tt.wantKind, tt.wantSuffix)
			}
		})
	}
}

func TestExtractMediaURLsFromJSON(t *testing.T) {
	body := []byte(`{
		"data": {
			"videoUrl": "https://cdn.example.com/a.mp4?token=1",
			"nested": {"playUrl": "https://cdn.example.com/live/index.m3u8"},
			"coverImage": "https://cdn.example.com/cover.jpg",
			"ignore": "https://cdn.example.com/page.html"
		}
	}`)
	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 3 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 3: %#v", len(got), got)
	}
	if got[0].Kind == "" || got[1].Kind == "" {
		t.Fatalf("expected classified media candidates, got %#v", got)
	}
}

func TestExtractMediaURLsFromJSONFindsImageFields(t *testing.T) {
	body := []byte(`{
		"data": {
			"avatar": "https://cdn.example.com/avatar.png",
			"poster": "https://cdn.example.com/poster.avif",
			"thumbnailUrl": "https://cdn.example.com/thumb.webp"
		}
	}`)
	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 3 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 3: %#v", len(got), got)
	}
	for _, item := range got {
		if item.Kind != "image" {
			t.Fatalf("expected image candidate, got %#v", item)
		}
	}
}

func TestExtractMediaURLsFromJSONUsesNearbyTitle(t *testing.T) {
	body := []byte(`{
		"data": {
			"title": "春日活动",
			"videoUrl": "https://cdn.example.com/videos/spring.mp4"
		}
	}`)

	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 1 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 1: %#v", len(got), got)
	}
	if got[0].Title != "春日活动" {
		t.Fatalf("Title = %q, want nearby JSON title", got[0].Title)
	}
}

func TestExtractMediaURLsFromJSONUsesAudioTitle(t *testing.T) {
	got := ExtractMediaURLsFromJSON([]byte(`{"audioTitle":"课程讲解","voiceUrl":"https://cdn.example.com/lesson.m4a"}`))
	if len(got) != 1 || got[0].Kind != "audio" || got[0].Suffix != ".m4a" || got[0].Title != "课程讲解" || got[0].FieldPath != "voiceUrl" {
		t.Fatalf("JSON audio title or source was lost: %#v", got)
	}
}

func TestCleanHeadersRemovesUnsafeReplayHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", "ua")
	h.Set("Referer", "https://example.com")
	h.Set("Cookie", "a=b")
	h.Set("Proxy-Authorization", "proxy-secret")
	h.Set("Host", "bad")
	h.Set("Accept-Encoding", "gzip")
	h.Set("If-None-Match", "etag")
	h.Set("If-Modified-Since", "yesterday")
	h.Set("Range", "bytes=0-")
	got := CleanHeaders(h)
	if got["User-Agent"] != "ua" || got["Referer"] != "https://example.com" || got["Cookie"] != "a=b" {
		t.Fatalf("expected replay headers to be preserved, got %#v", got)
	}
	if _, ok := got["Host"]; ok {
		t.Fatalf("Host should be removed, got %#v", got)
	}
	if _, ok := got["Proxy-Authorization"]; ok {
		t.Fatal("proxy credentials must not be replayed to media servers")
	}
	if _, ok := got["Accept-Encoding"]; ok {
		t.Fatalf("Accept-Encoding should be removed, got %#v", got)
	}
	if _, ok := got["If-None-Match"]; ok {
		t.Fatalf("If-None-Match should be removed, got %#v", got)
	}
	if _, ok := got["If-Modified-Since"]; ok {
		t.Fatalf("If-Modified-Since should be removed, got %#v", got)
	}
	if _, ok := got["Range"]; ok {
		t.Fatalf("Range should be removed, got %#v", got)
	}
}

func TestStoreOnCandidateAddedRunsAfterUnlock(t *testing.T) {
	store := NewStore()
	done := make(chan struct{})
	store.OnCandidateAdded = func(Candidate) {
		_ = store.List("")
		close(done)
	}

	go store.Add(Candidate{URL: "https://example.com/video.mp4", AppID: "app"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OnCandidateAdded appears to run while store lock is held")
	}
}

func TestStoreAddMergesDuplicateM3U8Metadata(t *testing.T) {
	store := NewStore()
	first, changed := store.Add(Candidate{
		AppID: "app",
		URL:   "https://example.com/index.m3u8",
		Kind:  "m3u8",
	})
	if !changed {
		t.Fatal("first Add should report a change")
	}

	updated, changed := store.Add(Candidate{
		AppID:         "app",
		URL:           first.URL,
		Kind:          "m3u8",
		ContentType:   "application/vnd.apple.mpegurl",
		ContentLength: 1234,
		Headers:       map[string]string{"Referer": "https://example.com"},
		CachedPath:    "downloads/index.m3u8",
	})
	if !changed {
		t.Fatal("duplicate Add with cache metadata should report a change")
	}
	if updated.CachedPath != "downloads/index.m3u8" {
		t.Fatalf("CachedPath = %q, want updated path", updated.CachedPath)
	}

	list := store.List("app")
	if len(list) != 1 {
		t.Fatalf("len(List) = %d, want 1", len(list))
	}
	if list[0].CachedPath != "downloads/index.m3u8" || list[0].ContentLength != 1234 {
		t.Fatalf("stored candidate was not merged: %#v", list[0])
	}
	if list[0].Headers["Referer"] != "https://example.com" {
		t.Fatalf("stored headers were not merged: %#v", list[0].Headers)
	}
}
