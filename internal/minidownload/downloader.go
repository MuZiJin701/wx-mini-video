package minidownload

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// ErrRecordingStopped 表示用户停止直播录制；已录内容已保存为最终文件。
var ErrRecordingStopped = errors.New("已停止直播录制")

// ErrRecordingInterrupted 表示直播在非用户停止的情况下中断（如断网、unexpected EOF）；
// 已录内容已尽量保存为最终文件，中断原因附在错误信息中。
var ErrRecordingInterrupted = errors.New("直播流意外中断")

// ErrDownloadStopped 表示用户停止普通下载；未完成内容保留在 .part 文件，便于之后续传。
var ErrDownloadStopped = errors.New("已停止下载")

// isContextCancellation 只匹配真正的上下文取消/超时，用于把“用户停止”与保存失败区分开。
func isContextCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

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
	Live       bool
	StartedAt  time.Time
	LastUpdate time.Time
	Error      string
}

type DownloadHistory struct {
	Path         string    `json:"path"`
	AppID        string    `json:"app_id"`
	SourceAppID  string    `json:"source_app_id"`
	Kind         string    `json:"kind"`
	Attribution  string    `json:"attribution"`
	SourceDomain string    `json:"source_domain"`
	Size         int64     `json:"size"`
	CompletedAt  time.Time `json:"completed_at"`
}

type Downloader struct {
	Dir        string
	FFmpegPath string
	Client     *http.Client
	OnProgress func(Progress)
	// OnLiveResolved 在下载前已知直播、以及下载后按实际响应修正直播状态时回调，
	// 供界面按候选身份回写 Live 标记；candidateID 是候选 ID，live 是最终判定。
	OnLiveResolved     func(candidateID string, live bool)
	SegmentConcurrency int
	SegmentRetries     int
	KeepSegments       bool
	DownloadMode       string
	historyMu          sync.Mutex
}

