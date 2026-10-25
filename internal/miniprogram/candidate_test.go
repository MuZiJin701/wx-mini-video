package miniprogram

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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

func TestStoreUnattributedBucketIsIndependentAndDeduplicated(t *testing.T) {
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, generation := store.Observe("wx123")
	if _, ok := store.AddForTarget(generation, Candidate{AppID: "wx123", URL: "https://cdn.example.com/target.mp4", Kind: "video"}, nil); !ok {
		t.Fatal("target candidate should be admitted")
	}
	first, ok := store.AddUnattributed(Candidate{
		URL:     "https://h5.example.com/lesson.mp4?sign=1",
		Kind:    "video",
		Suffix:  ".mp4",
		Headers: map[string]string{"Referer": "https://h5.example.com/course", "Cookie": "sid=old"},
	})
	if !ok || first.Attribution != AttributionUnattributed {
		t.Fatalf("AddUnattributed() = %#v, %v", first, ok)
	}
	if got := store.List(""); len(got) != 1 || got[0].AppID != "wx123" {
		t.Fatalf("target list = %#v, want only the confirmed candidate", got)
	}
	unattributed := store.Unattributed()
	if len(unattributed) != 1 || unattributed[0].URL != first.URL || unattributed[0].AppID != "" {
		t.Fatalf("unattributed list = %#v", unattributed)
	}

	updated, changed := store.AddUnattributed(Candidate{
		URL:         first.URL,
		Kind:        "video",
		Suffix:      ".mp4",
		SourceAppID: "wx456",
		Headers:     map[string]string{"Referer": "https://h5.example.com/course", "Cookie": "sid=fresh"},
	})
	if !changed || updated.Headers["Cookie"] != "sid=fresh" {
		t.Fatalf("duplicate unattributed candidate did not refresh headers: %#v", updated)
	}
	if got := store.Unattributed(); len(got) != 1 {
		t.Fatalf("duplicate unattributed candidate was not merged: %#v", got)
	}
	if got := store.Unattributed()[0]; got.SourceAppID != "wx456" {
		t.Fatalf("duplicate merge should refresh source AppID, got %q", got.SourceAppID)
	}
	store.ClearUnattributed()
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("ClearUnattributed() kept candidates: %#v", got)
	}
}

func TestStoreUnattributedBucketDropsOldestBeyondLimit(t *testing.T) {
	store := NewStore()
	for i := 0; i < unattributedLimit+3; i++ {
		store.AddUnattributed(Candidate{URL: "https://h5.example.com/media/" + strconv.Itoa(i) + ".mp4", Kind: "video"})
	}
	got := store.Unattributed()
	if len(got) != unattributedLimit {
		t.Fatalf("unattributed len = %d, want %d", len(got), unattributedLimit)
	}
	if !strings.HasSuffix(got[0].URL, "/3.mp4") || !strings.HasSuffix(got[len(got)-1].URL, "/502.mp4") {
		t.Fatalf("unattributed window = %q ... %q", got[0].URL, got[len(got)-1].URL)
	}
}

func TestStoreClearListClearsUnattributedWhileTargetSwitchKeepsIt(t *testing.T) {
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, generation := store.Observe("wx123")
	store.AddForTarget(generation, Candidate{AppID: "wx123", URL: "https://cdn.example.com/target.mp4", Kind: "video"}, nil)
	store.AddUnattributed(Candidate{URL: "https://h5.example.com/lesson.mp4", Kind: "video"})

	store.SetTarget(Target{AppID: "wx456"})
	if got := store.List(""); len(got) != 0 {
		t.Fatalf("target switch kept target candidates: %#v", got)
	}
	if got := store.Unattributed(); len(got) != 1 {
		t.Fatalf("target switch should keep unattributed candidates, got %#v", got)
	}

	store.Clear("wx456")
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("clear list should clear unattributed candidates, got %#v", got)
	}

	store.AddUnattributed(Candidate{URL: "https://h5.example.com/lesson2.mp4", Kind: "video"})
	store.Clear("")
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("clear all should clear unattributed candidates, got %#v", got)
	}
}

