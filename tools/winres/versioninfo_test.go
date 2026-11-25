package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    [4]uint16
		wantErr bool
	}{
		{"1.0.1", [4]uint16{1, 0, 1, 0}, false},
		{"2.10", [4]uint16{2, 10, 0, 0}, false},
		{"1.2.3.4", [4]uint16{1, 2, 3, 4}, false},
		{"", [4]uint16{}, true},
		{"1.2.3.4.5", [4]uint16{}, true},
		{"1.x", [4]uint16{}, true},
		{"1.70000", [4]uint16{}, true},
	}
	for _, tt := range tests {
		got, err := parseVersion(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseVersion(%q) error = %v, want error %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestVersionInfo(t *testing.T) {
	data, err := versionInfo(versionData{
		Version:     "1.0.1",
		Description: "Simple Clip Trimmer",
		Product:     "Simple Clip Trimmer",
		Company:     "Matheus Nascimento",
		Copyright:   "Copyright",
		FileName:    "Simple Clip Trimmer.exe",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The first word is the length of the whole structure.
	if got := int(binary.LittleEndian.Uint16(data)); got != len(data) {
		t.Errorf("length field = %d, want %d", got, len(data))
	}
	// The fixed part follows the key and its padding.
	if sig := binary.LittleEndian.Uint32(data[40:]); sig != 0xFEEF04BD {
		t.Errorf("signature = %#x, want 0xFEEF04BD", sig)
	}
	for _, s := range []string{
		"VS_VERSION_INFO", "040904B0", "FileDescription", "Simple Clip Trimmer",
		"OriginalFilename", "Simple Clip Trimmer.exe", "FileVersion", "1.0.1.0",
	} {
		if !bytes.Contains(data, utf16z(s)[:len(utf16z(s))-2]) {
			t.Errorf("version resource lacks %q", s)
		}
	}
}
