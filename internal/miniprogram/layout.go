package miniprogram

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// 下载产物的一级分类目录，与界面分类保持一致。
const (
	DirImages = "图片"
	DirVideos = "视频"
	DirAudios = "音频"
	DirM3U8   = "m3u8"
	DirOther  = "其他"
)

// MediaKind 把候选归一到五个下载分类对应的类型：
// image/video/audio 原样保留，kind 或 URL 扩展名表明是 m3u8 时归为 m3u8，
// file 及其他未知类型统一归为 file（目录 其他）。
func MediaKind(c Candidate) string {
	if u, err := url.Parse(c.URL); err == nil && strings.EqualFold(path.Ext(u.Path), ".m3u8") {
		return "m3u8"
	}
	switch c.Kind {
	case "image", "video", "audio", "m3u8":
		return c.Kind
	default:
		return "file"
	}
}

// TypeDir 返回媒体类型对应的下载分类目录名。
func TypeDir(kind string) string {
	switch kind {
	case "image":
		return DirImages
	case "video":
		return DirVideos
	case "audio":
		return DirAudios
	case "m3u8":
		return DirM3U8
	default:
		return DirOther
	}
}

// SanitizeAppIDDir 把 AppID 清洗成可用的单级目录名。
// 控制字符与文件系统非法字符替换为 '-'，去掉首尾空格与结尾点；
// 空值、'.'、'..' 回退为 _unknown；Windows 保留名加 '_' 前缀。
func SanitizeAppIDDir(appID string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, appID)
	name = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(name), "."))
	if name == "" || name == "." || name == ".." {
		return "_unknown"
	}
	if isWindowsReservedName(name) {
		return "_" + name
	}
	return name
}

func isWindowsReservedName(name string) bool {
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}

// NormalizeTargetAppID 对手动输入的 AppID 做宽松校验：
// 只拒绝空值、路径字符与文件系统非法字符，其余原样返回（保留原始大小写）。
func NormalizeTargetAppID(raw string) (string, error) {
	appID := strings.TrimSpace(raw)
	if appID == "" {
		return "", fmt.Errorf("请先输入小程序 AppID")
	}
	if appID == "." || appID == ".." {
		return "", fmt.Errorf("AppID 不能包含路径字符")
	}
	if strings.ContainsAny(appID, `/\<>:"|?*`) {
		return "", fmt.Errorf(`AppID 不能包含路径字符或文件名非法字符（/ \ < > : " | ? *）`)
	}
	if strings.IndexFunc(appID, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("AppID 不能包含控制字符")
	}
	return appID, nil
}

// MediaSubdir 返回候选的下载子目录：<归属 AppID>/<资源类型>。
// 已确认候选使用自身 AppID；来源未确认候选使用下载时的当前目标 AppID。
func MediaSubdir(c Candidate, targetAppID string) string {
	appID := strings.TrimSpace(c.AppID)
	if appID == "" {
		appID = strings.TrimSpace(targetAppID)
	}
	return filepath.Join(SanitizeAppIDDir(appID), TypeDir(MediaKind(c)))
}
