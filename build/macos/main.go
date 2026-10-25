package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"wx_channel/internal/minidownload"
)

const (
	ffmpegCacheDirName  = "ffmpeg-cache"
	ffmpegDownloadAgent = "wx-mini-video-macos-builder"
	// ffmpegDownloadTimeout 限制整份 ffmpeg ZIP 的下载时间，避免 CI 卡住。
	ffmpegDownloadTimeout = 30 * time.Minute
	// bundledFFmpegEnv 指向自建 ffmpeg 目录（ffmpeg-amd64、ffmpeg-arm64、LICENSE.txt）。
	bundledFFmpegEnv = "WXMV_FFMPEG_DIR"
)

func main() {
	if err := build(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("请在仓库根目录运行: %w", err)
	}
	dist := filepath.Join(root, "dist", "wx-mini-video-macos")
	app := filepath.Join(dist, "wx-mini-video.app", "Contents")
	if err := os.MkdirAll(filepath.Join(app, "MacOS"), 0o755); err != nil {
		return err
	}
	var slices [][]byte
	for _, arch := range []string{"amd64", "arm64"} {
		path := filepath.Join(root, "dist", "wx-mini-video-darwin-"+arch)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w -X main.Mode=release", "-o", path, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=darwin", "GOARCH="+arch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("构建 %s: %w", arch, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		slices = append(slices, data)
	}
	data, err := universalMachO(slices[0], slices[1])
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "MacOS", "wx-mini-video"), data, 0o755); err != nil {
		return err
	}
	if err := copyAppIcon(root, app); err != nil {
		return err
	}
	source, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return err
	}
	version := regexp.MustCompile(`var AppVer = "v([^"]+)"`).FindSubmatch(source)
	if len(version) != 2 {
		return fmt.Errorf("未能从 main.go 读取版本")
	}
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(infoPlist(string(version[1]))), 0o644); err != nil {
		return err
	}
	for src, dst := range map[string]string{"docs/macos-usage.md": "README.md", "internal/config/config.template.yaml": "wx-mini-video.yaml"} {
		data, err := os.ReadFile(filepath.Join(root, src))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dist, dst), data, 0o644); err != nil {
			return err
		}
	}
	if err := prepareBundledFFmpeg(root, dist); err != nil {
		return err
	}
	zipPath := filepath.Join(root, "dist", "wx-mini-video-macos-universal-unsigned.zip")
	if err := writePackage(dist, zipPath); err != nil {
		return err
	}
	fmt.Printf("已构建未签名测试包: %s\n正式公开发布前须在 Mac 上完成签名、公证和真机验收。\n", zipPath)
	return nil
}