func New(dir string) *Downloader {
	return &Downloader{
		Dir:                dir,
		FFmpegPath:         "",
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

func (d *Downloader) do(req *http.Request) (*http.Response, error) {
	client := *d.Client
	originalRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if originalRedirect != nil {
			if err := originalRedirect(next, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return fmt.Errorf("重定向次数过多")
		}
		headers := make(map[string]string)
		for key := range next.Header {
			headers[key] = next.Header.Get(key)
		}
		next.Header = make(http.Header)
		for key, value := range miniprogram.HeadersForURL(headers, via[len(via)-1].URL.String(), next.URL.String()) {
			next.Header.Set(key, value)
		}
		return nil
	}
	return client.Do(req)
}

// now 是测试注入点：默认文件名里的下载开始时间统一从这里获取。
var now = time.Now

func OutputName(candidate miniprogram.Candidate, preferred string) string {
	name := strings.TrimSpace(preferred)
	if name == "" {
		name = defaultOutputName(candidate, now())
	}
	ext := strings.ToLower(filepath.Ext(name))
	if miniprogram.IsM3U8(candidate) {
		if ext != "" {
			name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		return safeOutputName(name + ".mp4")
	}
	suffix := candidate.Suffix
	if suffix == "" {
		_, suffix = miniprogram.Classify("", candidate.URL)
	}
	if suffix == "" && miniprogram.MediaKind(candidate) == "live" {
		suffix = ".flv"
	}
	if suffix != "" && !strings.EqualFold(filepath.Ext(name), suffix) {
		name += suffix
	}
	return safeOutputName(name)
}

// defaultOutputName 按「类型_YYYYMMDD-HHMMSS_候选ID前8位.扩展名」生成默认文件名。
// 类型前缀取最终类型目录名：直播录制为 直播_，有限 FLV 为 视频_，分段媒体为 分段媒体_。
func defaultOutputName(candidate miniprogram.Candidate, startedAt time.Time) string {
	kind := miniprogram.MediaKind(candidate)
	suffix := ".mp4"
	if kind != "m3u8" {
		suffix = candidate.Suffix
		if suffix == "" {
			_, suffix = miniprogram.Classify("", candidate.URL)
		}
		if suffix == "" && kind == "live" {
			suffix = ".flv"
		}
		if suffix == "" {
			suffix = ".bin"
		}
	}
	token := shortCandidateID(candidate.ID)
	if token == "" {
		token = "copy"
	}
	return miniprogram.TypeDir(kind) + "_" + startedAt.Format("20060102-150405") + "_" + token + suffix
}

func safeOutputName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '-'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "wx-mini-video"
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		return "_" + name
	}
	return name
}

func shortCandidateID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 8 {
		return id[:8]
	}
	return id
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

func (d *Downloader) Download(ctx context.Context, candidate miniprogram.Candidate, preferredName, targetAppID string) (string, error) {
	if d.Client == nil {
		d.Client = defaultHTTPClient()
	}
	if d.Dir == "" {
		d.Dir = "."
	}
	if err := os.MkdirAll(d.Dir, 0o755); err != nil {
		return "", fmt.Errorf("创建下载根目录 %s 失败: %w", d.Dir, err)
	}
	// 下载前已知的直播标记先回写：界面可以立即按“直播”更新该候选的展示与分类。
	if candidate.Live {
		d.notifyLiveResolved(candidate.ID, true)
	}
	subdir := miniprogram.MediaSubdir(candidate, targetAppID)
	if !filepath.IsLocal(subdir) {
		return "", fmt.Errorf("下载子目录不安全: %q", subdir)
	}
	bucket := filepath.Join(d.Dir, subdir)
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		return "", fmt.Errorf("创建下载目录 %s 失败: %w", bucket, err)
	}
	name := priorRecordingName(bucket, candidate, preferredName)
	// 成品已存在时不再复用遗留 .part，避免覆盖已有成品；此时走新路径并保留旧 .part。
	if name != "" && fileExists(filepath.Join(bucket, name)) {
		name = ""
	}
	var targetPath string
	if name != "" {
		// 续传：直接使用遗留 .part 的基名，不能再经过成品重命名的二次改名。
		targetPath = filepath.Join(bucket, name)
	} else {
		name, targetPath = availableOutputPath(bucket, OutputName(candidate, preferredName), candidate, strings.TrimSpace(preferredName) == "")
	}
	initialKind := miniprogram.MediaKind(candidate)
	isM3U8 := miniprogram.IsM3U8(candidate)
	d.emit(Progress{ID: candidate.ID, Name: name, Path: targetPath, Status: "starting", Total: candidate.ContentLength, Live: candidate.Live})
	var live bool
	var err error
	if isM3U8 {
		err = d.downloadM3U8(ctx, candidate, targetPath)
	} else {
		live, err = d.downloadDirect(ctx, candidate, targetPath)
	}
	// 按实际响应把成品归入最终类型目录：直播录音进 直播/，确认是有限文件的 FLV 回到 视频/。
	// .part 仍留在初始目录以保持续传语义，这里只处理已提交的成品。
	finalKind := resolvedKind(initialKind, isM3U8, live)
	targetPath, name = d.finalizeOutputPath(candidate, preferredName, targetAppID, targetPath, name, initialKind, finalKind)
	// 只有下载真正拿到响应并完成（含直播停止/断流保存）时才回写修正；
	// 请求失败或用户提前取消时无法确认实际类型，保留候选原有标记。
	resolved := err == nil || errors.Is(err, ErrRecordingStopped) || errors.Is(err, ErrRecordingInterrupted)
	if !isM3U8 && resolved && live != candidate.Live {
		// 下载阶段确认的直播状态与候选原有标记不一致时回写，界面按候选身份保持选择。
		d.notifyLiveResolved(candidate.ID, live)
	}
	// 只有真正的取消错误才折算为“已停止”；保存/重命名等失败必须原样暴露给用户。
	if err != nil && ctx.Err() != nil && isContextCancellation(err) &&
		!errors.Is(err, ErrRecordingStopped) && !errors.Is(err, ErrDownloadStopped) && !errors.Is(err, ErrRecordingInterrupted) {
		err = ErrDownloadStopped
	}
	if err == nil || errors.Is(err, ErrRecordingStopped) || errors.Is(err, ErrRecordingInterrupted) {
		_ = d.appendHistory(candidate, targetPath, targetAppID, finalKind)
	}
	d.finishProgress(candidate, name, targetPath, live, finalKind, err)
	return targetPath, err
}

