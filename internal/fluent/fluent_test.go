package fluent

import (
	"image/color"
	"testing"
)

func TestNewPaletteAccent(t *testing.T) {
	ap := make([]byte, 32)
	for i := range 8 {
		ap[i*4], ap[i*4+1], ap[i*4+2], ap[i*4+3] = byte(i), byte(10+i), byte(20+i), 0xFF
	}
	light := NewPalette(false, ap)
	if light.Accent != (color.NRGBA{4, 14, 24, 0xFF}) {
		t.Errorf("light accent = %v, want Dark1 (index 4)", light.Accent)
	}
	dark := NewPalette(true, ap)
	if dark.Accent != (color.NRGBA{1, 11, 21, 0xFF}) || !dark.Dark {
		t.Errorf("dark accent = %v, want Light2 (index 1)", dark.Accent)
	}
	if NewPalette(false, nil).Accent != defaultAccentLight {
		t.Error("a missing accent palette must use the default accent")
	}
}

func TestCOLORREF(t *testing.T) {
	if got := COLORREF(color.NRGBA{0x11, 0x22, 0x33, 0xFF}); got != 0x332211 {
		t.Errorf("COLORREF = %#x, want 0x332211", got)
	}
}

func TestRoundRectAntialiasing(t *testing.T) {
	p := NewPainter(nil, 40, 40, 1)
	p.Fill(color.NRGBA{0, 0, 0, 0xFF})
	p.RoundRect(Rect{10, 10, 20, 20}, 6, color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF})

	if c := p.Img.RGBAAt(20, 20); c.R != 0xFF {
		t.Errorf("center = %v, want white", c)
	}
	if c := p.Img.RGBAAt(5, 5); c.R != 0 {
		t.Errorf("outside = %v, want black", c)
	}
	// The corner pixel is outside the rounded corner.
	if c := p.Img.RGBAAt(10, 10); c.R > 0x40 {
		t.Errorf("corner = %v, want (almost) black", c)
	}
	// A pixel on the curved edge (its center is 5.7 px from the arc center,
	// the radius is 6) is partially covered.
	if c := p.Img.RGBAAt(11, 12); c.R == 0 || c.R == 0xFF {
		t.Errorf("edge = %v, want a blended value", c)
	}
}

func TestNewPainterReusesBuffer(t *testing.T) {
	a := NewPainter(nil, 10, 10, 1)
	if b := NewPainter(a, 10, 10, 2); b != a || b.Scale != 2 {
		t.Error("same size must reuse the painter")
	}
	if c := NewPainter(a, 11, 10, 1); c == a {
		t.Error("a new size must allocate a new painter")
	}
}

func TestSwapRB(t *testing.T) {
	src := []byte{0x11, 0x22, 0x33, 0xFF, 0xAA, 0xBB, 0xCC, 0x80}
	dst := make([]byte, len(src))
	swapRB(dst, src)
	want := []byte{0x33, 0x22, 0x11, 0xFF, 0xCC, 0xBB, 0xAA, 0x80}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("swapRB = % x, want % x", dst, want)
		}
	}
}

func TestRectContains(t *testing.T) {
	r := Rect{10, 10, 5, 5}
	if !r.Contains(10, 14.9) || r.Contains(15, 12) || r.Contains(9.9, 12) {
		t.Error("Contains must include the top-left edge and exclude the bottom-right one")
	}
}

func TestFontsAndEllipsize(t *testing.T) {
	f, err := LoadFonts()
	if err != nil {
		t.Skip("Windows fonts not available:", err)
	}
	face := f.Text(14)
	if f.Text(14) != face {
		t.Error("faces must be cached")
	}
	full := "2560×1440 · 60 fps · hevc · 2 audio tracks"
	w := Measure(face, full)
	if w <= 0 {
		t.Fatal("text must have a width")
	}
	short := Ellipsize(face, full, w/2)
	if Measure(face, short) > w/2 || short == full || []rune(short)[len([]rune(short))-1] != '…' {
		t.Errorf("Ellipsize = %q", short)
	}
	if Ellipsize(face, full, w+1) != full {
		t.Error("text that fits must not change")
	}
	if Measure(f.Icons(16), IconPlay) <= 0 {
		t.Error("the icon font must have the play glyph")
	}
}
