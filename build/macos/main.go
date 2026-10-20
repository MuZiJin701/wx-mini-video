package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	source, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		return err
	}
	version := regexp.MustCompile(`var AppVer = "v([^"]+)"`).FindSubmatch(source)
	if len(version) != 2 {
		return fmt.Errorf("未能从 main.go 读取版本")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>io.github.muzijin701.wx-mini-video</string>
<key>CFBundleName</key><string>wx-mini-video</string>
<key>CFBundleExecutable</key><string>wx-mini-video</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>%s</string>
<key>CFBundleVersion</key><string>%s</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSAppleEventsUsageDescription</key><string>在 Terminal 中打开微信小程序媒体下载界面。</string>
</dict></plist>
`, version[1], version[1])
	if err := os.WriteFile(filepath.Join(app, "Info.plist"), []byte(plist), 0o644); err != nil {
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
	zipPath := filepath.Join(root, "dist", "wx-mini-video-macos-universal-unsigned.zip")
	if err := writePackage(dist, zipPath); err != nil {
		return err
	}
	fmt.Printf("已构建未签名测试包: %s\n正式公开发布前须在 Mac 上完成签名、公证和真机验收。\n", zipPath)
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
	for _, name := range []string{"README.md", "wx-mini-video.yaml", "wx-mini-video.app/Contents/Info.plist", "wx-mini-video.app/Contents/MacOS/wx-mini-video"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			archive.Close()
			return err
		}
		header := &zip.FileHeader{Name: "wx-mini-video-macos/" + name, Method: zip.Deflate}
		mode := os.FileMode(0o644)
		if name == "wx-mini-video.app/Contents/MacOS/wx-mini-video" {
			mode = 0o755
		}
		header.SetMode(mode)
		entry, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			return err
		}
		if _, err := io.Copy(entry, bytes.NewReader(data)); err != nil {
			archive.Close()
			return err
		}
	}
	return archive.Close()
}