func TestExtractMediaURLsFromJSONJoinsAdjacentToken(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantURL string
	}{
		{
			name:    "question mark token",
			body:    `{"url":"https://cdn.example.com/v.mp4","urlToken":"?auth=1"}`,
			wantURL: "https://cdn.example.com/v.mp4?auth=1",
		},
		{
			name:    "ampersand token with existing query",
			body:    `{"playUrl":"https://cdn.example.com/v.mp4?a=1","playToken":"&token=2"}`,
			wantURL: "https://cdn.example.com/v.mp4?a=1&token=2",
		},
		{
			name:    "ampersand token without query",
			body:    `{"videoUrl":"https://cdn.example.com/v.mp4","authKey":"&k=3"}`,
			wantURL: "https://cdn.example.com/v.mp4?k=3",
		},
		{
			name:    "nested pair",
			body:    `{"data":{"mediaUrl":"https://cdn.example.com/v.mp4","mediaUrlToken":"?sig=4"}}`,
			wantURL: "https://cdn.example.com/v.mp4?sig=4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMediaURLsFromJSON([]byte(tt.body))
			if len(got) != 1 {
				t.Fatalf("candidates = %#v, want exactly the joined URL", got)
			}
			if got[0].URL != tt.wantURL || got[0].Kind != "video" || got[0].Suffix != ".mp4" {
				t.Fatalf("candidate = %#v, want %q", got[0], tt.wantURL)
			}
		})
	}
}

func TestExtractMediaURLsFromJSONProvisionalStrongFields(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantKind   string
		wantSuffix string
	}{
		{name: "video field without suffix", body: `{"videoAddress":"https://cdn.example.com/play/abc123"}`, wantKind: "video", wantSuffix: ".mp4"},
		{name: "stream field without suffix", body: `{"streamUrl":"https://live.example.com/channel/9"}`, wantKind: "m3u8", wantSuffix: ".m3u8"},
		{name: "play field without suffix", body: `{"mediaPlay":"https://cdn.example.com/play/x"}`, wantKind: "video", wantSuffix: ".mp4"},
		{name: "cover field without suffix skipped", body: `{"cover":"https://cdn.example.com/img/abc"}`},
		{name: "weak field without suffix skipped", body: `{"homepage":"https://example.com/about"}`},
		{name: "segment still excluded", body: `{"videoUrl":"https://cdn.example.com/a/1.ts"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMediaURLsFromJSON([]byte(tt.body))
			if tt.wantKind == "" {
				if len(got) != 0 {
					t.Fatalf("candidates = %#v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Kind != tt.wantKind || got[0].Suffix != tt.wantSuffix || !got[0].Provisional {
				t.Fatalf("candidates = %#v, want provisional %s", got, tt.wantKind)
			}
		})
	}
}

func TestCandidateProcessJSONTags(t *testing.T) {
	data, err := json.Marshal(Candidate{SourceAppID: "wx456", Attribution: AttributionUnattributed, Provisional: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, want := range []string{`"source_app_id":"wx456"`, `"attribution":"unattributed"`, `"provisional":true`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("marshaled candidate = %q, want %q", encoded, want)
		}
	}
}

func TestUnattributedGenerationInvalidatesOnlyOnClear(t *testing.T) {
	store := NewStore()
	generation := store.UnattributedGeneration()
	store.SetTarget(Target{AppID: "wx123"})
	candidate := Candidate{URL: "https://cdn.example.com/v.mp4", Kind: "video"}
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); !ok {
		t.Fatal("target selection discarded first-page response")
	}
	store.Clear("wx123")
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); ok {
		t.Fatal("old response repopulated cleared list")
	}
	generation = store.UnattributedGeneration()
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); !ok {
		t.Fatal("new response after clear was discarded")
	}
	store.ClearUnattributed()
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); ok {
		t.Fatal("old response survived ClearUnattributed")
	}
}
