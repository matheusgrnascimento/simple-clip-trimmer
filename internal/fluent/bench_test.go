package fluent

import (
	"image"
	"image/color"
	"testing"
)

// Sizes of the video area at 100% and 150% display scaling.
var benchSizes = []struct {
	name string
	w, h int
}{
	{"100%", 1184, 637},
	{"150%", 1776, 956},
}

var (
	procGetDC                  = user32.NewProc("GetDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
)

// offscreenDC returns a memory device context with a w×h bitmap selected,
// so the benchmark never draws on the screen.
func offscreenDC(b *testing.B, w, h int) uintptr {
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	dc, _, _ := procCreateCompatibleDC.Call(screen)
	bmp, _, _ := procCreateCompatibleBitmap.Call(screen, uintptr(w), uintptr(h))
	if dc == 0 || bmp == 0 {
		b.Fatal("could not create an off-screen bitmap")
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	b.Cleanup(func() {
		procSelectObject.Call(dc, old)
		procDeleteObject.Call(bmp)
		procDeleteDC.Call(dc)
	})
	return dc
}

// BenchmarkBlit measures copying a painted frame to a device context, as
// every repaint does.
func BenchmarkBlit(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			dc := offscreenDC(b, s.w, s.h)
			p := NewPainter(nil, s.w, s.h, 1)
			for b.Loop() {
				if err := p.Blit(dc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCropPaint measures drawing one crop-mode frame: the scaled
// video frame, the scrim around the box and the box handles.
func BenchmarkCropPaint(b *testing.B) {
	for _, s := range benchSizes {
		b.Run(s.name, func(b *testing.B) {
			p := NewPainter(nil, s.w, s.h, float64(s.w)/1184)
			frame := image.NewRGBA(image.Rect(0, 0, s.w-48, s.h-48))
			for b.Loop() {
				p.Fill(videoBlack)
				p.DrawImage(frame, 24, 24)
				p.FillRect(Rect{24, 24, float64(s.w - 48), 100}, scrim)
				p.FillRect(Rect{24, float64(s.h - 124), float64(s.w - 48), 100}, scrim)
				p.FillRect(Rect{24, 124, 200, float64(s.h - 248)}, scrim)
				p.FillRect(Rect{float64(s.w - 224), 124, 200, float64(s.h - 248)}, scrim)
				for range 8 {
					p.BorderedRoundRect(Rect{300, 300, 12, 12}, 3, color.NRGBA{255, 255, 255, 255}, scrim)
				}
			}
		})
	}
}
