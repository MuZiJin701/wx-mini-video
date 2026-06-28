package minidownload

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wx_channel/internal/miniprogram"
)

type Progress struct {
	ID         string
	Name       string
	Path       string
	Status     string
	Downloaded int64
	Total      int64
	Completed  int
	Items      int
	Failed     int
	Reused     int
	StartedAt  time.Time
	LastUpdate time.Time
	Error      string
}

type Downloader struct {
	Dir                string
	FFmpegPath         string
	Client             *http.Client
	OnProgress         func(Progress)
	SegmentConcurrency int
	SegmentRetries     int
	KeepSegments       bool
	DownloadMode       string
}

func New(dir string) *Downloader {
	return &Downloader{
		Dir:                dir,
		FFmpegPath:         "ffmpeg",
		Client:             defaultHTTPClient(),
		SegmentConcurrency: 6,
		SegmentRetries:     3,
		DownloadMode:       "auto",
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			MaxIdleConns:        128,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func OutputName(candidate miniprogram.Candidate, preferred string) string {
	name := strings.TrimSpace(preferred)
	if name == "" {
		name = candidateName(candidate)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if candidate.Kind == "m3u8" {
		if ext != "" {
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		return name + ".mp4"
	}
	suffix := candidate.Suffix
	if suffix == "" {
		_, suffix = miniprogram.Classify("", candidate.URL)
	}
	if suffix != "" && !strings.EqualFold(filepath.Ext(name), suffix) {
		name += suffix
	}
	return name
}

func candidateName(candidate miniprogram.Candidate) string {
	if candidate.AppName != "" && candidate.ID != "" {
		n := 8
		if len(candidate.ID) < n {
			n = len(candidate.ID)
		}
		return candidate.AppName + "_" + candidate.ID[:n]
	}
	if u, err := url.Parse(candidate.URL); err == nil {
		base := filepath.Base(u.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return fmt.Sprintf("wx-mini-video_%d", time.Now().Unix())
}

func BuildFFmpegArgs(inputURL, outputPath string, headers map[string]string, overwrite bool) []string {
	args := make([]string, 0, 8)
	if overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	remoteInput := isRemoteInput(inputURL)
	if headerText := ffmpegHeaderText(headers); headerText != "" && remoteInput {
		args = append(args, "-headers", headerText)
	}
	if remoteInput {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_delay_max", "5", "-http_persistent", "0")
	}
	args = append(args, "-protocol_whitelist", "file,http,https,tcp,tls,crypto,data")
	args = append(args, "-i", inputURL, "-c", "copy", outputPath)
	return args
}

func isRemoteInput(inputURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(inputURL))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func ffmpegHeaderText(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(headers[key]))
		b.WriteString("\r\n")
	}
	return b.String()
}

func stripConditionalHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		switch lower {
		case "if-match", "if-modified-since", "if-none-match", "if-range", "if-unmodified-since", "range":
			continue
		default:
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *Downloader) Download(ctx context.Context, candidate miniprogram.Candidate, preferredName string) (string, error) {
	if d.Client == nil {
		d.Client = defaultHTTPClient()
	}
	if d.FFmpegPath == "" {
		d.FFmpegPath = "ffmpeg"
	}
	if d.Dir == "" {
		d.Dir = "."
	}
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return "", err
	}
	name := OutputName(candidate, preferredName)
	outputPath := filepath.Join(d.Dir, name)
	d.emit(Progress{ID: candidate.ID, Name: name, Path: outputPath, Status: "starting", Total: candidate.ContentLength})
	if candidate.Kind == "m3u8" || strings.EqualFold(filepath.Ext(candidate.URL), ".m3u8") {
		err := d.downloadM3U8(ctx, candidate, outputPath)
		d.finishProgress(candidate, name, outputPath, err)
		return outputPath, err
	}
	err := d.downloadDirect(ctx, candidate, outputPath)
	d.finishProgress(candidate, name, outputPath, err)
	return outputPath, err
}

func (d *Downloader) finishProgress(candidate miniprogram.Candidate, name, outputPath string, err error) {
	status := "done"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	d.emit(Progress{
		ID:         candidate.ID,
		Name:       name,
		Path:       outputPath,
		Status:     status,
		Downloaded: fileSize(outputPath),
		Total:      candidate.ContentLength,
		Error:      errText,
	})
}

func (d *Downloader) emit(p Progress) {
	if p.LastUpdate.IsZero() {
		p.LastUpdate = time.Now()
	}
	if d.OnProgress != nil {
		d.OnProgress(p)
	}
}

func (d *Downloader) downloadM3U8(ctx context.Context, candidate miniprogram.Candidate, outputPath string) error {
	inputURL := candidate.CachedPath
	source := "缓存的本地文件"
	if inputURL == "" || !fileExists(inputURL) {
		d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: "cache-m3u8"})
		localPath, err := d.cacheM3U8FromURL(ctx, candidate)
		if err != nil {
			return fmt.Errorf("没有拿到 m3u8 本地缓存，远程 playlist 也已不可用(%v)。请按 c 清空候选后重新播放目标视频，等待列表中出现 cache 标记后再下载", err)
		}
		inputURL = localPath
		source = "下载到本地文件"
	}
	if _, err := exec.LookPath(d.FFmpegPath); err != nil {
		return fmt.Errorf("未找到 ffmpeg，无法下载 m3u8 视频；请安装 ffmpeg 或配置 download.ffmpeg 路径")
	}
	playlistBody, err := os.ReadFile(inputURL)
	if err != nil {
		return fmt.Errorf("读取 m3u8 缓存失败: %w", err)
	}
	totalDuration := parseM3U8Duration(playlistBody)
	segmentStats := segmentStats{}
	if d.chooseM3U8Mode(playlistBody) == "prefetch" {
		localPlaylist, stats, err := d.localizeM3U8(ctx, candidate.ID, playlistBody, candidate.Headers)
		if err != nil {
			return err
		}
		inputURL = localPlaylist
		segmentStats = stats
	}
	d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: "ffmpeg", Total: totalDuration})
	err = d.runFFmpeg(ctx, candidate, inputURL, outputPath, totalDuration)
	if err != nil {
		return fmt.Errorf("ffmpeg 合并失败(%s): %w", source, err)
	}
	if !d.KeepSegments && segmentStats.Dir != "" {
		_ = os.RemoveAll(segmentStats.Dir)
	}
	return nil
}