// infoPlist renders the bundle metadata. CFBundleIconFile/CFBundleIconName
// point at Contents/Resources/logo.icns, the shared brand icon reused from the
// reference project and copied by copyAppIcon.
func infoPlist(version string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>io.github.muzijin701.wx-mini-video</string>
<key>CFBundleName</key><string>wx-mini-video</string>
<key>CFBundleExecutable</key><string>wx-mini-video</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleIconFile</key><string>logo</string>
<key>CFBundleIconName</key><string>logo</string>
<key>CFBundleShortVersionString</key><string>%s</string>
<key>CFBundleVersion</key><string>%s</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSAppleEventsUsageDescription</key><string>在 Terminal 中打开微信小程序媒体下载界面。</string>
</dict></plist>
`, version, version)
}

// copyAppIcon installs build/icons/logo.icns at
// wx-mini-video.app/Contents/Resources/logo.icns. The ICO, ICNS and PNG are
// reused from the local reference project; this build step only copies the
// checked-in asset.
func copyAppIcon(root, app string) error {
	resources := filepath.Join(app, "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	source := filepath.Join(root, "build", "icons", "logo.icns")
	if err := copyFile(source, filepath.Join(resources, "logo.icns"), 0o644); err != nil {
		return fmt.Errorf("复制 macOS 应用图标: %w", err)
	}
	return nil
}

// prepareBundledFFmpeg 下载并校验两份 Martin Riedl 9.0.2 原始构建，解出 .app 之外的
// ffmpeg/ffmpeg-<arch>，同时放入 GPLv3 全文与第三方声明。不做 lipo 合并。
func prepareBundledFFmpeg(root, dist string) error {
	outputDir := filepath.Join(dist, "ffmpeg")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	licenseSource := filepath.Join(root, "build", "licenses", "GPLv3.txt")
	if sourceDir := strings.TrimSpace(os.Getenv(bundledFFmpegEnv)); sourceDir != "" {
		for _, arch := range []string{"amd64", "arm64"} {
			source := filepath.Join(sourceDir, "ffmpeg-"+arch)
			destination := filepath.Join(outputDir, "ffmpeg-"+arch)
			if err := copyFile(source, destination, 0o755); err != nil {
				return fmt.Errorf("复制预构建 darwin/%s ffmpeg: %w", arch, err)
			}
			fmt.Printf("已准备预构建随包 ffmpeg: %s\n", destination)
		}
		licenseSource = filepath.Join(sourceDir, "LICENSE.txt")
	} else {
		cacheDir := filepath.Join(root, "dist", ffmpegCacheDirName)
		for _, arch := range []string{"amd64", "arm64"} {
			sources := minidownload.FFmpegSourcesForPlatform("darwin", arch)
			if len(sources) == 0 {
				return fmt.Errorf("缺少 darwin/%s 的 ffmpeg 下载源", arch)
			}
			source := sources[0]
			archivePath := filepath.Join(cacheDir, "martin-riedl-9.0.2-"+arch+".zip")
			if err := ensureFFmpegArchive(source, archivePath); err != nil {
				return fmt.Errorf("准备 darwin/%s ffmpeg: %w", arch, err)
			}
			binaryPath := filepath.Join(outputDir, "ffmpeg-"+arch)
			if err := extractFFmpegBinary(archivePath, binaryPath, arch); err != nil {
				return fmt.Errorf("解出 darwin/%s ffmpeg: %w", arch, err)
			}
			fmt.Printf("已准备随包 ffmpeg: %s\n", binaryPath)
		}
	}
	license, err := os.ReadFile(licenseSource)
	if err != nil {
		return fmt.Errorf("读取随包 ffmpeg 许可失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "LICENSE.txt"), license, 0o644); err != nil {
		return err
	}
	notices, err := os.ReadFile(filepath.Join(root, "THIRD-PARTY-NOTICES.md"))
	if err != nil {
		return fmt.Errorf("读取第三方声明失败: %w", err)
	}
	return os.WriteFile(filepath.Join(dist, "THIRD-PARTY-NOTICES.md"), notices, 0o644)
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	return os.Chmod(destination, mode)
}

// ensureFFmpegArchive 复用已校验的缓存压缩包，否则下载并按固定 SHA-256 校验。
func ensureFFmpegArchive(source minidownload.FFmpegSource, path string) error {
	if err := verifyArchiveSHA256(path, source.SHA256); err == nil {
		fmt.Printf("复用已校验 ffmpeg 压缩包: %s\n", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: ffmpegDownloadTimeout}
	request, err := http.NewRequest(http.MethodGet, source.URL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	request.Header.Set("User-Agent", ffmpegDownloadAgent)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("下载 %s 失败: %w", source.URL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("下载 %s 失败: HTTP %s", source.URL, response.Status)
	}
	partial := path + ".part"
	file, err := os.Create(partial)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(partial)
		return fmt.Errorf("下载 %s 失败: %w", source.URL, errors.Join(copyErr, closeErr))
	}
	if err := verifyArchiveSHA256(partial, source.SHA256); err != nil {
		_ = os.Remove(partial)
		return fmt.Errorf("下载的 ffmpeg 校验失败: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(partial)
		return err
	}
	return os.Rename(partial, path)
}

func verifyArchiveSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("SHA-256 不匹配: 得到 %s", actual)
	}
	return nil
}

// extractFFmpegBinary 从 Martin Riedl ZIP 解出原始 ffmpeg 二进制并核对架构，不合并架构。
func extractFFmpegBinary(archivePath, outputPath, arch string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开 ffmpeg 压缩包失败: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if filepath.Base(file.Name) != "ffmpeg" {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝解压符号链接形式的 ffmpeg")
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		defer source.Close()
		temp, err := os.CreateTemp(filepath.Dir(outputPath), ".ffmpeg-*")
		if err != nil {
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		defer os.Remove(temp.Name())
		if _, err := io.Copy(temp, source); err != nil {
			temp.Close()
			return fmt.Errorf("解压 ffmpeg 失败: %w", err)
		}
		if err := temp.Close(); err != nil {
			return fmt.Errorf("写入 ffmpeg 失败: %w", err)
		}
		if err := os.Chmod(temp.Name(), 0o755); err != nil {
			return err
		}
		if err := verifyThinMachO(temp.Name(), arch); err != nil {
			return err
		}
		return os.Rename(temp.Name(), outputPath)
	}
	return fmt.Errorf("压缩包中未找到 ffmpeg")
}

func verifyThinMachO(path, arch string) error {
	expected := map[string]macho.Cpu{"amd64": macho.CpuAmd64, "arm64": macho.CpuArm64}[arch]
	file, err := macho.Open(path)
	if err != nil {
		return fmt.Errorf("ffmpeg 不是可执行 Mach-O: %w", err)
	}
	defer file.Close()
	if file.Type != macho.TypeExec {
		return fmt.Errorf("ffmpeg 不是可执行 Mach-O")
	}
	if file.Cpu != expected {
		return fmt.Errorf("ffmpeg 架构为 %v，期望 %v", file.Cpu, expected)
	}
	return nil
}

func universalMachO(amd64, arm64 []byte) ([]byte, error) {
	slices := [][]byte{amd64, arm64}
	var headers [2]*macho.File
	for i, data := range slices {
		file, err := macho.NewFile(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		headers[i] = file
		if file.Type != macho.TypeExec {
			return nil, fmt.Errorf("输入不是可执行 Mach-O")
		}
	}
	if headers[0].Cpu != macho.CpuAmd64 || headers[1].Cpu != macho.CpuArm64 {
		return nil, fmt.Errorf("通用包需要按顺序提供 amd64 与 arm64")
	}
	const alignment = 1 << 14
	var offsets [2]uint32
	length := uint64(alignment)
	for i, data := range slices {
		if length+uint64(len(data)) > uint64(^uint32(0)) {
			return nil, fmt.Errorf("Mach-O 输入超过 FAT32 容量")
		}
		offsets[i] = uint32(length)
		length = (length + uint64(len(data)) + alignment - 1) &^ (alignment - 1)
	}
	result := make([]byte, offsets[1]+uint32(len(arm64)))
	binary.BigEndian.PutUint32(result, macho.MagicFat)
	binary.BigEndian.PutUint32(result[4:], 2)
	for i, data := range slices {
		offset := 8 + 20*i
		for j, value := range []uint32{uint32(headers[i].Cpu), headers[i].SubCpu, offsets[i], uint32(len(data)), 14} {
			binary.BigEndian.PutUint32(result[offset+j*4:], value)
		}
		copy(result[offsets[i]:], data)
	}
	if _, err := macho.NewFatFile(bytes.NewReader(result)); err != nil {
		return nil, fmt.Errorf("验证通用 Mach-O 失败: %w", err)
	}
	return result, nil
}

func writePackage(root, path string) (err error) {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	archive := zip.NewWriter(file)
	// An allowlist keeps existing downloads, private keys and local files out of builds.
	entries := []struct {
		name string
		mode os.FileMode
	}{
		{"README.md", 0o644},
		{"wx-mini-video.yaml", 0o644},
		{"THIRD-PARTY-NOTICES.md", 0o644},
		{"wx-mini-video.app/Contents/Info.plist", 0o644},
		{"wx-mini-video.app/Contents/MacOS/wx-mini-video", 0o755},
		{"wx-mini-video.app/Contents/Resources/logo.icns", 0o644},
		{"ffmpeg/ffmpeg-amd64", 0o755},
		{"ffmpeg/ffmpeg-arm64", 0o755},
		{"ffmpeg/LICENSE.txt", 0o644},
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entry.name)))
		if err != nil {
			archive.Close()
			return err
		}
		header := &zip.FileHeader{Name: "wx-mini-video-macos/" + entry.name, Method: zip.Deflate}
		header.SetMode(entry.mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			return err
		}
		if _, err := io.Copy(writer, bytes.NewReader(data)); err != nil {
			archive.Close()
			return err
		}
	}
	return archive.Close()
}