// resolvedKind 合并初始分类与下载阶段的实际结果：只有视频/直播之间需要互相修正，
// 图片、音频、分段媒体与其他文件保持原分类。
func resolvedKind(initialKind string, isM3U8 bool, live bool) string {
	if isM3U8 {
		return "m3u8"
	}
	switch initialKind {
	case "live":
		if !live {
			return "video"
		}
		return "live"
	case "video":
		if live {
			return "live"
		}
	}
	return initialKind
}

func (d *Downloader) notifyLiveResolved(candidateID string, live bool) {
	if d.OnLiveResolved == nil || strings.TrimSpace(candidateID) == "" {
		return
	}
	d.OnLiveResolved(candidateID, live)
}

func (d *Downloader) finishProgress(candidate miniprogram.Candidate, name, outputPath string, live bool, finalKind string, err error) {
	status := "done"
	errText := ""
	downloadedPath := outputPath
	switch {
	case err == nil:
	case errors.Is(err, ErrRecordingStopped):
		status = "stopped"
	case errors.Is(err, ErrRecordingInterrupted):
		status = "interrupted"
		errText = err.Error()
	case errors.Is(err, ErrDownloadStopped):
		status = "stopped"
		if !fileExists(outputPath) {
			downloadedPath = partialPath(outputPath, candidate)
		}
	default:
		status = "error"
		errText = err.Error()
		if !fileExists(outputPath) {
			downloadedPath = partialPath(outputPath, candidate)
		}
	}
	total := candidate.ContentLength
	if err == nil && finalKind != "m3u8" {
		total = fileSize(outputPath)
	}
	d.emit(Progress{
		ID:         candidate.ID,
		Name:       name,
		Path:       outputPath,
		Status:     status,
		Downloaded: fileSize(downloadedPath),
		Total:      total,
		Live:       live,
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
	bucket := filepath.Dir(outputPath)
	inputURL := candidate.CachedPath
	source := "缓存的本地文件"
	if inputURL == "" || !fileExists(inputURL) {
		d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: "cache-m3u8"})
		localPath, err := d.cacheM3U8FromURL(ctx, candidate, bucket)
		if err != nil {
			if ctx.Err() != nil {
				return ErrDownloadStopped
			}
			return fmt.Errorf("没有拿到 m3u8 本地缓存，远程 playlist 也已不可用(%v)。请重新播放目标媒体后重试", err)
		}
		inputURL = localPath
		source = "下载到本地文件"
	}
	if _, err := os.Stat(d.FFmpegPath); err != nil {
		return fmt.Errorf("未找到 ffmpeg（路径不可用），无法下载 m3u8 视频；请配置 download.ffmpeg 路径或使用受管 ffmpeg")
	}
	playlistBody, err := os.ReadFile(inputURL)
	if err != nil {
		return fmt.Errorf("读取 m3u8 缓存失败: %w", err)
	}
	playlistBody, err = rewriteRemotePlaylist(playlistBody, candidate.URL, nil)
	if err != nil {
		return err
	}
	totalDuration := parseM3U8Duration(playlistBody)
	segmentStats := segmentStats{}
	if d.chooseM3U8Mode(playlistBody) == "prefetch" && !bytes.Contains(playlistBody, []byte("#EXT-X-STREAM-INF")) && !bytes.Contains(playlistBody, []byte("#EXT-X-MEDIA:")) {
		localPlaylist, stats, err := d.localizeM3U8(ctx, bucket, candidate.ID, playlistBody, candidate.Headers, candidate.URL)
		if err != nil {
			if ctx.Err() != nil {
				return ErrDownloadStopped
			}
			return err
		}
		inputURL = localPlaylist
		segmentStats = stats
	} else {
		remoteInput, closeServer, err := d.servePlaylist(ctx, candidate, playlistBody)
		if err != nil {
			if ctx.Err() != nil {
				return ErrDownloadStopped
			}
			return err
		}
		defer closeServer()
		inputURL = remoteInput
	}
	d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: "ffmpeg", Total: totalDuration})
	err = d.runFFmpeg(ctx, candidate, inputURL, outputPath, totalDuration)
	if err != nil {
		if ctx.Err() != nil {
			return ErrDownloadStopped
		}
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
	headers := candidate.Headers
	if isRemoteInput(inputURL) {
		headers = miniprogram.HeadersForURL(headers, candidate.URL, inputURL)
	}
	args := BuildFFmpegArgs(inputURL, outputPath, headers, true)
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
	errText := redactFFmpegOutput(strings.TrimSpace(output.String()), candidate.URL, candidate.Headers)
	logPath := ffmpegLogPath(d.Dir, candidate.ID)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("%s", extractFFmpegError(errText))
	}
	if writeErr := os.WriteFile(logPath, []byte(errText), 0o644); writeErr == nil {
		return fmt.Errorf("完整日志: %s\n%s", logPath, extractFFmpegError(errText))
	}
	return fmt.Errorf("%s", extractFFmpegError(errText))
}

