// Command genicon embeds the Windows executable icon resources.
//
// The icon artwork is reused as-is from the local reference project
// (wechat-channels-downloader/downloader/assets): logo.png for the TUI,
// logo.ico for the Windows executable and logo.icns for the macOS bundle.
// This generator does not draw or regenerate brand artwork; it only converts
// the checked-in build/icons/logo.ico into a COFF object file. The former
// build/genlogo generator was removed when the reference PNG replaced the
// generated brand image.
//
//	go run ./build/genicon
//
// The output, wx-mini-video_windows_amd64.syso, holds one .rsrc section with
// RT_GROUP_ICON and RT_ICON resources. Its filename makes the Go toolchain use
// it only for windows/amd64 builds; the Go linker copies the section into the
// PE resource directory. .gitignore ignores *.syso, so the object is always a
// local build artifact.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	repoModFile  = "go.mod"
	icoAssetPath = "build/icons/logo.ico"
	outputName   = "wx-mini-video_windows_amd64.syso"

	// Resource type and language identifiers, see
	// https://learn.microsoft.com/windows/win32/menurc/resource-types
	rtIcon          = 3
	rtGroupIcon     = 14
	groupIconID     = 1
	firstIconID     = 1
	languageID      = 0x0409 // en-US
	groupIconHeader = 6
	groupIconEntry  = 14

	// COFF object file layout constants.
	coffMachineAMD64         = 0x8664
	coffFileHeaderSize       = 20
	coffSectionHeaderSize    = 40
	coffRelocationSize       = 10
	coffSymbolSize           = 18
	coffSectionInitDataRead  = 0x40000040 // IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
	coffSymClassStatic       = 3          // IMAGE_SYM_CLASS_STATIC
	coffRelAMD64Addr32NB     = 3          // IMAGE_REL_AMD64_ADDR32NB
	coffFileLineNumsStripped = 0x0004

	resourceDirHeaderSize = 16
	resourceDirEntrySize  = 8
	resourceDataEntrySize = 16
	resourceSubdirFlag    = 0x80000000
	resourceSectionName   = ".rsrc"
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "genicon:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	target, err := generate(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", target, info.Size())
	return nil
}

// findRepoRoot walks up from the working directory until it finds go.mod, so
// the generator can be started from any directory inside the repository.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, repoModFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("未找到仓库根目录（缺少 %s）：请在仓库内运行", repoModFile)
		}
		dir = parent
	}
}

// generate reads build/icons/logo.ico, converts it into a COFF object and
// writes wx-mini-video_windows_amd64.syso at the repository root.
func generate(root string) (string, error) {
	icoData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(icoAssetPath)))
	if err != nil {
		return "", fmt.Errorf("读取 %s: %w", icoAssetPath, err)
	}
	icons, err := parseICO(icoData)
	if err != nil {
		return "", fmt.Errorf("解析 %s: %w", icoAssetPath, err)
	}
	section, err := buildResourceSection(icons)
	if err != nil {
		return "", err
	}
	object, err := writeCOFFObject(section)
	if err != nil {
		return "", err
	}

	target := filepath.Join(root, outputName)
	temp, err := os.CreateTemp(root, ".genicon-*.syso")
	if err != nil {
		return "", err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(object); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempName, target); err != nil {
		return "", err
	}
	return target, nil
}

// iconFrame is one image stored in the ICO file. data holds the payload as
// stored in the ICO: either a complete PNG stream or a DIB (BITMAPINFOHEADER,
// color table, XOR bitmap and AND mask). RT_ICON uses both formats verbatim,
// so the ICO entry header itself is dropped and only the payload is kept.
type iconFrame struct {
	width      uint8 // 0 means 256 in the ICO encoding
	height     uint8
	colorCount uint8
	planes     uint16
	bitCount   uint16
	data       []byte
}

type icoFile struct {
	frames []iconFrame
}

