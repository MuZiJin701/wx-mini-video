package interceptor

import (
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

const (
	debugEnvVar          = "WX_MINI_VIDEO_DEBUG"
	debugLogFileName     = "wx-mini-video-debug.log"
	unknownBinaryMinSize = 256 * 1024
)

var m3u8Client = &http.Client{Timeout: 0}

// CreateMiniProgramInterceptorPlugin 构建小程序媒体嗅探插件。
// showUnattributed 缺省为 true；显式传 false 时不收集来源未确认的媒体。
func CreateMiniProgramInterceptorPlugin(target miniprogram.Target, store *miniprogram.Store, downloadDir string, showUnattributed ...bool) *proxy.Plugin {
	captureUnattributed := true
	if len(showUnattributed) > 0 {
		captureUnattributed = showUnattributed[0]
	}
	if store != nil {
		store.SetTarget(target)
	}
	dir := downloadDir
	if dir == "" {
		dir = "."
	}
	debugLog := newDebugLog(dir)
	return &proxy.Plugin{
		Match: "",
		OnResponse: func(ctx proxy.Context) {
			if store == nil {
				return
			}
			req := ctx.Req()
			if req == nil || req.URL == nil {
				return
			}
			rawURL := contextURLString(req.URL)
			appID := requestAppID(req.Header)
			target, generation := store.Observe(appID)
			contentType := ctx.GetResponseHeader("Content-Type")
			contentLength := responseContentLength(ctx)
			status := responseStatus(ctx)
			domKind, domSuffix := miniprogram.Classify(contentType, rawURL)
			headers := miniprogram.CleanHeaders(req.Header)
			decision := ""
			jsonCandidates := 0
			confirmed := appID != "" && appID == target.AppID

			switch {
			case rawURL == "":
				decision = "drop:no-url"
			case domKind == "segment":
				decision = "drop:segment"
			case domKind != "":
				if confirmed {
					addTargetResponseCandidate(store, dir, generation, target, appID, rawURL, contentType, contentLength, headers, domKind, domSuffix, ctx)
					decision = "admit-target"
				} else if target.AppID != "" {
					if !captureUnattributed {
						decision = "drop:disabled"
						break
					}
					store.AddUnattributed(miniprogram.Candidate{
						SourceAppID:   appID,
						Attribution:   miniprogram.AttributionUnattributed,
						URL:           rawURL,
						SourceURL:     rawURL,
						Source:        "response",
						ContentType:   contentType,
						ContentLength: contentLength,
						Kind:          domKind,
						Suffix:        domSuffix,
						Headers:       headers,
					})
					decision = "admit-unattributed"
				} else {
					decision = "drop:no-target"
				}
			case isJSONContentType(contentType):
				if confirmed {
					jsonCandidates = addJSONCandidates(store, generation, target, appID, rawURL, headers, ctx)
					if jsonCandidates > 0 {
						decision = "admit-target"
					} else {
						decision = "drop:not-media"
					}
				} else {
					decision = "drop:not-media"
				}
			case confirmed:
				// 未知二进制兜底：只有目标 AppID 匹配时才收未知文件，避免全量流量噪音。
				fileDecision := tryUnknownBinaryCandidate(store, generation, target, appID, rawURL, status, contentType, contentLength, headers, ctx.GetResponseHeader("Content-Disposition"))
				decision = fileDecision
			case target.AppID == "":
				decision = "drop:no-target"
			default:
				decision = "drop:not-media"
			}

			debugLog.logResponse(debugResponse{
				Method:         req.Method,
				URL:            sanitizeURLForLog(rawURL),
				Status:         status,
				ContentType:    contentType,
				ContentLength:  contentLength,
				Referer:        refererForLog(req.Header),
				AppID:          appID,
				TargetAppID:    target.AppID,
				Kind:           domKind,
				Decision:       decision,
				JSONCandidates: jsonCandidates,
			})
		},
	}
}

func addTargetResponseCandidate(store *miniprogram.Store, dir string, generation uint64, target miniprogram.Target, sourceAppID, rawURL, contentType string, contentLength int64, headers map[string]string, kind, suffix string, ctx proxy.Context) {
	candidate := miniprogram.Candidate{
		AppID:         target.AppID,
		AppName:       target.Name,
		SourceAppID:   sourceAppID,
		Attribution:   miniprogram.AttributionConfirmed,
		URL:           rawURL,
		SourceURL:     rawURL,
		Source:        "response",
		ContentType:   contentType,
		ContentLength: contentLength,
		Kind:          kind,
		Suffix:        suffix,
		Headers:       headers,
	}
	candidate.ID = miniprogram.CandidateID(target.AppID, candidate.URL)
	var cache func() string
	if kind == "m3u8" {
		// 只有归属候选才读取/缓存 playlist；未归属 m3u8 不写盘。
		body := readProxyBody(ctx)
		if len(body) == 0 && store.IsCurrentTarget(generation) {
			body = fetchM3U8HTTP(candidate)
		}
		cache = func() string { return cacheM3U8(body, dir, candidate) }
	}
	_, _ = store.AddForTarget(generation, candidate, cache)
}

func addJSONCandidates(store *miniprogram.Store, generation uint64, target miniprogram.Target, sourceAppID, sourceURL string, headers map[string]string, ctx proxy.Context) int {
	body, err := ctx.GetResponseBody()
	if err != nil || len(body) == 0 || len(body) > 4*1024*1024 {
		return 0
	}
	ctx.SetResponseBody(string(body))
	added := 0
	for _, candidate := range miniprogram.ExtractMediaURLsFromJSON(body) {
		if candidate.Kind == "m3u8" || candidate.Kind == "segment" {
			continue
		}
		candidate.SourceURL = sourceURL
		candidate.Headers = miniprogram.HeadersForURL(headers, sourceURL, candidate.URL)
		candidate.AppID = target.AppID
		candidate.AppName = target.Name
		candidate.SourceAppID = sourceAppID
		candidate.Attribution = miniprogram.AttributionConfirmed
		_, _ = store.AddForTarget(generation, candidate, nil)
		added++
	}
	return added
}

// tryUnknownBinaryCandidate 在目标归属成立时把满足大小阈值的未知二进制收为 "file" 候选。
// 返回写入 debug 日志的决定；不缓存、不自动下载。
func tryUnknownBinaryCandidate(store *miniprogram.Store, generation uint64, target miniprogram.Target, sourceAppID, rawURL string, status int, contentType string, contentLength int64, headers map[string]string, contentDisposition string) string {
	if status != http.StatusOK && status != http.StatusPartialContent {
		return "drop:status"
	}
	if !unknownBinaryContentType(contentType) {
		return "drop:not-media"
	}
	if contentLength < unknownBinaryMinSize {
		return "drop:small-file"
	}
	suffix := unknownBinarySuffix(contentDisposition, rawURL)
	candidate := miniprogram.Candidate{
		AppID:         target.AppID,
		AppName:       target.Name,
		SourceAppID:   sourceAppID,
		Attribution:   miniprogram.AttributionConfirmed,
		URL:           rawURL,
		SourceURL:     rawURL,
		Source:        "response",
		ContentType:   contentType,
		ContentLength: contentLength,
		Kind:          "file",
		Suffix:        suffix,
		Headers:       headers,
	}
	candidate.ID = miniprogram.CandidateID(target.AppID, candidate.URL)
	_, _ = store.AddForTarget(generation, candidate, nil)
	return "admit-target"
}

func isJSONContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json")
}