func ffmpegLogPath(dir, candidateID string) string {
	name := safeOutputName(strings.TrimSpace(candidateID))
	if name == "" || name == "wx-mini-video" {
		name = "ffmpeg"
	}
	return filepath.Join(dir, ".ffmpeg", name+".log")
}

func redactFFmpegOutput(output, resourceURL string, headers map[string]string) string {
	if parsed, err := url.Parse(resourceURL); err == nil && parsed.RawQuery != "" {
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		output = strings.ReplaceAll(output, resourceURL, parsed.String())
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		output = strings.ReplaceAll(output, key+": "+value, "<REDACTED HEADER>")
		output = strings.ReplaceAll(output, value, "<REDACTED>")
	}
	return output
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

// segmentMetadata records that a localized m3u8 resource finished downloading.
// It is written only after the bytes are renamed into place, so a truncated
// file left behind by an interrupted run can never pass for a finished one.
type segmentMetadata struct {
	URL         string    `json:"url"`
	Size        int64     `json:"size"`
	CompletedAt time.Time `json:"completed_at"`
}

func segmentPartPath(path string) string {
	return path + ".part"
}

func segmentDonePath(path string) string {
	return path + ".done"
}

// segmentComplete reports whether path holds the fully downloaded body of
// rawURL. Reuse requires the sidecar metadata, a matching source URL, and a
// matching file size; legacy files without metadata are always re-downloaded.
func segmentComplete(path, rawURL string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(segmentDonePath(path))
	if err != nil {
		return false
	}
	var meta segmentMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return false
	}
	return meta.URL == rawURL && meta.Size == info.Size()
}

