//go:build windows && amd64

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLinkedExecutableIconReadableByWindows(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "build", "icons"), 0o755); err != nil {
		t.Fatal(err)
	}
	ico := readFixtureICO(t)
	if err := os.WriteFile(filepath.Join(root, icoAssetPath), ico, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := generate(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "icon.exe")
	cmd := exec.Command("go", "build", "-o", executable, ".")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GO111MODULE=off", "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("链接图标测试程序: %v\n%s", err, output)
	}
	// Load only resources; never execute the test binary.
	module, err := windows.LoadLibraryEx(executable, 0, windows.LOAD_LIBRARY_AS_DATAFILE|windows.LOAD_LIBRARY_AS_IMAGE_RESOURCE)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.FreeLibrary(module)
	read := func(id, kind uint32) []byte {
		t.Helper()
		info, err := windows.FindResource(module, windows.ResourceID(id), windows.ResourceID(kind))
		if err != nil {
			t.Fatalf("Windows 无法读取图标资源 type=%d id=%d: %v", kind, id, err)
		}
		data, err := windows.LoadResourceData(module, info)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	icons, err := parseICO(ico)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(groupIconID, rtGroupIcon), buildGroupIconData(icons.frames)) {
		t.Fatal("Windows 读取的图标组与参考 ICO 不一致")
	}
	for index, frame := range icons.frames {
		if !bytes.Equal(read(uint32(firstIconID+index), rtIcon), frame.data) {
			t.Fatalf("Windows 读取的图标帧 %d 与参考 ICO 不一致", index)
		}
	}
}