func (d *Downloader) chooseM3U8Mode(body []byte) string {
	switch strings.ToLower(strings.TrimSpace(d.DownloadMode)) {
	case "prefetch":
		return "prefetch"
	case "remote-ffmpeg":
		return "remote-ffmpeg"
	default:
		if countM3U8Segments(body) >= 20 {
			return "prefetch"
		}
		return "remote-ffmpeg"
	}
}

func (d *Downloader) runFFmpeg(ctx context.Context, candidate miniprogram.Candidate, inputURL, outputPath string, totalDuration int64) error {
	args := BuildFFmpegArgs(inputURL, outputPath, candidate.Headers, true)
	args = append([]string{"-stats_period", "1"}, args...)
	cmd := exec.CommandContext(ctx, d.FFmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stderr)
	scanner.Split(scanFFmpegProgress)
	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line)
		output.WriteByte('\n')
		if current, ok := parseFFmpegProgressTime(line); ok {
			d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: "ffmpeg", Downloaded: current, Total: totalDuration})
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		output.WriteString(scanErr.Error())
		output.WriteByte('\n')
	}
	err = cmd.Wait()
	if err == nil {
		return nil
	}
	errText := strings.TrimSpace(output.String())
	logPath := outputPath + ".ffmpeg.log"
	if writeErr := os.WriteFile(logPath, []byte(errText), 0o644); writeErr == nil {
		return fmt.Errorf("完整日志: %s\n%s", logPath, extractFFmpegError(errText))
	}
	return fmt.Errorf("%s", extractFFmpegError(errText))
}

func scanFFmpegProgress(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			if i == 0 {
				return 1, nil, nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func extractFFmpegError(output string) string {
	lines := strings.Split(output, "\n")
	var relevant []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "ffmpeg version") || strings.Contains(trimmed, "built with") || strings.Contains(trimmed, "configuration:") || strings.Contains(trimmed, "libav") {
			continue
		}
		relevant = append(relevant, trimmed)
	}
	start := 0
	if len(relevant) > 6 {
		start = len(relevant) - 6
	}
	return strings.Join(relevant[start:], "\n")
}

var extinfRegexp = regexp.MustCompile(`(?m)^#EXTINF:([0-9]+(?:\.[0-9]+)?)`)

func parseM3U8Duration(body []byte) int64 {
	matches := extinfRegexp.FindAllSubmatch(body, -1)
	var total float64
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		value, err := strconv.ParseFloat(string(match[1]), 64)
		if err == nil {
			total += value
		}
	}
	return int64(total * 1000)
}

var ffmpegTimeRegexp = regexp.MustCompile(`time=(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?`)

func parseFFmpegProgressTime(line string) (int64, bool) {
	match := ffmpegTimeRegexp.FindStringSubmatch(line)
	if len(match) == 0 {
		return 0, false
	}
	hours, _ := strconv.Atoi(match[1])
	minutes, _ := strconv.Atoi(match[2])
	seconds, _ := strconv.Atoi(match[3])
	ms := 0
	if match[4] != "" {
		frac := match[4]
		if len(frac) > 3 {
			frac = frac[:3]
		}
		for len(frac) < 3 {
			frac += "0"
		}
		ms, _ = strconv.Atoi(frac)
	}
	return int64((((hours*60)+minutes)*60+seconds)*1000 + ms), true
}