// unknownBinaryContentType 判断 Content-Type 是否属于未知二进制兜底范围。
// text/*、JSON、JavaScript、WASM、字体与 SVG 一律排除；无法识别的类型按未知二进制处理。
func unknownBinaryContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	if mediaType == "" {
		return true
	}
	switch mediaType {
	case "application/octet-stream", "binary/octet-stream", "application/binary", "application/download", "application/force-download", "application/x-download":
		return true
	}
	for _, excluded := range []string{"text/", "json", "javascript", "wasm", "font", "svg", "xml", "html", "css"} {
		if strings.Contains(mediaType, excluded) {
			return false
		}
	}
	return true
}

// unknownBinarySuffix 依次从 Content-Disposition 文件名和 URL 推断后缀，兜底 .bin。
func unknownBinarySuffix(contentDisposition, rawURL string) string {
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if name := strings.TrimSpace(params["filename"]); name != "" {
				if suffix := sanitizeSuffix(filepath.Ext(name)); suffix != "" {
					return suffix
				}
			}
		}
	}
	if u, err := url.Parse(rawURL); err == nil {
		if suffix := sanitizeSuffix(path.Ext(u.Path)); suffix != "" {
			return suffix
		}
	}
	return ".bin"
}

func sanitizeSuffix(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if len(ext) < 2 || len(ext) > 12 || ext[0] != '.' {
		return ""
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
}

func responseStatus(ctx proxy.Context) int {
	if response := ctx.Res(); response != nil {
		return response.StatusCode
	}
	return 0
}

// responseContentLength 返回实体总大小：Content-Range 优先，分段响应没有总长时返回 0。
func responseContentLength(ctx proxy.Context) int64 {
	if rawRange := ctx.GetResponseHeader("Content-Range"); rawRange != "" {
		_, _, total, ok := miniprogram.ParseContentRange(rawRange)
		if !ok {
			return 0
		}
		return total
	}
	if response := ctx.Res(); response != nil && response.StatusCode == http.StatusPartialContent {
		return 0
	}
	return parseContentLength(ctx.GetResponseHeader("Content-Length"))
}

func requestAppID(header http.Header) string {
	values := header.Values("Referer")
	if len(values) != 1 {
		return ""
	}
	u, err := url.Parse(values[0])
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "servicewechat.com") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 4 || parts[0] != "" || parts[1] == "" || parts[2] == "" || parts[3] != "page-frame.html" || strings.ContainsAny(parts[1]+parts[2], " \t\r\n\\") {
		return ""
	}
	return parts[1]
}

