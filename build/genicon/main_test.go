package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parsedCOFF is a minimal, independent COFF reader used to verify the object
// written by genicon. It intentionally duplicates layout knowledge from the
// generator so a wrong header field is caught instead of silently mirrored.
type parsedCOFF struct {
	machine     uint16
	sections    []parsedCOFFSection
	symbols     []parsedCOFFSymbol
	stringTable []byte
}

type parsedCOFFSection struct {
	name            string
	sizeOfRawData   int
	virtualSize     uint32
	characteristics uint32
	data            []byte
	relocations     []parsedCOFFRelocation
}

type parsedCOFFRelocation struct {
	offset      uint32
	symbolIndex uint32
	typ         uint16
}

type parsedCOFFSymbol struct {
	name         string
	value        uint32
	section      int16
	typ          uint16
	storageClass uint8
	auxCount     uint8
}

func parseCOFFObject(t *testing.T, object []byte) parsedCOFF {
	t.Helper()
	readU16 := func(offset int) uint16 {
		if offset < 0 || offset+2 > len(object) {
			t.Fatalf("COFF 读取越界: uint16 @ %d (len=%d)", offset, len(object))
		}
		return binary.LittleEndian.Uint16(object[offset:])
	}
	readU32 := func(offset int) uint32 {
		if offset < 0 || offset+4 > len(object) {
			t.Fatalf("COFF 读取越界: uint32 @ %d (len=%d)", offset, len(object))
		}
		return binary.LittleEndian.Uint32(object[offset:])
	}

	if len(object) < coffFileHeaderSize {
		t.Fatalf("COFF 对象过短: %d 字节", len(object))
	}
	parsed := parsedCOFF{machine: readU16(0)}
	sectionCount := int(readU16(2))
	symbolOffset := int(readU32(8))
	symbolCount := int(readU32(12))
	if optional := readU16(16); optional != 0 {
		t.Fatalf("COFF 可选头长度 = %d，期望 0", optional)
	}

	for index := 0; index < sectionCount; index++ {
		header := coffFileHeaderSize + coffSectionHeaderSize*index
		if header+coffSectionHeaderSize > len(object) {
			t.Fatalf("节头 %d 越界", index)
		}
		section := parsedCOFFSection{
			name:            strings.TrimRight(string(object[header:header+8]), "\x00"),
			virtualSize:     readU32(header + 8),
			sizeOfRawData:   int(readU32(header + 16)),
			characteristics: readU32(header + 36),
		}
		rawOffset := int(readU32(header + 20))
		relocationOffset := int(readU32(header + 24))
		relocationCount := int(readU16(header + 32))
		if rawOffset < 0 || rawOffset+section.sizeOfRawData > len(object) {
			t.Fatalf("节 %q 数据越界: offset=%d size=%d", section.name, rawOffset, section.sizeOfRawData)
		}
		section.data = object[rawOffset : rawOffset+section.sizeOfRawData]
		if relocationCount > 0 {
			if relocationOffset < 0 || relocationOffset+coffRelocationSize*relocationCount > len(object) {
				t.Fatalf("节 %q 重定位越界", section.name)
			}
			for relocIndex := 0; relocIndex < relocationCount; relocIndex++ {
				offset := relocationOffset + coffRelocationSize*relocIndex
				section.relocations = append(section.relocations, parsedCOFFRelocation{
					offset:      readU32(offset),
					symbolIndex: readU32(offset + 4),
					typ:         readU16(offset + 8),
				})
			}
		}
		parsed.sections = append(parsed.sections, section)
	}

	if symbolOffset < 0 || symbolOffset+coffSymbolSize*symbolCount > len(object) {
		t.Fatalf("符号表越界: offset=%d count=%d", symbolOffset, symbolCount)
	}
	// Walk the symbols, skipping auxiliary records the same way a linker does.
	for index := 0; index < symbolCount; {
		offset := symbolOffset + coffSymbolSize*index
		symbol := parsedCOFFSymbol{
			name:         strings.TrimRight(string(object[offset:offset+8]), "\x00"),
			value:        readU32(offset + 8),
			section:      int16(readU16(offset + 12)),
			typ:          readU16(offset + 14),
			storageClass: object[offset+16],
			auxCount:     object[offset+17],
		}
		parsed.symbols = append(parsed.symbols, symbol)
		index += int(symbol.auxCount) + 1
	}

	stringOffset := symbolOffset + coffSymbolSize*symbolCount
	if stringOffset+4 > len(object) {
		t.Fatalf("缺少 COFF 字符串表")
	}
	length := int(readU32(stringOffset))
	if length < 4 || stringOffset+length > len(object) {
		t.Fatalf("COFF 字符串表长度非法: %d", length)
	}
	parsed.stringTable = object[stringOffset : stringOffset+length]
	return parsed
}