type segmentStats struct {
	Dir       string
	Total     int
	Completed int
	Reused    int
	Failed    int
	Bytes     int64
	StartedAt time.Time
	Elapsed   time.Duration
}

type m3u8Resource struct {
	URL       string
	LocalName string
}

func (d *Downloader) localizeM3U8(ctx context.Context, candidateID string, body []byte, headers map[string]string) (string, segmentStats, error) {
	if candidateID == "" {
		candidateID = fmt.Sprintf("m3u8_%d", time.Now().UnixNano())
	}
	dir := filepath.Join(d.Dir, ".segments", candidateID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", segmentStats{Dir: dir}, err
	}
	rewritten, resources := collectM3U8Resources(body)
	startedAt := time.Now()
	stats := segmentStats{Dir: dir, Total: len(resources), StartedAt: startedAt}
	if len(resources) == 0 {
		localPath := filepath.Join(dir, "playlist.m3u8")
		if err := os.WriteFile(localPath, body, 0o644); err != nil {
			return "", stats, err
		}
		return localPath, stats, nil
	}

	concurrency := d.SegmentConcurrency
	if concurrency <= 0 {
		concurrency = 6
	}
	retries := d.SegmentRetries
	if retries <= 0 {
		retries = 3
	}
	var completed int32
	var reused int32
	var failed int32
	var downloadedBytes int64
	jobs := make(chan m3u8Resource)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for resource := range jobs {
				outputPath := filepath.Join(dir, resource.LocalName)
				if fileSize(outputPath) > 0 {
					currentDone := int(atomic.AddInt32(&completed, 1))
					currentReused := int(atomic.AddInt32(&reused, 1))
					currentBytes := atomic.AddInt64(&downloadedBytes, fileSize(outputPath))
					d.emit(Progress{Status: "segments", Completed: currentDone, Items: len(resources), Reused: currentReused, Downloaded: currentBytes, StartedAt: startedAt})
					continue
				}
				if err := d.downloadResourceWithRetry(ctx, resource.URL, outputPath, headers, retries); err != nil {
					currentFailed := int(atomic.AddInt32(&failed, 1))
					d.emit(Progress{Status: "segments", Completed: int(atomic.LoadInt32(&completed)), Items: len(resources), Failed: currentFailed, Reused: int(atomic.LoadInt32(&reused)), Downloaded: atomic.LoadInt64(&downloadedBytes), StartedAt: startedAt})
					select {
					case errCh <- err:
					default:
					}
					continue
				}
				size := fileSize(outputPath)
				currentBytes := atomic.AddInt64(&downloadedBytes, size)
				currentDone := int(atomic.AddInt32(&completed, 1))
				d.emit(Progress{Status: "segments", Completed: currentDone, Items: len(resources), Failed: int(atomic.LoadInt32(&failed)), Reused: int(atomic.LoadInt32(&reused)), Downloaded: currentBytes, StartedAt: startedAt})
			}
		}()
	}
	for _, resource := range resources {
		select {
		case err := <-errCh:
			close(jobs)
			wg.Wait()
			return "", stats, err
		case jobs <- resource:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return "", stats, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		stats.Completed = int(atomic.LoadInt32(&completed))
		stats.Reused = int(atomic.LoadInt32(&reused))
		stats.Failed = int(atomic.LoadInt32(&failed))
		stats.Bytes = atomic.LoadInt64(&downloadedBytes)
		stats.Elapsed = time.Since(startedAt)
		return "", stats, err
	default:
	}
	stats.Completed = int(completed)
	stats.Reused = int(reused)
	stats.Failed = int(failed)
	stats.Bytes = downloadedBytes
	stats.Elapsed = time.Since(startedAt)
	localPath := filepath.Join(dir, "playlist.m3u8")
	if err := os.WriteFile(localPath, []byte(rewritten), 0o644); err != nil {
		return "", stats, err
	}
	return localPath, stats, nil
}

func countM3U8Segments(body []byte) int {
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		if isRemoteInput(strings.TrimSpace(line)) {
			count++
		}
	}
	return count
}

