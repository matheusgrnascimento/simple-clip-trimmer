package fluent

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Icon glyphs of the Segoe Fluent Icons font (same code points in Segoe
// MDL2 Assets on Windows 10).
const (
	IconPlay        = ""
	IconPause       = ""
	IconOpenFile    = ""
	IconChevronDown = ""
	IconVideo       = ""
	IconCrop        = ""
)

// Fonts loads the Windows UI and icon fonts and caches faces by pixel size.
type Fonts struct {
	text, icons *opentype.Font

	mu    sync.Mutex
	faces map[faceKey]font.Face
}

type faceKey struct {
	icons bool
	px    int
}

// LoadFonts loads Segoe UI Variable and Segoe Fluent Icons from the
// Windows fonts folder, falling back to the Windows 10 fonts.
func LoadFonts() (*Fonts, error) {
	dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	text, err := loadFirst(dir, "SegUIVar.ttf", "segoeui.ttf")
	if err != nil {
		return nil, err
	}
	icons, err := loadFirst(dir, "SegoeIcons.ttf", "segmdl2.ttf")
	if err != nil {
		return nil, err
	}
	return &Fonts{text: text, icons: icons, faces: map[faceKey]font.Face{}}, nil
}

func loadFirst(dir string, names ...string) (*opentype.Font, error) {
	var lastErr error
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			lastErr = err
			continue
		}
		f, err := opentype.Parse(b)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", n, err)
			continue
		}
		return f, nil
	}
	return nil, fmt.Errorf("no usable font among %v: %w", names, lastErr)
}

// Text returns the UI text face at a size in pixels.
func (f *Fonts) Text(px float64) font.Face { return f.face(false, px) }

// Icons returns the icon face at a size in pixels.
func (f *Fonts) Icons(px float64) font.Face { return f.face(true, px) }

func (f *Fonts) face(icons bool, px float64) font.Face {
	key := faceKey{icons, int(math.Round(px))}
	f.mu.Lock()
	defer f.mu.Unlock()
	if face, ok := f.faces[key]; ok {
		return face
	}
	src := f.text
	if icons {
		src = f.icons
	}
	// DPI 72 makes Size a size in pixels.
	face, err := opentype.NewFace(src, &opentype.FaceOptions{Size: float64(key.px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		// The fonts parsed already; NewFace only fails on invalid options,
		// which are constant here.
		panic(fmt.Sprintf("fluent: face %+v: %v", key, err))
	}
	f.faces[key] = face
	return face
}