func writeSegmentMetadata(path, rawURL string, size int64) error {
	data, err := json.Marshal(segmentMetadata{URL: rawURL, Size: size, CompletedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	metaPath := segmentDonePath(path)
	tempPath := metaPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tempPath, metaPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

type m3u8Resource struct {
	URL       string
	LocalName string
}

func (d *Downloader) localizeM3U8(ctx context.Context, bucket, candidateID string, body []byte, headers map[string]string, sourceURL string) (string, segmentStats, error) {
	if candidateID == "" {
		candidateID = fmt.Sprintf("m3u8_%d", time.Now().UnixNano())
	}
	dir := filepath.Join(bucket, ".segments", candidateID, miniprogram.CandidateID("", string(body)))
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
				if segmentComplete(outputPath, resource.URL) {
					size := fileSize(outputPath)
					currentDone := int(atomic.AddInt32(&completed, 1))
					currentReused := int(atomic.AddInt32(&reused, 1))
					currentBytes := atomic.AddInt64(&downloadedBytes, size)
					d.emit(Progress{Status: "segments", Completed: currentDone, Items: len(resources), Reused: currentReused, Downloaded: currentBytes, StartedAt: startedAt})
					continue
				}
				// Any file that is not proven complete must not be reused: drop
				// stale bytes and half-written metadata before re-downloading.
				_ = os.Remove(outputPath)
				_ = os.Remove(segmentDonePath(outputPath))
				if err := d.downloadResourceWithRetry(ctx, resource.URL, outputPath, miniprogram.HeadersForURL(headers, sourceURL, resource.URL), retries); err != nil {
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
	partPath := segmentPartPath(outputPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := d.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	file, err := os.Create(partPath)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(partPath)
		if resp.ContentLength >= 0 && written != resp.ContentLength {
			return fmt.Errorf("分片正文长度与 Content-Length 不符: 收到 %d 字节，声明 %d 字节: %w", written, resp.ContentLength, copyErr)
		}
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(partPath)
		return closeErr
	}
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		_ = os.Remove(partPath)
		return fmt.Errorf("分片正文长度与 Content-Length 不符: 收到 %d 字节，声明 %d 字节", written, resp.ContentLength)
	}
	if err := os.Rename(partPath, outputPath); err != nil {
		_ = os.Remove(partPath)
		return err
	}
	if err := writeSegmentMetadata(outputPath, rawURL, written); err != nil {
		_ = os.Remove(outputPath)
		return err
	}
	return nil
}

func (d *Downloader) cacheM3U8FromURL(ctx context.Context, candidate miniprogram.Candidate, bucket string) (string, error) {
	body, err := d.fetchM3U8(ctx, candidate, candidate.Headers)
	if err != nil && strings.Contains(err.Error(), "HTTP 304") {
		body, err = d.fetchM3U8(ctx, candidate, stripConditionalHeaders(candidate.Headers))
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(bucket, 0o755); err != nil {
		return "", fmt.Errorf("创建 m3u8 缓存目录 %s 失败: %w", bucket, err)
	}
	localPath := filepath.Join(bucket, candidate.ID+".m3u8")
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
	resp, err := d.do(req)
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

type partialDownload struct {
	ETag  string `json:"etag,omitempty"`
	Total int64  `json:"total"`
}

func partialPath(outputPath string, candidate miniprogram.Candidate) string {
	return outputPath + partialSuffix(candidate)
}

// partialSuffix 是带候选标识的部分下载后缀，用于在目录里反查同一候选的遗留 .part。
func partialSuffix(candidate miniprogram.Candidate) string {
	return "." + miniprogram.CandidateID(candidate.AppID, candidate.URL) + ".part"
}

// priorRecordingName 在默认命名路径下复用同一候选遗留的 .part 基名，保证秒级重试仍能续传。
// 指定 preferredName 时沿用显式命名；疑似直播（含 .flv 地址）不复用，避免覆盖上一次未保存的录制。
func priorRecordingName(bucket string, candidate miniprogram.Candidate, preferredName string) string {
	if strings.TrimSpace(preferredName) != "" || candidate.Live || miniprogram.IsFLVURL(candidate.URL) {
		return ""
	}
	suffix := partialSuffix(candidate)
	entries, err := os.ReadDir(bucket)
	if err != nil {
		return ""
	}
	best := ""
	var bestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), suffix)
		if name == "" || safeOutputName(name) != name {
			continue
		}
		// 按实际内容判断：FLV 魔数说明这是未保存的直播录制，绝不能复用后再被 O_TRUNC 清空。
		if !partReusable(filepath.Join(bucket, entry.Name())) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best = name
			bestTime = info.ModTime()
		}
	}
	return best
}

// partReusable 判断遗留 .part 是否可以安全复用：空文件或普通下载可复用；
// 以 FLV 魔数开头的是直播录制，复用后会覆盖未保存内容；无法读取时同样不复用。
func partReusable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 3)
	n, _ := io.ReadFull(file, head)
	if n < 3 {
		return true
	}
	return !bytes.HasPrefix(head, []byte("FLV"))
}