func collectM3U8Resources(body []byte) (string, []m3u8Resource) {
	lines := strings.Split(string(body), "\n")
	resources := make([]m3u8Resource, 0)
	var out strings.Builder
	uriIndex := 0
	segmentIndex := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			rewritten := m3u8URIAttrRegexp.ReplaceAllStringFunc(line, func(match string) string {
				parts := m3u8URIAttrRegexp.FindStringSubmatch(match)
				if len(parts) != 2 || !isRemoteInput(parts[1]) {
					return match
				}
				uriIndex++
				name := fmt.Sprintf("res_%06d%s", uriIndex, safeResourceExt(parts[1], ".bin"))
				resources = append(resources, m3u8Resource{URL: parts[1], LocalName: name})
				return `URI="` + filepath.ToSlash(name) + `"`
			})
			out.WriteString(rewritten)
			out.WriteString("\n")
			continue
		}
		if isRemoteInput(trimmed) {
			segmentIndex++
			name := fmt.Sprintf("seg_%06d%s", segmentIndex, safeResourceExt(trimmed, ".ts"))
			resources = append(resources, m3u8Resource{URL: trimmed, LocalName: name})
			out.WriteString(filepath.ToSlash(name))
			out.WriteString("\n")
			continue
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n"), resources
}

func safeResourceExt(rawURL, fallback string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fallback
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	if ext == "" || len(ext) > 8 {
		return fallback
	}
	return ext
}

func (d *Downloader) downloadResourceWithRetry(ctx context.Context, rawURL, outputPath string, headers map[string]string, retries int) error {
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		if err := d.downloadResource(ctx, rawURL, outputPath, headers); err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		return nil
	}
	return fmt.Errorf("分片下载失败: %s (%v)", rawURL, lastErr)
}

func (d *Downloader) downloadResource(ctx context.Context, rawURL, outputPath string, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		os.Remove(outputPath)
		return err
	}
	return file.Close()
}

func (d *Downloader) cacheM3U8FromURL(ctx context.Context, candidate miniprogram.Candidate) (string, error) {
	body, err := d.fetchM3U8(ctx, candidate, candidate.Headers)
	if err != nil && strings.Contains(err.Error(), "HTTP 304") {
		body, err = d.fetchM3U8(ctx, candidate, stripConditionalHeaders(candidate.Headers))
	}
	if err != nil {
		return "", err
	}
	localPath := filepath.Join(d.Dir, candidate.ID+".m3u8")
	if err := os.WriteFile(localPath, rewriteM3U8URLs(body, candidate.URL), 0o644); err != nil {
		return "", err
	}
	return localPath, nil
}

func (d *Downloader) fetchM3U8(ctx context.Context, candidate miniprogram.Candidate, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func rewriteM3U8URLs(body []byte, baseURL string) []byte {
	base, err := url.Parse(baseURL)
	if err != nil {
		return body
	}
	lines := strings.Split(string(body), "\n")
	var out strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			out.WriteString(rewriteM3U8TagLine(line, base))
			out.WriteString("\n")
			continue
		}
		if trimmed == "" {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
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
		out.WriteString(resolveM3U8Reference(base, rel).String())
		out.WriteString("\n")
	}
	return []byte(strings.TrimRight(out.String(), "\n"))
}

var m3u8URIAttrRegexp = regexp.MustCompile(`URI="([^"]+)"`)

func rewriteM3U8TagLine(line string, base *url.URL) string {
	return m3u8URIAttrRegexp.ReplaceAllStringFunc(line, func(match string) string {
		parts := m3u8URIAttrRegexp.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		value := strings.TrimSpace(parts[1])
		if value == "" || strings.HasPrefix(value, "data:") {
			return match
		}
		rel, err := url.Parse(value)
		if err != nil {
			return match
		}
		return `URI="` + resolveM3U8Reference(base, rel).String() + `"`
	})
}

func resolveM3U8Reference(base *url.URL, rel *url.URL) *url.URL {
	resolved := base.ResolveReference(rel)
	if resolved.RawQuery == "" && rel.RawQuery == "" && base.RawQuery != "" {
		clone := *resolved
		clone.RawQuery = base.RawQuery
		return &clone
	}
	return resolved
}

func (d *Downloader) downloadDirect(ctx context.Context, candidate miniprogram.Candidate, outputPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return err
	}
	for key, value := range candidate.Headers {
		req.Header.Set(key, value)
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败，HTTP %s", resp.Status)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	total := candidate.ContentLength
	if total <= 0 {
		total = resp.ContentLength
	}
	reader := &progressReader{
		Reader: resp.Body,
		Total:  total,
		OnProgress: func(downloaded int64, total int64) {
			d.emit(Progress{
				ID:         candidate.ID,
				Path:       outputPath,
				Status:     "downloading",
				Downloaded: downloaded,
				Total:      total,
			})
		},
	}
	_, err = io.Copy(file, reader)
	return err
}

type progressReader struct {
	Reader     io.Reader
	Total      int64
	Current    int64
	OnProgress func(downloaded int64, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.Reader.Read(b)
	p.Current += int64(n)
	if n > 0 && p.OnProgress != nil {
		p.OnProgress(p.Current, p.Total)
	}
	return n, err
}

func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