func parseICO(data []byte) (*icoFile, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("ICO 文件过短: %d 字节", len(data))
	}
	if reserved := binary.LittleEndian.Uint16(data[0:]); reserved != 0 {
		return nil, fmt.Errorf("ICO 保留字段 = %d，期望 0", reserved)
	}
	if kind := binary.LittleEndian.Uint16(data[2:]); kind != 1 {
		return nil, fmt.Errorf("ICO 类型 = %d，期望 1（图标）", kind)
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if count == 0 {
		return nil, fmt.Errorf("ICO 不含任何图像")
	}
	if count > 0xffff {
		return nil, fmt.Errorf("ICO 图像数量 %d 超出资源 ID 范围", count)
	}
	if len(data) < 6+16*count {
		return nil, fmt.Errorf("ICO 目录被截断: 需要 %d 字节，实际 %d 字节", 6+16*count, len(data))
	}

	icons := &icoFile{frames: make([]iconFrame, 0, count)}
	seen := make(map[int]bool, count)
	for index := 0; index < count; index++ {
		entryOffset := 6 + 16*index
		entry := data[entryOffset : entryOffset+16]
		size := int(binary.LittleEndian.Uint32(entry[8:]))
		imageOffset := int(binary.LittleEndian.Uint32(entry[12:]))
		if size <= 0 {
			return nil, fmt.Errorf("第 %d 帧长度为 %d", index+1, size)
		}
		if imageOffset < 0 || imageOffset > len(data) || size > len(data)-imageOffset {
			return nil, fmt.Errorf("第 %d 帧越界: offset=%d size=%d total=%d", index+1, imageOffset, size, len(data))
		}
		if seen[imageOffset] {
			return nil, fmt.Errorf("第 %d 帧与前面的帧共用数据偏移 %d", index+1, imageOffset)
		}
		seen[imageOffset] = true
		payload := data[imageOffset : imageOffset+size]
		if !bytes.HasPrefix(payload, pngSignature) {
			if err := validateDIB(payload); err != nil {
				return nil, fmt.Errorf("第 %d 帧不是可用的 PNG 或 DIB: %w", index+1, err)
			}
		}
		icons.frames = append(icons.frames, iconFrame{
			width:      entry[0],
			height:     entry[1],
			colorCount: entry[2],
			planes:     binary.LittleEndian.Uint16(entry[4:]),
			bitCount:   binary.LittleEndian.Uint16(entry[6:]),
			data:       payload,
		})
	}
	return icons, nil
}

// validateDIB checks that a non-PNG ICO payload starts with a known
// BITMAPINFOHEADER variant. The payload is stored in RT_ICON unchanged: it is
// already the DIB (with the AND mask) without the ICO directory entry.
func validateDIB(payload []byte) error {
	if len(payload) < 40 {
		return fmt.Errorf("DIB 数据仅 %d 字节，缺少 BITMAPINFOHEADER", len(payload))
	}
	headerSize := binary.LittleEndian.Uint32(payload[:4])
	switch headerSize {
	case 12, 40, 52, 56, 108, 124:
	default:
		return fmt.Errorf("不支持的 DIB 头长度 %d", headerSize)
	}
	if int(headerSize) > len(payload) {
		return fmt.Errorf("DIB 头声明 %d 字节，实际仅 %d 字节", headerSize, len(payload))
	}
	return nil
}

// resourceSection is the raw .rsrc payload plus the offsets of every 32-bit
// field that must become an RVA after linking.
type resourceSection struct {
	data   []byte
	relocs []int
}

