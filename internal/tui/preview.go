//go:build preview

// 本文件只在 `go run -tags preview ./build/uipreview` 生成文档预览图时编译，
// 不进入正式二进制；它复用真实 View 渲染，避免手工伪造界面。
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"wx_channel/internal/app"
	"wx_channel/internal/config"
	"wx_channel/internal/minidownload"
	"wx_channel/internal/miniprogram"
	"wx_channel/internal/tui/ui"
)

// 预览在非 TTY 环境生成，必须强制真彩色输出，否则 lipgloss 会剥掉所有 SGR。
func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// PreviewKinds 是文档预览支持的界面名称。
var PreviewKinds = []string{"home", "discovered", "appearance", "media", "recording", "downloaded", "logs"}

// RenderPreview 渲染一个固定尺寸的界面预览。
func RenderPreview(kind string, width, height int) (string, error) {
	model, err := buildPreviewModel(kind, width, height)
	if err != nil {
		return "", err
	}
	return model.View(), nil
}

func buildPreviewModel(kind string, width, height int) (model, error) {
	if width <= 0 || height <= 0 {
		return model{}, fmt.Errorf("无效尺寸")
	}
	dir := "downloads"
	runtime := &app.Runtime{
		Settings: config.AppSettings{
			DownloadDir:   dir,
			ProxyHostname: "127.0.0.1",
			ProxyPort:     2023,
			Target:        miniprogram.Target{AppID: "wx6ab29f61bd262656"},
		},
		Store:      miniprogram.NewStore(),
		Downloader: minidownload.New(dir),
	}
	appearance := ui.AppearanceDark
	if kind == "home" {
		appearance = ui.AppearanceLight
	}
	m := model{
		runtime:         runtime,
		hits:            newHitState(),
		downloaded:      make(map[string]string),
		width:           width,
		height:          height,
		category:        categoryAll,
		appearance:      appearance,
		targetExpanded:  true,
		focus:           focusInput,
		appIDInput:      "wx6ab29f61bd262656",
		inputCursor:     len([]rune("wx6ab29f61bd262656")),
		selectedProgram: "wx6ab29f61bd262656",
		logs:            previewLogs(),
	}
	m.applyTheme()

	now := time.Now()
	programs := []miniprogram.Program{
		{AppID: "wx6ab29f61bd262656", FirstSeen: now.Add(-3 * time.Minute), LastSeen: now.Add(-12 * time.Second)},
		{AppID: "wx1d2e3f4051627384", FirstSeen: now.Add(-9 * time.Minute), LastSeen: now.Add(-2 * time.Minute)},
	}

	switch kind {
	case "home":
		m.starting = true
		m.appIDInput = ""
		m.inputCursor = 0
		m.selectedProgram = ""
	case "discovered":
		m.started = true
		m.programs = programs
		m.selectedProgram = programs[0].AppID
		m.focus = focusPrograms
		m.appIDInput = ""
		m.inputCursor = 0
	case "appearance":
		m.started = true
		m.programs = programs
		m.selectedProgram = programs[0].AppID
		m.appIDInput = ""
		m.inputCursor = 0
		m.appearanceOpen = true
		m.appearanceIdx = appearanceIndex(appearance)
	case "media":
		m.targetReady = true
		m.targetExpanded = false
		m.started = true
		m.candidates = previewCandidates()
		m.selected = previewLiveIndex(m.candidates)
		m.selectedID = m.candidates[m.selected].ID
		m.showDetails = true
	case "recording":
		m.targetReady = true
		m.targetExpanded = false
		m.started = true
		m.candidates = previewCandidates()
		m.selected = previewLiveIndex(m.candidates)
		m.selectedID = m.candidates[m.selected].ID
		m.downloading = true
		m.downloadLive = true
		m.downloadID = m.candidates[m.selected].ID
		m.progress = minidownload.Progress{
			ID:         m.downloadID,
			Status:     "recording",
			Downloaded: 96 * 1024 * 1024,
			Live:       true,
		}
	case "downloaded":
		m.targetReady = true
		m.targetExpanded = false
		m.started = true
		m.candidates = previewCandidates()
		m.selected = 0
		m.selectedID = m.candidates[0].ID
		m.downloaded[m.candidates[0].ID] = "downloads/wx6ab29f61bd262656/图片/图片_20261025-130512_9f8e7d6c.jpg"
		m.lastSavedPath = m.downloaded[m.candidates[0].ID]
		m.progress = minidownload.Progress{Status: "done", Path: m.lastSavedPath}
		m.setNotice("已保存 → "+m.relativeDownloadPath(m.lastSavedPath), time.Hour)
		m.showDetails = true
	case "logs":
		m.targetReady = true
		m.targetExpanded = false
		m.started = true
		m.candidates = previewCandidates()
		m.showLogs = true
	default:
		return model{}, fmt.Errorf("未知预览界面 %q", kind)
	}
	return m, nil
}

