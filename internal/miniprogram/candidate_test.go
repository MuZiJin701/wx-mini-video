package miniprogram

import (
	"net/http"
	"testing"
	"time"
)

func TestClassifyDetectsVideoAndM3U8(t *testing.T) {
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
		{name: "unknown", contentType: "text/html", rawURL: "https://example.com/page", wantKind: "", wantSuffix: ""},
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

func TestCleanHeadersRemovesUnsafeReplayHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", "ua")
	h.Set("Referer", "https://example.com")
	h.Set("Cookie", "a=b")
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