func cacheM3U8(body []byte, dir string, candidate miniprogram.Candidate) string {
	if len(body) == 0 {
		return ""
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	localPath := filepath.Join(dir, candidate.ID+".m3u8")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		return ""
	}
	return localPath
}

func readProxyBody(ctx proxy.Context) []byte {
	body, err := ctx.GetResponseBody()
	if err == nil && len(body) > 0 {
		ctx.SetResponseBody(string(body))
		return body
	}
	return nil
}

func fetchM3U8HTTP(candidate miniprogram.Candidate) []byte {
	req, err := http.NewRequest(http.MethodGet, candidate.URL, nil)
	if err != nil {
		return nil
	}
	for key, value := range candidate.Headers {
		req.Header.Set(key, value)
	}
	resp, err := m3u8Client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return nil
	}
	return body
}

func parseContentLength(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func contextURLString(u *proxy.ContextURL) string {
	if u.String != "" && strings.HasPrefix(u.String, "http") {
		return u.String
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	host := u.Host
	if host == "" && u.Hostname != nil {
		host = u.Hostname()
	}
	if host == "" {
		return ""
	}
	out := &url.URL{Scheme: scheme, Host: host, Path: u.Path, RawQuery: u.RawQuery}
	return out.String()
}

type debugLog struct {
	mu   sync.Mutex
	path string
}

type debugResponse struct {
	Method         string
	URL            string
	Status         int
	ContentType    string
	ContentLength  int64
	Referer        string
	AppID          string
	TargetAppID    string
	Kind           string
	Decision       string
	JSONCandidates int
}

// newDebugLog 仅在 WX_MINI_VIDEO_DEBUG=1 时启用，日志写入下载目录。
func newDebugLog(dir string) *debugLog {
	if strings.TrimSpace(os.Getenv(debugEnvVar)) != "1" {
		return nil
	}
	if dir == "" {
		return nil
	}
	return &debugLog{path: filepath.Join(dir, debugLogFileName)}
}

func (l *debugLog) logResponse(entry debugResponse) {
	if l == nil {
		return
	}
	status := "-"
	if entry.Status > 0 {
		status = strconv.Itoa(entry.Status)
	}
	fields := []string{
		"method=" + logValue(entry.Method),
		"url=" + logValue(entry.URL),
		"status=" + status,
		"content-type=" + logValue(entry.ContentType),
		"content-length=" + strconv.FormatInt(entry.ContentLength, 10),
		"referer=" + logValue(entry.Referer),
		"app-id=" + logValue(entry.AppID),
		"target-app-id=" + logValue(entry.TargetAppID),
		"kind=" + logValue(entry.Kind),
		"decision=" + logValue(entry.Decision),
	}
	if entry.JSONCandidates > 0 {
		fields = append(fields, "json-candidates="+strconv.Itoa(entry.JSONCandidates))
	}
	l.write(strings.Join(fields, " "))
}

// write 追加一行诊断日志；任何错误都被忽略，避免影响代理主流程。
func (l *debugLog) write(line string) {
	if l == nil || l.path == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(time.Now().Format(time.RFC3339) + " " + line + "\n")
}

func logValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if strings.ContainsAny(value, " \t\r\n\"") {
		return strconv.Quote(value)
	}
	return value
}

// sanitizeURLForLog 只保留 scheme+host+path，去掉 query/fragment/userinfo。
func sanitizeURLForLog(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.User = nil
	return u.String()
}

// refererForLog 同样去掉 Referer 的 query/fragment/userinfo。
func refererForLog(header http.Header) string {
	for _, value := range header.Values("Referer") {
		if u, err := url.Parse(value); err == nil && u.Host != "" {
			u.RawQuery = ""
			u.ForceQuery = false
			u.Fragment = ""
			u.User = nil
			return u.String()
		}
	}
	return ""
}