// resourceView maps RT_* type -> resource ID -> language ID -> payload.
type resourceView map[uint32]map[uint32]map[uint32][]byte

type resourceDirEntry struct {
	id        uint32
	isDir     bool
	subdir    uint32
	dataEntry uint32
}

// parseResourceSection walks the resource tree in the .rsrc data and enforces
// that every 32-bit RVA field is covered by exactly one relocation. Unconsumed
// or duplicated relocation offsets fail the test.
func parseResourceSection(t *testing.T, section parsedCOFFSection) resourceView {
	t.Helper()
	data := section.data
	relocations := make(map[uint32]bool, len(section.relocations))
	for _, reloc := range section.relocations {
		if relocations[reloc.offset] {
			t.Fatalf("重复的重定位偏移 %d", reloc.offset)
		}
		relocations[reloc.offset] = true
	}
	readU16 := func(offset int) uint16 {
		if offset < 0 || offset+2 > len(data) {
			t.Fatalf("资源数据读取越界: uint16 @ %d (len=%d)", offset, len(data))
		}
		return binary.LittleEndian.Uint16(data[offset:])
	}
	readU32 := func(offset int) uint32 {
		if offset < 0 || offset+4 > len(data) {
			t.Fatalf("资源数据读取越界: uint32 @ %d (len=%d)", offset, len(data))
		}
		return binary.LittleEndian.Uint32(data[offset:])
	}
	consumeRelocation := func(offset uint32) {
		if !relocations[offset] {
			t.Fatalf("RVA 字段 @ %d 没有对应的重定位", offset)
		}
		delete(relocations, offset)
	}
	checkAligned := func(offset uint32, what string) {
		if offset%4 != 0 {
			t.Fatalf("%s 偏移 %d 未按 4 字节对齐", what, offset)
		}
	}

	var readDirectory func(offset uint32) []resourceDirEntry
	readDirectory = func(offset uint32) []resourceDirEntry {
		checkAligned(offset, "资源目录")
		if named := readU16(int(offset) + 12); named != 0 {
			t.Fatalf("资源目录 @ %d 含 %d 个命名项，生成器只应写 ID 项", offset, named)
		}
		count := int(readU16(int(offset) + 14))
		entries := make([]resourceDirEntry, 0, count)
		for index := 0; index < count; index++ {
			entryOffset := int(offset) + resourceDirHeaderSize + resourceDirEntrySize*index
			name := readU32(entryOffset)
			if name&resourceSubdirFlag != 0 {
				t.Fatalf("资源目录项 @ %d 使用了字符串名称", entryOffset)
			}
			// Directory pointers are relative to the resource root, not image RVAs.
			if relocations[uint32(entryOffset+4)] {
				t.Fatalf("资源目录相对偏移 @ %d 不应重定位", entryOffset+4)
			}
			value := readU32(entryOffset + 4)
			entry := resourceDirEntry{id: name}
			if value&resourceSubdirFlag != 0 {
				entry.isDir = true
				entry.subdir = value &^ resourceSubdirFlag
				checkAligned(entry.subdir, "资源子目录")
			} else {
				entry.dataEntry = value
				checkAligned(entry.dataEntry, "资源数据项")
			}
			entries = append(entries, entry)
		}
		return entries
	}
	readDataEntry := func(offset uint32) []byte {
		checkAligned(offset, "资源数据项")
		consumeRelocation(offset)
		payloadOffset := readU32(int(offset))
		size := readU32(int(offset) + 4)
		if codepage := readU32(int(offset) + 8); codepage != 0 {
			t.Fatalf("资源数据项 @ %d 的代码页 = %d，期望 0", offset, codepage)
		}
		if reserved := readU32(int(offset) + 12); reserved != 0 {
			t.Fatalf("资源数据项 @ %d 的保留字段 = %d，期望 0", offset, reserved)
		}
		checkAligned(payloadOffset, "资源载荷")
		if int(payloadOffset)+int(size) > len(data) {
			t.Fatalf("资源载荷越界: offset=%d size=%d len=%d", payloadOffset, size, len(data))
		}
		return data[payloadOffset : payloadOffset+size]
	}

	view := make(resourceView)
	for _, typeEntry := range readDirectory(0) {
		if !typeEntry.isDir {
			t.Fatalf("资源类型项 %d 未指向目录", typeEntry.id)
		}
		names := make(map[uint32]map[uint32][]byte)
		for _, nameEntry := range readDirectory(typeEntry.subdir) {
			if !nameEntry.isDir {
				t.Fatalf("资源名称项 %d 未指向目录", nameEntry.id)
			}
			languages := make(map[uint32][]byte)
			for _, languageEntry := range readDirectory(nameEntry.subdir) {
				if languageEntry.isDir {
					t.Fatalf("语言项 %d 不应再指向目录", languageEntry.id)
				}
				languages[languageEntry.id] = readDataEntry(languageEntry.dataEntry)
			}
			names[nameEntry.id] = languages
		}
		view[typeEntry.id] = names
	}
	if len(relocations) != 0 {
		for offset := range relocations {
			t.Fatalf("重定位 @ %d 没有被任何 RVA 字段使用", offset)
		}
	}
	return view
}