// availableOutputPath 在一次循环里同时避开已存在的成品与同候选遗留的 .part，
// 避免选名后又因成品重名二次改名、撞上未保存录制的 .part。
// avoidPartFirst 为 true 时（默认命名）首个候选也检查 .part；指定文件名时保留首个候选的续传语义。
func availableOutputPath(bucket, name string, candidate miniprogram.Candidate, avoidPartFirst bool) (string, string) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidateName := name
		if i > 1 {
			candidateName = fmt.Sprintf("%s_%d%s", stem, i, ext)
		}
		final := filepath.Join(bucket, candidateName)
		if fileExists(final) {
			continue
		}
		if (avoidPartFirst || i > 1) && fileExists(final+partialSuffix(candidate)) {
			continue
		}
		return candidateName, final
	}
}

// finalizeOutputPath 按下载阶段确认的实际类型把已提交的成品移动到最终类型目录，
// 并按默认命名规则修正类型前缀与直播所需的 .flv 后缀。
// 只处理已存在的成品：普通下载被停止或失败时成品尚未提交，.part 仍留在初始目录以保持续传语义。
// 跨目录调整先选一个不冲突的名称，绝不覆盖目标目录里已有的成品或同候选未保存录制。
func (d *Downloader) finalizeOutputPath(candidate miniprogram.Candidate, preferredName, targetAppID, targetPath, currentName, initialKind, finalKind string) (string, string) {
	if !fileExists(targetPath) {
		return targetPath, currentName
	}
	initialBucket := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)
	finalBucket := initialBucket
	if finalKind != initialKind {
		adjusted := candidate
		adjusted.Live = finalKind == "live"
		finalBucket = filepath.Join(d.Dir, miniprogram.MediaSubdir(adjusted, targetAppID))
	}
	name := base
	if finalKind != initialKind && strings.TrimSpace(preferredName) == "" {
		name = renameTypePrefix(name, initialKind, finalKind)
	}
	if finalKind == "live" && !strings.EqualFold(filepath.Ext(name), ".flv") {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".flv"
	}
	if finalBucket == initialBucket && name == base {
		return targetPath, currentName
	}
	if finalBucket != initialBucket {
		if err := os.MkdirAll(finalBucket, 0o755); err != nil {
			return targetPath, currentName
		}
	}
	finalName, finalPath := availableOutputPath(finalBucket, name, candidate, true)
	if finalPath == targetPath {
		return targetPath, currentName
	}
	if err := os.Rename(targetPath, finalPath); err != nil {
		// 移动失败时保留已下载内容在初始目录，避免因分类整理丢失成品。
		return targetPath, currentName
	}
	return finalPath, finalName
}

// renameTypePrefix 把默认文件名开头的类型前缀从初始类型改为最终类型（直播_ ↔ 视频_）。
func renameTypePrefix(name, initialKind, finalKind string) string {
	oldPrefix := miniprogram.TypeDir(initialKind) + "_"
	if !strings.HasPrefix(name, oldPrefix) {
		return name
	}
	return miniprogram.TypeDir(finalKind) + "_" + strings.TrimPrefix(name, oldPrefix)
}

func (p partialDownload) validator() string {
	if len(p.ETag) >= 2 && strings.HasPrefix(p.ETag, `"`) && strings.HasSuffix(p.ETag, `"`) && !strings.Contains(p.ETag[1:len(p.ETag)-1], `"`) {
		return p.ETag
	}
	return ""
}

