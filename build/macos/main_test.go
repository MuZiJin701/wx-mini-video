package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func thinMachO(cpu macho.Cpu, subtype uint32) []byte {
	var data bytes.Buffer
	_ = binary.Write(&data, binary.LittleEndian, []uint32{macho.Magic64, uint32(cpu), subtype, uint32(macho.TypeExec), 0, 0, 0, 0})
	return data.Bytes()
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
	for _, name := range []string{"README.md", "wx-mini-video.yaml", "wx-mini-video.app/Contents/Info.plist", "wx-mini-video.app/Contents/MacOS/wx-mini-video", "downloads/private.pem"} {
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
	if len(archive.File) != 4 {
		t.Fatalf("unexpected package contents: %d", len(archive.File))
	}
	for _, file := range archive.File {
		if filepath.Base(file.Name) == "wx-mini-video" && file.Mode().Perm() != 0o755 {
			t.Fatal("ZIP lost executable permissions")
		}
	}
}