func readFixtureICO(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "icons", "logo.ico"))
	if err != nil {
		t.Fatalf("读取参考 ICO: %v", err)
	}
	return data
}

func TestGenerateBuildsCOFFObjectFromCheckedInICO(t *testing.T) {
	icoData := readFixtureICO(t)
	root := t.TempDir()
	iconDir := filepath.Join(root, "build", "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconDir, "logo.ico"), icoData, 0o644); err != nil {
		t.Fatal(err)
	}

	target, err := generate(root)
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	if want := filepath.Join(root, outputName); target != want {
		t.Fatalf("generate() target = %q, want %q", target, want)
	}
	object, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	parsed := parseCOFFObject(t, object)
	if parsed.machine != coffMachineAMD64 {
		t.Fatalf("COFF machine = %#x, want %#x", parsed.machine, coffMachineAMD64)
	}
	if len(parsed.sections) != 1 {
		t.Fatalf("COFF 节数量 = %d, want 1", len(parsed.sections))
	}
	section := parsed.sections[0]
	if section.name != resourceSectionName {
		t.Fatalf("节名 = %q, want %q", section.name, resourceSectionName)
	}
	if section.characteristics != coffSectionInitDataRead {
		t.Fatalf("节属性 = %#x, want %#x", section.characteristics, coffSectionInitDataRead)
	}
	if section.virtualSize != 0 {
		t.Fatalf("对象文件 VirtualSize = %d, want 0", section.virtualSize)
	}
	if len(section.relocations) == 0 {
		t.Fatal("COFF 对象没有重定位")
	}
	previous := int64(-1)
	for _, reloc := range section.relocations {
		if reloc.typ != coffRelAMD64Addr32NB {
			t.Fatalf("重定位类型 = %d, want %d", reloc.typ, coffRelAMD64Addr32NB)
		}
		if reloc.symbolIndex != 0 {
			t.Fatalf("重定位符号索引 = %d, want 0", reloc.symbolIndex)
		}
		if reloc.offset%4 != 0 {
			t.Fatalf("重定位偏移 %d 未按 4 字节对齐", reloc.offset)
		}
		if int64(reloc.offset) <= previous {
			t.Fatalf("重定位偏移未严格递增: %d after %d", reloc.offset, previous)
		}
		previous = int64(reloc.offset)
		if int(reloc.offset)+4 > len(section.data) {
			t.Fatalf("重定位偏移 %d 超出节数据", reloc.offset)
		}
	}
	if len(parsed.symbols) != 1 {
		t.Fatalf("COFF 符号数量 = %d, want 1", len(parsed.symbols))
	}
	symbol := parsed.symbols[0]
	if symbol.name != resourceSectionName || symbol.section != 1 || symbol.storageClass != coffSymClassStatic ||
		symbol.typ != 0 || symbol.value != 0 || symbol.auxCount != 0 {
		t.Fatalf("节符号不符合预期: %+v", symbol)
	}
	if len(parsed.stringTable) != 4 {
		t.Fatalf("字符串表长度 = %d, want 4", len(parsed.stringTable))
	}

	icons, err := parseICO(icoData)
	if err != nil {
		t.Fatal(err)
	}
	view := parseResourceSection(t, section)
	if len(view) != 2 {
		t.Fatalf("资源类型数量 = %d, want 2", len(view))
	}
	iconNames, ok := view[rtIcon]
	if !ok {
		t.Fatalf("缺少 RT_ICON (%d) 资源", rtIcon)
	}
	if len(iconNames) != len(icons.frames) {
		t.Fatalf("RT_ICON 数量 = %d, want %d", len(iconNames), len(icons.frames))
	}
	for index, frame := range icons.frames {
		id := uint32(firstIconID + index)
		languages, ok := iconNames[id]
		if !ok {
			t.Fatalf("缺少 RT_ICON id %d", id)
		}
		payload, ok := languages[languageID]
		if !ok {
			t.Fatalf("RT_ICON id %d 缺少语言 %#x", id, languageID)
		}
		if !bytes.Equal(payload, frame.data) {
			t.Fatalf("RT_ICON id %d 载荷与 ICO 帧不一致: %d vs %d 字节", id, len(payload), len(frame.data))
		}
	}
	groupNames, ok := view[rtGroupIcon]
	if !ok {
		t.Fatalf("缺少 RT_GROUP_ICON (%d) 资源", rtGroupIcon)
	}
	if len(groupNames) != 1 {
		t.Fatalf("RT_GROUP_ICON 数量 = %d, want 1", len(groupNames))
	}
	groupPayload, ok := groupNames[groupIconID][languageID]
	if !ok {
		t.Fatalf("缺少 RT_GROUP_ICON id %d", groupIconID)
	}
	if want := groupIconHeader + groupIconEntry*len(icons.frames); len(groupPayload) != want {
		t.Fatalf("GRPICONDIR 长度 = %d, want %d", len(groupPayload), want)
	}
	if reserved := binary.LittleEndian.Uint16(groupPayload[0:]); reserved != 0 {
		t.Fatalf("GRPICONDIR 保留字段 = %d", reserved)
	}
	if kind := binary.LittleEndian.Uint16(groupPayload[2:]); kind != 1 {
		t.Fatalf("GRPICONDIR 类型 = %d, want 1", kind)
	}
	if count := binary.LittleEndian.Uint16(groupPayload[4:]); int(count) != len(icons.frames) {
		t.Fatalf("GRPICONDIR 数量 = %d, want %d", count, len(icons.frames))
	}
	for index, frame := range icons.frames {
		entry := groupPayload[groupIconHeader+groupIconEntry*index:]
		if entry[0] != frame.width || entry[1] != frame.height || entry[2] != frame.colorCount {
			t.Fatalf("GRPICONDIR 第 %d 项尺寸/颜色数不一致: %v", index, entry[:4])
		}
		if size := binary.LittleEndian.Uint32(entry[8:]); int(size) != len(frame.data) {
			t.Fatalf("GRPICONDIR 第 %d 项大小 = %d, want %d", index, size, len(frame.data))
		}
		if id := binary.LittleEndian.Uint16(entry[12:]); int(id) != firstIconID+index {
			t.Fatalf("GRPICONDIR 第 %d 项资源 ID = %d, want %d", index, id, firstIconID+index)
		}
	}
}

