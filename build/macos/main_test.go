package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wx_channel/internal/minidownload"
)

func thinMachO(cpu macho.Cpu, subtype uint32) []byte {
	var data bytes.Buffer
	_ = binary.Write(&data, binary.LittleEndian, []uint32{macho.Magic64, uint32(cpu), subtype, uint32(macho.TypeExec), 0, 0, 0, 0})
	return data.Bytes()
}

func ffmpegZip(t *testing.T, content []byte) ([]byte, string) {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	return archive.Bytes(), hex.EncodeToString(sum[:])
}

func TestUniversalMachOPreservesBothArchitectures(t *testing.T) {
	amd, arm := thinMachO(macho.CpuAmd64, 3), thinMachO(macho.CpuArm64, 0)
	data, err := universalMachO(amd, arm)
	if err != nil {
		t.Fatal(err)
	}
	fat, err := macho.NewFatFile(bytes.NewReader(data))
	if err != nil || len(fat.Arches) != 2 {
		t.Fatalf("fat file = %v, %v", fat, err)
	}
	for i, input := range [][]byte{amd, arm} {
		arch := fat.Arches[i]
		if arch.Offset%(1<<14) != 0 || !bytes.Equal(data[arch.Offset:arch.Offset+arch.Size], input) {
			t.Fatal("slice alignment or contents changed")
		}
	}
	if _, err := universalMachO(arm, amd); err == nil {
		t.Fatal("accepted reversed architectures")
	}
	if _, err := universalMachO(nil, arm); err == nil {
		t.Fatal("accepted invalid executable")
	}
}

func TestPackagePreservesExecutableModeAndExcludesRuntimeData(t *testing.T) {
	root := t.TempDir()
	executables := []string{
		"wx-mini-video.app/Contents/MacOS/wx-mini-video",
		"ffmpeg/ffmpeg-amd64",
		"ffmpeg/ffmpeg-arm64",
	}
	documents := []string{
		"README.md", "wx-mini-video.yaml", "THIRD-PARTY-NOTICES.md",
		"wx-mini-video.app/Contents/Info.plist",
		"wx-mini-video.app/Contents/Resources/logo.icns",
		"ffmpeg/LICENSE.txt",
	}
	for _, name := range append(append([]string{}, executables...), append(append([]string{}, documents...), "downloads/private.pem")...) {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "package.zip")
	if err := writePackage(root, path); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != len(executables)+len(documents) {
		t.Fatalf("unexpected package contents: %d", len(archive.File))
	}
	modes := make(map[string]os.FileMode, len(archive.File))
	for _, file := range archive.File {
		name := file.Name
		modes[name] = file.Mode().Perm()
	}
	for _, name := range executables {
		if got := modes["wx-mini-video-macos/"+name]; got != 0o755 {
			t.Fatalf("ZIP lost executable permissions for %s: %v", name, got)
		}
	}
	for _, name := range documents {
		if got := modes["wx-mini-video-macos/"+name]; got != 0o644 {
			t.Fatalf("ZIP mode for %s = %v, want 0644", name, got)
		}
	}
	if _, ok := modes["wx-mini-video-macos/downloads/private.pem"]; ok {
		t.Fatal("package included runtime data")
	}
}

func TestExtractFFmpegBinaryKeepsThinArchitecture(t *testing.T) {
	root := t.TempDir()
	arm := thinMachO(macho.CpuArm64, 0)
	archivePath := filepath.Join(root, "arm64.zip")
	archive, _ := ffmpegZip(t, arm)
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "ffmpeg-arm64")
	if err := extractFFmpegBinary(archivePath, outputPath, "arm64"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, arm) {
		t.Fatal("extracted ffmpeg changed (lipo or reinterpretation must not happen)")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	// Windows 不以权限位表达可执行，0766/0755 的执行位断言只在类 Unix 主机成立。
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("extracted ffmpeg mode = %v, want 0755", info.Mode().Perm())
	}
	if err := extractFFmpegBinary(archivePath, filepath.Join(root, "wrong"), "amd64"); err == nil {
		t.Fatal("accepted mismatched architecture")
	}
}

func TestEnsureFFmpegArchiveDownloadsVerifiesAndReuses(t *testing.T) {
	archive, sum := ffmpegZip(t, thinMachO(macho.CpuAmd64, 3))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.UserAgent() != ffmpegDownloadAgent {
			t.Errorf("User-Agent = %q, want %q", r.UserAgent(), ffmpegDownloadAgent)
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	source := minidownload.FFmpegSource{URL: server.URL, SHA256: sum}
	path := filepath.Join(t.TempDir(), "ffmpeg-cache", "martin-9.0.2-amd64.zip")
	if err := ensureFFmpegArchive(source, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, archive) || requests != 1 {
		t.Fatalf("cache file or request count incorrect: requests=%d", requests)
	}
	server.Close()
	if err := ensureFFmpegArchive(source, path); err != nil {
		t.Fatalf("cached archive was not reused: %v", err)
	}
	if requests != 1 {
		t.Fatalf("cache reuse performed another request: %d", requests)
	}
}

func TestEnsureFFmpegArchiveRejectsChecksumMismatch(t *testing.T) {
	archive, _ := ffmpegZip(t, thinMachO(macho.CpuArm64, 0))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	source := minidownload.FFmpegSource{URL: server.URL, SHA256: "invalid-checksum"}
	path := filepath.Join(t.TempDir(), "martin-9.0.2-arm64.zip")
	if err := ensureFFmpegArchive(source, path); err == nil {
		t.Fatal("accepted archive with mismatched checksum")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unverified archive left behind: %v", err)
	}
}

func TestCopyAppIconInstallsBundleIcon(t *testing.T) {
	root := t.TempDir()
	icon := []byte("icns-test-icon-bytes")
	source := filepath.Join(root, "build", "icons", "logo.icns")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, icon, 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "wx-mini-video.app", "Contents")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyAppIcon(root, app); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(app, "Resources", "logo.icns")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, icon) {
		t.Fatal("已安装的应用图标与 build/icons/logo.icns 不一致")
	}
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("已安装的应用图标权限 = %v, want 0644", info.Mode().Perm())
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := copyAppIcon(root, app); err == nil {
		t.Fatal("缺少 build/icons/logo.icns 时 copyAppIcon 未报错")
	}
}

func TestInfoPlistDeclaresBundleIcon(t *testing.T) {
	plist := infoPlist("0.2.0")
	for _, want := range []string{
		"<key>CFBundleIconFile</key><string>logo</string>",
		"<key>CFBundleIconName</key><string>logo</string>",
		"<key>CFBundleShortVersionString</key><string>0.2.0</string>",
		"<key>CFBundleVersion</key><string>0.2.0</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("Info.plist 缺少 %q", want)
		}
	}
}