// sniffLiveFLV 在响应体开头确认 FLV 魔数，判断是否按直播录制处理。
// 只有没有 Content-Length（分块传输）且地址或响应类型指向 FLV 时才嗅探，
// 避免把普通分块响应误判成直播；嗅探不会消费正文。
func sniffLiveFLV(candidate miniprogram.Candidate, resp *http.Response) bool {
	if resp.ContentLength >= 0 {
		return false
	}
	contentType := resp.Header.Get("Content-Type")
	if !miniprogram.IsFLVContentType(contentType) && !miniprogram.IsFLVURL(candidate.URL) {
		return false
	}
	buffered := bufio.NewReaderSize(resp.Body, 4096)
	resp.Body = struct {
		io.Reader
		io.Closer
	}{buffered, resp.Body}
	head, err := buffered.Peek(4)
	if err != nil && len(head) < 3 {
		return false
	}
	return bytes.HasPrefix(head, []byte("FLV"))
}

// commitRecording 把已录制的 .part 保存为最终文件；没有内容时清理临时文件。
func commitRecording(partPath, metaPath, outputPath string, written int64) error {
	if written <= 0 {
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
		return nil
	}
	if err := commitPartFile(partPath, outputPath); err != nil {
		return err
	}
	_ = os.Remove(metaPath)
	return nil
}