func TestGenerateFailsWithoutICO(t *testing.T) {
	if _, err := generate(t.TempDir()); err == nil {
		t.Fatal("缺少 build/icons/logo.ico 时 generate() 未报错")
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	img.Set(1, 1, color.RGBA{R: 0x40, G: 0x50, B: 0x60, A: 0x80})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// testDIB builds a 32bpp ICO-style DIB: BITMAPINFOHEADER with doubled height,
// an XOR bitmap and a 1bpp AND mask. The bytes are deliberately non-trivial so
// a lossy conversion would be detected.
func testDIB(width, height int) []byte {
	xorRow := width * 4
	xorSize := xorRow * height
	maskRow := ((width + 31) / 32) * 4
	maskSize := maskRow * height
	data := make([]byte, 40+xorSize+maskSize)
	binary.LittleEndian.PutUint32(data[0:], 40)
	binary.LittleEndian.PutUint32(data[4:], uint32(width))
	binary.LittleEndian.PutUint32(data[8:], uint32(height*2))
	binary.LittleEndian.PutUint16(data[12:], 1)  // biPlanes
	binary.LittleEndian.PutUint16(data[14:], 32) // biBitCount
	binary.LittleEndian.PutUint32(data[20:], uint32(xorSize))
	for index := 40; index < len(data); index++ {
		data[index] = byte(index)
	}
	return data
}

type icoSpec struct {
	width      uint8
	height     uint8
	colorCount uint8
	planes     uint16
	bitCount   uint16
	payload    []byte
}

func buildICOFixture(specs []icoSpec) []byte {
	header := make([]byte, 6+16*len(specs))
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], uint16(len(specs)))
	offset := len(header)
	payloads := make([]byte, 0)
	for index, spec := range specs {
		entry := header[6+16*index:]
		entry[0] = spec.width
		entry[1] = spec.height
		entry[2] = spec.colorCount
		binary.LittleEndian.PutUint16(entry[4:], spec.planes)
		binary.LittleEndian.PutUint16(entry[6:], spec.bitCount)
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(spec.payload)))
		binary.LittleEndian.PutUint32(entry[12:], uint32(offset))
		offset += len(spec.payload)
		payloads = append(payloads, spec.payload...)
	}
	return append(header, payloads...)
}

