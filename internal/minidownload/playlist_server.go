package minidownload

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"wx_channel/internal/miniprogram"
)

// 将已捕获的播放列表交给 ffmpeg，同时由 Go 按请求来源限定分片和子播放列表的鉴权头。
func (d *Downloader) servePlaylist(ctx context.Context, candidate miniprogram.Candidate, body []byte) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		listener.Close()
		return "", nil, err
	}
	prefix := "/" + hex.EncodeToString(tokenBytes) + "/"
	base := "http://" + listener.Addr().String()
	var mu sync.Mutex
	urls := make(map[string]string)
	register := func(rawURL string) string {
		key := prefix + miniprogram.CandidateID("", rawURL) + path.Ext(strings.SplitN(rawURL, "?", 2)[0])
		mu.Lock()
		urls[key] = rawURL
		mu.Unlock()
		return base + key
	}
	rootBody, err := rewriteRemotePlaylist(body, candidate.URL, register)
	if err != nil {
		listener.Close()
		return "", nil, err
	}
	rootPath := prefix + "playlist.m3u8"
	relayCtx, cancel := context.WithCancel(ctx)
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == rootPath {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write(rootBody)
			return
		}
		mu.Lock()
		rawURL, allowed := urls[r.URL.Path]
		mu.Unlock()
		if !allowed {
			http.NotFound(w, r)
			return
		}
		req, err := http.NewRequestWithContext(relayCtx, r.Method, rawURL, nil)
		if err != nil {
			http.Error(w, "媒体请求无效", http.StatusBadGateway)
			return
		}
		for key, value := range miniprogram.HeadersForURL(candidate.Headers, candidate.URL, rawURL) {
			req.Header.Set(key, value)
		}
		if value := r.Header.Get("Range"); value != "" && !strings.EqualFold(path.Ext(strings.SplitN(rawURL, "?", 2)[0]), ".m3u8") {
			req.Header.Set("Range", value)
		}
		resp, err := d.do(req)
		if err != nil {
			http.Error(w, "媒体获取失败", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		signature, _ := reader.Peek(7)
		fullPlaylist := resp.StatusCode == http.StatusOK
		if resp.StatusCode == http.StatusPartialContent {
			start, end, total, ok := miniprogram.ParseContentRange(resp.Header.Get("Content-Range"))
			fullPlaylist = ok && start == 0 && end+1 == total
		}
		if r.Method == http.MethodGet && string(signature) == "#EXTM3U" {
			if !fullPlaylist {
				http.Error(w, "需要完整的子播放列表", http.StatusBadGateway)
				return
			}
			data, err := io.ReadAll(io.LimitReader(reader, 4*1024*1024+1))
			if err != nil || len(data) > 4*1024*1024 {
				http.Error(w, "播放列表读取失败或过大", http.StatusBadGateway)
				return
			}
			data, err = rewriteRemotePlaylist(data, resp.Request.URL.String(), register)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write(data)
			return
		}
		for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if value := resp.Header.Get(key); value != "" {
				w.Header().Set(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, reader)
	})}
	go server.Serve(listener)
	return base + rootPath, func() { cancel(); server.Close() }, nil
}

func rewriteRemotePlaylist(data []byte, sourceURL string, replaceURL func(string) string) ([]byte, error) {
	lines := strings.Split(string(rewriteM3U8URLs(data, sourceURL)), "\n")
	var rewriteErr error
	replace := func(rawURL string) string {
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			rewriteErr = fmt.Errorf("播放列表含有不支持的资源地址")
			return ""
		}
		if replaceURL != nil {
			return replaceURL(rawURL)
		}
		return rawURL
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			lines[i] = m3u8URIAttrRegexp.ReplaceAllStringFunc(line, func(match string) string {
				parts := m3u8URIAttrRegexp.FindStringSubmatch(match)
				if strings.HasPrefix(parts[1], "data:") {
					return match
				}
				return `URI="` + replace(parts[1]) + `"`
			})
		} else {
			lines[i] = replace(trimmed)
		}
	}
	return []byte(strings.Join(lines, "\n")), rewriteErr
}
