package interceptor

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wx_channel/internal/interceptor/proxy"
	"wx_channel/internal/miniprogram"
)

var m3u8Client = &http.Client{Timeout: 0}

func CreateMiniProgramInterceptorPlugin(target miniprogram.Target, store *miniprogram.Store, downloadDir string) *proxy.Plugin {
	if store != nil {
		store.SetTarget(target)
	}
	dir := downloadDir
	if dir == "" {
		dir = "."
	}
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
			appID := requestAppID(req.Header)
			target, generation := store.Observe(appID)
			if appID == "" || appID != target.AppID {
				return
			}
			rawURL := contextURLString(req.URL)
			if rawURL == "" {
				return
			}
			contentType := ctx.GetResponseHeader("Content-Type")
			contentLength := parseContentLength(ctx.GetResponseHeader("Content-Length"))
			kind, suffix := miniprogram.Classify(contentType, rawURL)
			headers := miniprogram.CleanHeaders(req.Header)
			sourceURL := rawURL
			if kind != "" && kind != "segment" {
				candidate := miniprogram.Candidate{
					AppID:         target.AppID,
					AppName:       target.Name,
					URL:           rawURL,
					SourceURL:     sourceURL,
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
					body := readProxyBody(ctx)
					if len(body) == 0 && store.IsCurrentTarget(generation) {
						body = fetchM3U8HTTP(candidate)
					}
					cache = func() string { return cacheM3U8(body, dir, candidate) }
				}
				_, _ = store.AddForTarget(generation, candidate, cache)
			}
			if !strings.Contains(strings.ToLower(contentType), "json") {
				return
			}
			body, err := ctx.GetResponseBody()
			if err != nil || len(body) == 0 || len(body) > 4*1024*1024 {
				return
			}
			ctx.SetResponseBody(string(body))
			for _, candidate := range miniprogram.ExtractMediaURLsFromJSON(body) {
				if candidate.Kind == "m3u8" || candidate.Kind == "segment" {
					continue
				}
				candidate.SourceURL = sourceURL
				candidate.ContentType = contentType
				candidate.Headers = headers
				candidate.AppID = target.AppID
				candidate.AppName = target.Name
				_, _ = store.AddForTarget(generation, candidate, nil)
			}
		},
	}
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
	if err := os.WriteFile(localPath, rewriteRelativeSegments(body, candidate.URL), 0o644); err != nil {
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

func rewriteRelativeSegments(body []byte, baseURL string) []byte {
	base, err := url.Parse(baseURL)
	if err != nil {
		return body
	}
	lines := strings.Split(string(body), "\n")
	var out strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}
		rel, err := url.Parse(trimmed)
		if err != nil {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}
		out.WriteString(base.ResolveReference(rel).String())
		out.WriteString("\n")
	}
	return []byte(strings.TrimRight(out.String(), "\n"))
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
