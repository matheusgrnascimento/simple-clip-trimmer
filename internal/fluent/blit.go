package fluent

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32                 = windows.NewLazySystemDLL("gdi32.dll")
	procSetDIBitsToDevice = gdi32.NewProc("SetDIBitsToDevice")
)

// bitmapInfoHeader is BITMAPINFOHEADER (wingdi.h).
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Blit copies the painter's image to a device context at (0, 0) in a single
// SetDIBitsToDevice call. It converts the pixels from RGBA to the BGRA order
// GDI expects into a buffer reused between calls, so a repaint allocates
// nothing. The image is opaque: every pixel is written.
func (p *Painter) Blit(hdc uintptr) error {
	w, h := p.Img.Rect.Dx(), p.Img.Rect.Dy()
	n := len(p.Img.Pix)
	if n == 0 || p.Img.Stride != 4*w {
		return errors.New("fluent: blit needs a non-empty image without padding")
	}
	if cap(p.bgra) < n {
		p.bgra = make([]byte, n)
	}
	p.bgra = p.bgra[:n]
	swapRB(p.bgra, p.Img.Pix)

	bi := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32} // negative height: top-down rows
	bi.Size = uint32(unsafe.Sizeof(bi))
	const dibRGBColors = 0
	r, _, err := procSetDIBitsToDevice.Call(hdc, 0, 0, uintptr(w), uintptr(h), 0, 0, 0, uintptr(h),
		uintptr(unsafe.Pointer(&p.bgra[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors)
	if r == 0 {
		return err
	}
	return nil
}

// swapRB converts RGBA pixels to BGRA, four bytes at a time.
func swapRB(dst, src []byte) {
	d := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), len(dst)/4)
	s := unsafe.Slice((*uint32)(unsafe.Pointer(&src[0])), len(src)/4)
	for i, v := range s {
		// Little endian: v is A B G R from the high byte down; GDI wants A R G B.
		d[i] = v&0xFF00FF00 | v>>16&0xFF | v&0xFF<<16
	}
}