// buildResourceSection lays out the PE resource tree:
//
//	RT_ICON (3)        -> id 1..N      -> language 0x0409 -> data entry -> PNG/DIB
//	RT_GROUP_ICON (14) -> id 1         -> language 0x0409 -> data entry -> GRPICONDIR
//
// Every RVA field stores the section-relative offset; the linker adds the
// section RVA using the recorded ADDR32NB relocations.
func buildResourceSection(icons *icoFile) (*resourceSection, error) {
	frames := icons.frames
	count := len(frames)
	if count == 0 {
		return nil, fmt.Errorf("没有可嵌入的图标帧")
	}
	if count > 0xffff {
		return nil, fmt.Errorf("图标帧数量 %d 超出范围", count)
	}
	groupData := buildGroupIconData(frames)

	// Allocate the directory tree first, then the data entries and finally the
	// payloads, so every offset is known before entries are written.
	rootOffset := 0
	iconNamesOffset := rootOffset + resourceDirHeaderSize + 2*resourceDirEntrySize
	groupNameOffset := iconNamesOffset + resourceDirHeaderSize + count*resourceDirEntrySize
	iconLanguagesOffset := groupNameOffset + resourceDirHeaderSize + resourceDirEntrySize
	groupLanguageOffset := iconLanguagesOffset + (resourceDirHeaderSize+resourceDirEntrySize)*count
	dataEntriesOffset := groupLanguageOffset + resourceDirHeaderSize + resourceDirEntrySize
	payloadOffset := alignUp(dataEntriesOffset+resourceDataEntrySize*(count+1), 4)

	iconPayloadOffsets := make([]int, count)
	cursor := payloadOffset
	for index, frame := range frames {
		iconPayloadOffsets[index] = cursor
		cursor = alignUp(cursor+len(frame.data), 4)
	}
	groupPayloadOffset := cursor
	sectionSize := alignUp(groupPayloadOffset+len(groupData), 4)
	// RVA fields are patched through signed 32-bit relocations; keep the
	// section comfortably below 2 GiB.
	if sectionSize > 1<<30 {
		return nil, fmt.Errorf("资源数据过大: %d 字节", sectionSize)
	}

	section := &resourceSection{data: make([]byte, sectionSize)}
	data := section.data
	put16 := func(offset int, value uint16) {
		binary.LittleEndian.PutUint16(data[offset:], value)
	}
	put32 := func(offset int, value uint32) {
		binary.LittleEndian.PutUint32(data[offset:], value)
	}
	putRVA := func(offset int, value uint32) {
		put32(offset, value)
		section.relocs = append(section.relocs, offset)
	}
	putDirectory := func(offset, ids int) {
		put16(offset+12, 0) // no named entries
		put16(offset+14, uint16(ids))
	}

	// Type directory: RT_ICON then RT_GROUP_ICON (ascending ID).
	putDirectory(rootOffset, 2)
	put32(rootOffset+16, rtIcon)
	putRVA(rootOffset+20, uint32(iconNamesOffset)|resourceSubdirFlag)
	put32(rootOffset+24, rtGroupIcon)
	putRVA(rootOffset+28, uint32(groupNameOffset)|resourceSubdirFlag)

	// One name entry per icon frame, and one for the single group icon.
	putDirectory(iconNamesOffset, count)
	for index := range frames {
		entryOffset := iconNamesOffset + resourceDirHeaderSize + index*resourceDirEntrySize
		put32(entryOffset, uint32(firstIconID+index))
		languageOffset := iconLanguagesOffset + (resourceDirHeaderSize+resourceDirEntrySize)*index
		putRVA(entryOffset+4, uint32(languageOffset)|resourceSubdirFlag)
	}
	putDirectory(groupNameOffset, 1)
	groupNameEntry := groupNameOffset + resourceDirHeaderSize
	put32(groupNameEntry, groupIconID)
	putRVA(groupNameEntry+4, uint32(groupLanguageOffset)|resourceSubdirFlag)

	// Every icon and the group icon use a single en-US language entry.
	for index := range frames {
		languageOffset := iconLanguagesOffset + (resourceDirHeaderSize+resourceDirEntrySize)*index
		putDirectory(languageOffset, 1)
		languageEntry := languageOffset + resourceDirHeaderSize
		put32(languageEntry, languageID)
		putRVA(languageEntry+4, uint32(dataEntriesOffset+resourceDataEntrySize*index))
	}
	putDirectory(groupLanguageOffset, 1)
	groupLanguageEntry := groupLanguageOffset + resourceDirHeaderSize
	put32(groupLanguageEntry, languageID)
	putRVA(groupLanguageEntry+4, uint32(dataEntriesOffset+resourceDataEntrySize*count))

	// Data entries point at the payload bytes and record their exact size.
	for index, frame := range frames {
		entryOffset := dataEntriesOffset + resourceDataEntrySize*index
		putRVA(entryOffset, uint32(iconPayloadOffsets[index]))
		put32(entryOffset+4, uint32(len(frame.data)))
		copy(data[iconPayloadOffsets[index]:], frame.data)
	}
	groupEntryOffset := dataEntriesOffset + resourceDataEntrySize*count
	putRVA(groupEntryOffset, uint32(groupPayloadOffset))
	put32(groupEntryOffset+4, uint32(len(groupData)))
	copy(data[groupPayloadOffset:], groupData)

	sort.Ints(section.relocs)
	return section, nil
}

// buildGroupIconData serialises GRPICONDIR. It mirrors the ICO directory but
// replaces the 4-byte image offset with the 2-byte RT_ICON resource ID.
func buildGroupIconData(frames []iconFrame) []byte {
	data := make([]byte, groupIconHeader+groupIconEntry*len(frames))
	binary.LittleEndian.PutUint16(data[2:], 1) // icon type
	binary.LittleEndian.PutUint16(data[4:], uint16(len(frames)))
	for index, frame := range frames {
		entry := data[groupIconHeader+groupIconEntry*index:]
		entry[0] = frame.width
		entry[1] = frame.height
		entry[2] = frame.colorCount
		entry[3] = 0
		planes := frame.planes
		if planes == 0 {
			// The ICO writer stored 0; the group entry spec requires 1.
			planes = 1
		}
		binary.LittleEndian.PutUint16(entry[4:], planes)
		binary.LittleEndian.PutUint16(entry[6:], frame.bitCount)
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(frame.data)))
		binary.LittleEndian.PutUint16(entry[12:], uint16(firstIconID+index))
	}
	return data
}