func (d *Downloader) downloadDirect(ctx context.Context, candidate miniprogram.Candidate, outputPath string) (bool, error) {
	partPath := partialPath(outputPath, candidate)
	metaPath := partPath + ".json"
	// 直播流没有总长度、也不能续传：疑似直播时直接从头录制，避免对直播地址发 Range 请求。
	maybeLive := candidate.Live || miniprogram.IsFLVURL(candidate.URL)
	var previous partialDownload
	data, err := os.ReadFile(metaPath)
	offset := int64(0)
	if !maybeLive && err == nil && json.Unmarshal(data, &previous) == nil && previous.validator() != "" {
		offset = fileSize(partPath)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.URL, nil)
	if err != nil {
		return false, err
	}
	for key, value := range candidate.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Del("Range")
	req.Header.Del("If-Range")
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		req.Header.Set("If-Range", previous.validator())
	}
	resp, err := d.do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ErrDownloadStopped
		}
		return false, err
	}
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && offset > 0 {
		resp.Body.Close()
		req.Header.Del("Range")
		req.Header.Del("If-Range")
		offset = 0
		resp, err = d.do(req)
		if err != nil {
			if ctx.Err() != nil {
				return false, ErrDownloadStopped
			}
			return false, err
		}
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return false, fmt.Errorf("下载失败，HTTP %s", resp.Status)
	}
	live := sniffLiveFLV(candidate, resp)
	defer resp.Body.Close()
	total := resp.ContentLength
	expectedBytes := resp.ContentLength
	resumed := offset > 0 && resp.StatusCode == http.StatusPartialContent
	if live {
		// 直播流没有总长度，也没有“下载完成”的校验：持续录制到用户停止或服务端结束。
		total = -1
		expectedBytes = -1
		resumed = false
		offset = 0
	}
	if resp.StatusCode == http.StatusPartialContent {
		start, end, full, valid := miniprogram.ParseContentRange(resp.Header.Get("Content-Range"))
		expectedStart := int64(0)
		if resumed {
			expectedStart = offset
		}
		if !valid || start != expectedStart || (resp.ContentLength >= 0 && resp.ContentLength != end-start+1) {
			return false, fmt.Errorf("服务器返回了无效的 Content-Range")
		}
		total = full
		expectedBytes = end - start + 1
		if resumed && ((previous.Total > 0 && previous.Total != full) || (resp.Header.Get("ETag") != "" && previous.ETag != resp.Header.Get("ETag"))) {
			_ = os.Remove(metaPath)
			return false, fmt.Errorf("资源已变化，无法安全续传，请重试以重新下载")
		}
	}
	if !resumed {
		offset = 0
	}
	metadata := partialDownload{ETag: resp.Header.Get("ETag"), Total: total}
	if resumed && metadata.validator() == "" {
		metadata = previous
		metadata.Total = total
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	// 先清除旧的续传依据，避免文件已重写但元数据仍指向旧实体。
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	flags := os.O_CREATE | os.O_WRONLY
	if resumed {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(partPath, flags, 0o600)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(metaPath, meta, 0o600); err != nil {
		file.Close()
		return false, err
	}
	reader := &progressReader{Reader: resp.Body, Total: total, Current: offset, OnProgress: func(downloaded, total int64) {
		status := "downloading"
		if live {
			status = "recording"
		}
		d.emit(Progress{ID: candidate.ID, Path: outputPath, Status: status, Downloaded: downloaded, Total: total, Live: live})
	}}
	written, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if live {
		if copyErr != nil {
			// 用户停止录制：提交已录内容；保存失败要单独暴露，不能伪装成“已停止”。
			if ctx.Err() != nil {
				if written == 0 {
					_ = os.Remove(partPath)
					_ = os.Remove(metaPath)
					return true, fmt.Errorf("直播流没有返回数据")
				}
				if err := commitRecording(partPath, metaPath, outputPath, written); err != nil {
					return true, fmt.Errorf("已停止录制，但保存已录内容失败: %w", err)
				}
				return true, ErrRecordingStopped
			}
			// 异常断流（断网、unexpected EOF 等）：先保全已录内容，再报告中断原因。
			if written > 0 {
				if err := commitRecording(partPath, metaPath, outputPath, written); err != nil {
					return true, fmt.Errorf("直播流中断（%v），且保存已录内容失败: %w", copyErr, err)
				}
				return true, fmt.Errorf("%w：%v", ErrRecordingInterrupted, copyErr)
			}
			return true, copyErr
		}
		if closeErr != nil {
			return true, closeErr
		}
		if written == 0 {
			_ = os.Remove(partPath)
			_ = os.Remove(metaPath)
			return true, fmt.Errorf("直播流没有返回数据")
		}
		// 服务端结束或断开直播：把已录制内容保存为最终文件。
		if err := commitRecording(partPath, metaPath, outputPath, written); err != nil {
			return true, err
		}
		return true, nil
	}
	if copyErr != nil {
		if ctx.Err() != nil {
			return false, ErrDownloadStopped
		}
		return false, copyErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	if expectedBytes >= 0 && written != expectedBytes {
		return false, fmt.Errorf("响应正文长度与声明不符，请重试")
	}
	if total >= 0 && offset+written != total {
		return false, fmt.Errorf("文件尚未完整下载（%d/%d 字节），请重试", offset+written, total)
	}
	if err := commitPartFile(partPath, outputPath); err != nil {
		return false, err
	}
	_ = os.Remove(metaPath)
	return false, nil
}
func commitPartFile(partPath, outputPath string) error {
	if err := os.Rename(partPath, outputPath); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return err
	}
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(partPath, outputPath)
}

func (d *Downloader) appendHistory(candidate miniprogram.Candidate, outputPath, targetAppID, kind string) error {
	sourceDomain := ""
	if parsed, err := url.Parse(candidate.URL); err == nil {
		sourceDomain = parsed.Hostname()
	}
	archiveAppID := strings.TrimSpace(candidate.AppID)
	if archiveAppID == "" {
		archiveAppID = strings.TrimSpace(targetAppID)
	}
	record := DownloadHistory{
		Path:         outputPath,
		AppID:        archiveAppID,
		SourceAppID:  candidate.SourceAppID,
		Kind:         kind,
		Attribution:  candidate.Attribution,
		SourceDomain: sourceDomain,
		Size:         fileSize(outputPath),
		CompletedAt:  time.Now().UTC(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	d.historyMu.Lock()
	defer d.historyMu.Unlock()
	file, err := os.OpenFile(filepath.Join(d.Dir, "history.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
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