func previewLogs() []string {
	return []string{
		"13:00:01 自检通过: 下载目录可写",
		"13:00:01 自检通过: 代理端口可用 127.0.0.1:2023",
		"13:00:01 自检提示: 首次下载分段媒体时会准备受管 ffmpeg",
		"13:00:02 正在启动代理...",
		"13:00:03 代理已启动: 127.0.0.1:2023 (耗时 590ms)",
		"13:00:04 终端图片能力: sixel，像素格 10x20（品牌 logo 使用真图）",
		"13:00:12 已选定目标 wx6ab29f61bd262656，请重新打开图片或播放视频或音频",
		"13:00:31 检测到来源未确认媒体，已按类型并入列表（以 ? 标记）",
		"13:01:08 开始下载: 来源已确认 图片 甄选自营 · 封面",
		"13:01:12 下载完成: downloads/wx6ab29f61bd262656/图片/图片_20261025-130512_9f8e7d6c.jpg",
		"13:01:20 开始下载: 来源已确认 直播流 东方甄选直播 · 双11专场",
		"13:01:20 识别为直播流：会持续录制且不会自动结束，按 x 停止并保存",
		"13:02:41 已停止录制: downloads/wx6ab29f61bd262656/直播/直播_20261025-130241_00000003.flv",
	}
}

func previewCandidates() []miniprogram.Candidate {
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.Local)
	items := []struct {
		title  string
		kind   string
		source string
		size   int64
		attr   string
		live   bool
	}{
		{"甄选自营 · 封面", "image", "json", 1_258_291, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 限时补贴", "image", "json", 876_544, miniprogram.AttributionConfirmed, false},
		{"东方甄选直播 · 双11专场", "video", "json", 0, miniprogram.AttributionConfirmed, true},
		{"课程详情页视频", "video", "html", 13_002_291, miniprogram.AttributionUnattributed, false},
		{"甄选自营 · 肉禽海产", "image", "json", 1_572_664, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 时令果蔬", "image", "json", 1_310_720, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 乳品烘焙", "image", "json", 982_016, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 粮油干货", "image", "json", 1_048_576, miniprogram.AttributionConfirmed, false},
		{"宣传片", "m3u8", "response", 24_117_248, miniprogram.AttributionConfirmed, false},
		{"背景音乐", "audio", "json", 3_145_728, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 旅游度假", "image", "json", 1_703_936, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 日化美妆", "image", "json", 1_205_862, miniprogram.AttributionConfirmed, false},
		{"甄选自营 · 家纺家居", "image", "json", 1_454_080, miniprogram.AttributionConfirmed, false},
	}
	out := make([]miniprogram.Candidate, 0, len(items))
	for i, item := range items {
		url := fmt.Sprintf("https://cdn.example.com/media/%02d.%s", i+1, previewExt(item.kind, item.live))
		out = append(out, miniprogram.Candidate{
			ID:            fmt.Sprintf("%08x%032x", i+1, i+1),
			AppID:         "wx6ab29f61bd262656",
			Title:         item.title,
			URL:           url,
			Kind:          item.kind,
			Source:        item.source,
			ContentLength: item.size,
			Attribution:   item.attr,
			CreatedAt:     base.Add(time.Duration(i) * 17 * time.Second),
			SourceAppID:   sourceAppID(item.attr),
			Live:          item.live,
			Headers:       map[string]string{"Referer": "https://servicewechat.com/wx6ab29f61bd262656/12/page-frame.html", "User-Agent": "MicroMessenger"},
		})
	}
	return out
}

func previewLiveIndex(candidates []miniprogram.Candidate) int {
	for i, candidate := range candidates {
		if candidate.Live {
			return i
		}
	}
	return 0
}

func sourceAppID(attribution string) string {
	if attribution == miniprogram.AttributionUnattributed {
		return "wx1d2e3f4051627384"
	}
	return "wx6ab29f61bd262656"
}

func previewExt(kind string, live bool) string {
	if live {
		return "flv"
	}
	switch kind {
	case "image":
		return "jpg"
	case "video":
		return "mp4"
	case "audio":
		return "mp3"
	default:
		return "m3u8"
	}
}