// writeCOFFObject serialises a single-section COFF object that the Go linker
// accepts as a host object. The .rsrc section carries the resource tree and
// its relocations; one static section symbol provides the relocation target.
func writeCOFFObject(section *resourceSection) ([]byte, error) {
	if len(section.relocs) > 0xffff {
		return nil, fmt.Errorf("重定位数量 %d 超出 COFF 上限", len(section.relocs))
	}
	dataSize := len(section.data)
	for _, offset := range section.relocs {
		if offset < 0 || offset+4 > dataSize {
			return nil, fmt.Errorf("重定位偏移 %d 越界", offset)
		}
	}

	dataOffset := coffFileHeaderSize + coffSectionHeaderSize
	relocOffset := dataOffset + dataSize
	symbolOffset := relocOffset + coffRelocationSize*len(section.relocs)
	stringTableOffset := symbolOffset + coffSymbolSize
	totalSize := stringTableOffset + 4
	object := make([]byte, totalSize)

	// COFF file header.
	binary.LittleEndian.PutUint16(object[0:], coffMachineAMD64)
	binary.LittleEndian.PutUint16(object[2:], 1) // one section
	binary.LittleEndian.PutUint32(object[4:], 0) // timestamp
	binary.LittleEndian.PutUint32(object[8:], uint32(symbolOffset))
	binary.LittleEndian.PutUint32(object[12:], 1) // one symbol
	binary.LittleEndian.PutUint16(object[16:], 0) // no optional header
	binary.LittleEndian.PutUint16(object[18:], coffFileLineNumsStripped)

	// Section header for .rsrc. Field offsets follow the COFF specification:
	// Name 0, VirtualSize 8, VirtualAddress 12, SizeOfRawData 16,
	// PointerToRawData 20, PointerToRelocations 24, PointerToLineNumbers 28,
	// NumberOfRelocations 32, NumberOfLineNumbers 34, Characteristics 36.
	header := object[coffFileHeaderSize : coffFileHeaderSize+coffSectionHeaderSize]
	copy(header[0:8], resourceSectionName)
	binary.LittleEndian.PutUint32(header[8:], 0) // VirtualSize is 0 in object files
	binary.LittleEndian.PutUint32(header[12:], 0)
	binary.LittleEndian.PutUint32(header[16:], uint32(dataSize))
	binary.LittleEndian.PutUint32(header[20:], uint32(dataOffset))
	binary.LittleEndian.PutUint32(header[24:], uint32(relocOffset))
	binary.LittleEndian.PutUint32(header[28:], 0)
	binary.LittleEndian.PutUint16(header[32:], uint16(len(section.relocs)))
	binary.LittleEndian.PutUint16(header[34:], 0)
	binary.LittleEndian.PutUint32(header[36:], coffSectionInitDataRead)

	// Section data composed of the resource tree and its payloads.
	copy(object[dataOffset:], section.data)

	// Relocation table: every RVA field is relative to the .rsrc section and
	// resolved by the linker to the final section RVA.
	for index, offset := range section.relocs {
		entry := object[relocOffset+coffRelocationSize*index:]
		binary.LittleEndian.PutUint32(entry[0:], uint32(offset))
		binary.LittleEndian.PutUint32(entry[4:], 0) // section symbol index
		binary.LittleEndian.PutUint16(entry[8:], coffRelAMD64Addr32NB)
	}

	// Symbol table: a static section symbol named ".rsrc". The Go linker treats
	// it as the section symbol and resolves the relocations against it.
	symbol := object[symbolOffset : symbolOffset+coffSymbolSize]
	copy(symbol[0:8], []byte(".rsrc\x00\x00\x00"))
	binary.LittleEndian.PutUint32(symbol[8:], 0)  // Value
	binary.LittleEndian.PutUint16(symbol[12:], 1) // SectionNumber
	binary.LittleEndian.PutUint16(symbol[14:], 0) // Type
	symbol[16] = coffSymClassStatic
	symbol[17] = 0 // no auxiliary symbols

	// Empty string table: only its own 4-byte length.
	binary.LittleEndian.PutUint32(object[stringTableOffset:], 4)

	return object, nil
}

func alignUp(value, alignment int) int {
	return (value + alignment - 1) &^ (alignment - 1)
}
