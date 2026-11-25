package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// versionData is the text of the version resource. Task Manager lists a
// process by FileDescription; Explorer and Settings > Apps show ProductName,
// CompanyName and the version.
type versionData struct {
	Version     string // major.minor.patch, with up to four numeric parts
	Description string
	Product     string
	Company     string
	Copyright   string
	FileName    string // the name of the .exe, for OriginalFilename and InternalName
}

const (
	langIDEnUS  = 0x0409
	codePageUni = 0x04B0
)

// versionInfo encodes d as a VS_VERSIONINFO resource.
func versionInfo(d versionData) ([]byte, error) {
	nums, err := parseVersion(d.Version)
	if err != nil {
		return nil, err
	}
	ms := uint32(nums[0])<<16 | uint32(nums[1])
	ls := uint32(nums[2])<<16 | uint32(nums[3])
	var fixed bytes.Buffer
	for _, v := range []uint32{
		0xFEEF04BD, // signature
		0x00010000, // structure version
		ms, ls,     // file version
		ms, ls, // product version
		0x3F,       // flags mask
		0,          // flags
		0x00040004, // VOS_NT_WINDOWS32
		1,          // VFT_APP
		0,          // subtype
		0, 0,       // date
	} {
		binary.Write(&fixed, binary.LittleEndian, v)
	}
	full := fmt.Sprintf("%d.%d.%d.%d", nums[0], nums[1], nums[2], nums[3])
	name := strings.TrimSuffix(d.FileName, ".exe")
	table := block(fmt.Sprintf("%04X%04X", langIDEnUS, codePageUni), nil, 0, true,
		textBlock("CompanyName", d.Company),
		textBlock("FileDescription", d.Description),
		textBlock("FileVersion", full),
		textBlock("InternalName", name),
		textBlock("LegalCopyright", d.Copyright),
		textBlock("OriginalFilename", name+".exe"),
		textBlock("ProductName", d.Product),
		textBlock("ProductVersion", d.Version),
	)
	stringInfo := block("StringFileInfo", nil, 0, true, table)
	var translation bytes.Buffer
	binary.Write(&translation, binary.LittleEndian, [2]uint16{langIDEnUS, codePageUni})
	varInfo := block("VarFileInfo", nil, 0, true,
		block("Translation", translation.Bytes(), 4, false))
	return block("VS_VERSION_INFO", fixed.Bytes(), uint16(fixed.Len()), false, stringInfo, varInfo), nil
}

// parseVersion reads up to four dot-separated numbers; missing parts are 0.
func parseVersion(s string) ([4]uint16, error) {
	var nums [4]uint16
	parts := strings.Split(s, ".")
	if s == "" || len(parts) > len(nums) {
		return nums, fmt.Errorf("invalid version %q: want major.minor.patch", s)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return nums, fmt.Errorf("invalid version %q: %w", s, err)
		}
		nums[i] = uint16(n)
	}
	return nums, nil
}

// textBlock is a String entry of the string table.
func textBlock(key, value string) []byte {
	return block(key, utf16z(value), uint16(len(utf16.Encode([]rune(value)))+1), true)
}

// block encodes one node of the version resource tree: the length, the
// value length (in characters for text, bytes otherwise), the type, the key
// and the value, then the children, each aligned to 4 bytes.
func block(key string, value []byte, valueLen uint16, text bool, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0}) // length, patched below
	binary.Write(&b, binary.LittleEndian, valueLen)
	var kind uint16
	if text {
		kind = 1
	}
	binary.Write(&b, binary.LittleEndian, kind)
	b.Write(utf16z(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// utf16z encodes s as zero-terminated UTF-16LE.
func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, 2*len(u)+2)
	for _, c := range u {
		out = append(out, byte(c), byte(c>>8))
	}
	return append(out, 0, 0)
}