func TestBMPFramesKeepDIBAndMask(t *testing.T) {
	pngData := testPNG(t)
	dib := testDIB(4, 2)
	icoData := buildICOFixture([]icoSpec{
		{width: 2, height: 2, planes: 1, bitCount: 32, payload: pngData},
		{width: 4, height: 2, planes: 1, bitCount: 32, payload: dib},
	})
	icons, err := parseICO(icoData)
	if err != nil {
		t.Fatalf("parseICO() error = %v", err)
	}
	section, err := buildResourceSection(icons)
	if err != nil {
		t.Fatalf("buildResourceSection() error = %v", err)
	}
	object, err := writeCOFFObject(section)
	if err != nil {
		t.Fatalf("writeCOFFObject() error = %v", err)
	}
	parsed := parseCOFFObject(t, object)
	view := parseResourceSection(t, parsed.sections[0])

	if payload := view[rtIcon][1][languageID]; !bytes.Equal(payload, pngData) {
		t.Fatalf("PNG 帧 RT_ICON 数据被改写: %d vs %d 字节", len(payload), len(pngData))
	}
	// The DIB must be copied verbatim: header, color data, XOR bitmap and AND
	// mask stay intact because RT_ICON uses the ICO frame payload as-is.
	if payload := view[rtIcon][2][languageID]; !bytes.Equal(payload, dib) {
		t.Fatalf("DIB 帧 RT_ICON 数据被改写: %d vs %d 字节", len(payload), len(dib))
	}
	group := view[rtGroupIcon][groupIconID][languageID]
	if len(group) != groupIconHeader+groupIconEntry*2 {
		t.Fatalf("GRPICONDIR 长度 = %d", len(group))
	}
	if size := binary.LittleEndian.Uint32(group[groupIconHeader+groupIconEntry+8:]); int(size) != len(dib) {
		t.Fatalf("DIB 组图标大小 = %d, want %d", size, len(dib))
	}
}

func TestParseICORejectsMalformedFiles(t *testing.T) {
	valid := buildICOFixture([]icoSpec{{width: 2, height: 2, planes: 1, bitCount: 32, payload: testPNG(t)}})
	mutate := func(offset int, replacements ...byte) []byte {
		data := append([]byte(nil), valid...)
		copy(data[offset:], replacements)
		return data
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "truncated header", data: valid[:4]},
		{name: "reserved", data: mutate(0, 1, 0)},
		{name: "wrong type", data: mutate(2, 2, 0)},
		{name: "no images", data: mutate(4, 0, 0)},
		{name: "truncated directory", data: valid[:10]},
		{name: "zero size", data: mutate(6+8, 0, 0, 0, 0)},
		{name: "out of range offset", data: mutate(6+12, 0xff, 0xff, 0xff, 0x7f)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseICO(test.data); err == nil {
				t.Fatal("parseICO() 接受了损坏的 ICO")
			}
		})
	}
}

func TestParseICORejectsInvalidDIBPayload(t *testing.T) {
	payload := make([]byte, 16)
	icoData := buildICOFixture([]icoSpec{{width: 2, height: 2, payload: payload}})
	if _, err := parseICO(icoData); err == nil {
		t.Fatal("parseICO() 接受了过短的 DIB 数据")
	}
	binary.LittleEndian.PutUint32(payload, 7)
	payload = append(payload, make([]byte, 24)...)
	icoData = buildICOFixture([]icoSpec{{width: 2, height: 2, payload: payload}})
	if _, err := parseICO(icoData); err == nil {
		t.Fatal("parseICO() 接受了未知的 DIB 头")
	}
}
