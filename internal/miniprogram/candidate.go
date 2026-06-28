package miniprogram

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Target struct {
	Name       string `json:"name"`
	AppID      string `json:"app_id"`
	OriginalID string `json:"original_id"`
	LinkPrefix string `json:"link_prefix"`
}

type Candidate struct {
	ID            string            `json:"id"`
	AppID         string            `json:"app_id"`
	AppName       string            `json:"app_name"`
	URL           string            `json:"url"`
	SourceURL     string            `json:"source_url"`
	Source        string            `json:"source"`
	FieldPath     string            `json:"field_path,omitempty"`
	ContentType   string            `json:"content_type"`
	ContentLength int64             `json:"content_length"`
	Kind          string            `json:"kind"`
	Suffix        string            `json:"suffix"`
	Headers       map[string]string `json:"headers,omitempty"`
	CachedPath    string            `json:"cached_path,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

type Store struct {
	mu               sync.RWMutex
	items            map[string]Candidate
	order            []string
	OnCandidateAdded func(Candidate)
}

func NewStore() *Store {
	return &Store{
		items: make(map[string]Candidate),
	}
}

func (s *Store) Add(candidate Candidate) (Candidate, bool) {
	if candidate.URL == "" {
		return candidate, false
	}
	if candidate.ID == "" {
		candidate.ID = CandidateID(candidate.AppID, candidate.URL)
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	s.mu.Lock()
	if existing, exists := s.items[candidate.ID]; exists {
		changed := false
		if candidate.CachedPath != "" {
			existing.CachedPath = candidate.CachedPath
			changed = true
		}
		if candidate.ContentLength > 0 && existing.ContentLength == 0 {
			existing.ContentLength = candidate.ContentLength
			changed = true
		}
		if candidate.ContentType != "" && existing.ContentType == "" {
			existing.ContentType = candidate.ContentType
			changed = true
		}
		if len(candidate.Headers) > 0 && len(existing.Headers) == 0 {
			existing.Headers = candidate.Headers
			changed = true
		}
		if changed {
			s.items[candidate.ID] = existing
		}
		s.mu.Unlock()
		return existing, changed
	}
	s.items[candidate.ID] = candidate
	s.order = append(s.order, candidate.ID)
	s.mu.Unlock()
	if s.OnCandidateAdded != nil {
		s.OnCandidateAdded(candidate)
	}
	return candidate, true
}

func (s *Store) List(appID string) []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Candidate
	for _, id := range s.order {
		item, ok := s.items[id]
		if !ok {
			continue
		}
		if appID != "" && item.AppID != appID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (s *Store) Get(id string) (Candidate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

func (s *Store) SetCachedPath(id, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return
	}
	item.CachedPath = path
	s.items[id] = item
}

func (s *Store) Clear(appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if appID == "" {
		s.items = make(map[string]Candidate)
		s.order = nil
		return
	}
	filtered := s.order[:0]
	for _, id := range s.order {
		item, ok := s.items[id]
		if !ok {
			continue
		}
		if item.AppID == appID {
			delete(s.items, id)
			continue
		}
		filtered = append(filtered, id)
	}
	s.order = filtered
}

func CandidateID(appID, rawURL string) string {
	h := sha1.Sum([]byte(appID + "\n" + rawURL))
	return hex.EncodeToString(h[:])
}

func ClassifyByContentType(contentType string) (kind, suffix string) {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	switch mediaType {
	case "image/jpeg":
		return "image", ".jpg"
	case "image/png":
		return "image", ".png"
	case "image/webp":
		return "image", ".webp"
	case "image/gif":
		return "image", ".gif"
	case "image/bmp":
		return "image", ".bmp"
	case "image/avif":
		return "image", ".avif"
	case "video/mp4":
		return "video", ".mp4"
	case "video/webm":
		return "video", ".webm"
	case "video/quicktime":
		return "video", ".mov"
	case "application/vnd.apple.mpegurl", "application/x-mpegurl", "audio/x-mpegurl", "application/x-mpeg":
		return "m3u8", ".m3u8"
	case "video/mp2t":
		return "segment", ".ts"
	default:
		return "", ""
	}
}

func ClassifyByURL(rawURL string) (kind, suffix string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	ext := strings.ToLower(path.Ext(u.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
		if ext == ".jpeg" {
			return "image", ".jpg"
		}
		return "image", ext
	case ".mp4", ".webm", ".mov":
		return "video", ext
	case ".m3u8":
		return "m3u8", ".m3u8"
	case ".ts":
		return "segment", ".ts"
	default:
		return "", ""
	}
}

func Classify(contentType, rawURL string) (kind, suffix string) {
	if kind, suffix = ClassifyByContentType(contentType); kind != "" {
		return kind, suffix
	}
	return ClassifyByURL(rawURL)
}

func CleanHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	forbidden := map[string]struct{}{
		"accept-encoding":     {},
		"connection":          {},
		"content-length":      {},
		"host":                {},
		"if-match":            {},
		"if-modified-since":   {},
		"if-none-match":       {},
		"if-range":            {},
		"if-unmodified-since": {},
		"keep-alive":          {},
		"proxy-connection":    {},
		"range":               {},
		"transfer-encoding":   {},
	}
	clean := make(map[string]string)
	for key, values := range header {
		if _, ok := forbidden[strings.ToLower(key)]; ok || len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[0])
		if value == "" {
			continue
		}
		clean[http.CanonicalHeaderKey(key)] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

var mediaURLRegexp = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

func ExtractMediaURLsFromJSON(body []byte) []Candidate {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil
	}
	seen := make(map[string]Candidate)
	walkJSON("", v, seen)
	out := make([]Candidate, 0, len(seen))
	for _, candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URL == out[j].URL {
			return out[i].FieldPath < out[j].FieldPath
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func walkJSON(fieldPath string, v any, seen map[string]Candidate) {
	switch value := v.(type) {
	case map[string]any:
		for k, child := range value {
			nextPath := k
			if fieldPath != "" {
				nextPath = fieldPath + "." + k
			}
			walkJSON(nextPath, child, seen)
		}
	case []any:
		for i, child := range value {
			nextPath := fieldPath + "[]"
			if fieldPath == "" {
				nextPath = "[]"
			}
			_ = i
			walkJSON(nextPath, child, seen)
		}
	case string:
		key := strings.ToLower(fieldPath)
		fieldLooksRelevant := looksLikeMediaField(key)
		for _, match := range mediaURLRegexp.FindAllString(value, -1) {
			kind, suffix := Classify("", strings.TrimRight(match, ".,);]"))
			if kind == "" || !fieldLooksRelevant && kind == "segment" {
				continue
			}
			urlValue := strings.TrimRight(match, ".,);]")
			seen[urlValue+"|"+fieldPath] = Candidate{
				URL:       urlValue,
				Source:    "json",
				FieldPath: fieldPath,
				Kind:      kind,
				Suffix:    suffix,
			}
		}
	}
}

func looksLikeMediaField(key string) bool {
	if key == "" {
		return false
	}
	for _, token := range []string{
		"url", "src", "m3u8", "mp4", "media", "video",
		"image", "img", "pic", "photo", "cover", "poster",
		"avatar", "thumb", "thumbnail", "banner", "logo",
	} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}
