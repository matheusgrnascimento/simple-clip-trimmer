package fluent

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Rect is a rectangle in device pixels, with float coordinates so shapes
// can be laid out with DPI scaling without accumulating rounding errors.
type Rect struct{ X, Y, W, H float64 }

// Right returns the right edge.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the bottom edge.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// CenterY returns the vertical center.
func (r Rect) CenterY() float64 { return r.Y + r.H/2 }

// Contains reports whether the point is inside the rectangle.
func (r Rect) Contains(x, y float64) bool {
	return x >= r.X && x < r.Right() && y >= r.Y && y < r.Bottom()
}

// Painter draws anti-aliased shapes and text into an RGBA image.
type Painter struct {
	Img   *image.RGBA
	Scale float64 // device pixels per DIP (DPI / 96)
	ras   vector.Rasterizer
	bgra  []byte // Blit's conversion buffer, reused between repaints
}

// NewPainter returns a painter for an image of the given size, reusing the
// previous one's pixel buffer when the size did not change.
func NewPainter(prev *Painter, w, h int, scale float64) *Painter {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if prev != nil && prev.Img.Rect.Dx() == w && prev.Img.Rect.Dy() == h {
		prev.Scale = scale
		return prev
	}
	return &Painter{Img: image.NewRGBA(image.Rect(0, 0, w, h)), Scale: scale}
}

// S converts DIPs to device pixels.
func (p *Painter) S(dip float64) float64 { return dip * p.Scale }

// Fill paints the whole image with an opaque color.
func (p *Painter) Fill(c color.NRGBA) {
	draw.Draw(p.Img, p.Img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
}

// DrawImage copies an image with its top-left corner at (x, y).
func (p *Painter) DrawImage(src image.Image, x, y int) {
	r := src.Bounds().Sub(src.Bounds().Min).Add(image.Pt(x, y))
	draw.Draw(p.Img, r, src, src.Bounds().Min, draw.Src)
}

// FillRect paints a pixel-aligned rectangle.
func (p *Painter) FillRect(r Rect, c color.NRGBA) {
	ir := image.Rect(round(r.X), round(r.Y), round(r.Right()), round(r.Bottom()))
	draw.Draw(p.Img, ir, image.NewUniform(c), image.Point{}, draw.Over)
}

// RoundRect paints a rectangle with rounded corners.
func (p *Painter) RoundRect(r Rect, radius float64, c color.NRGBA) {
	radius = math.Min(radius, math.Min(r.W, r.H)/2)
	p.fillPath(r, c, func(z *vector.Rasterizer, ox, oy float32) {
		x0, y0 := float32(r.X)-ox, float32(r.Y)-oy
		x1, y1 := x0+float32(r.W), y0+float32(r.H)
		rr := float32(radius)
		k := rr * 0.4477 // 1 - 0.5523: control point offset for a circle arc
		z.MoveTo(x0+rr, y0)
		z.LineTo(x1-rr, y0)
		z.CubeTo(x1-k, y0, x1, y0+k, x1, y0+rr)
		z.LineTo(x1, y1-rr)
		z.CubeTo(x1, y1-k, x1-k, y1, x1-rr, y1)
		z.LineTo(x0+rr, y1)
		z.CubeTo(x0+k, y1, x0, y1-k, x0, y1-rr)
		z.LineTo(x0, y0+rr)
		z.CubeTo(x0, y0+k, x0+k, y0, x0+rr, y0)
		z.ClosePath()
	})
}

// BorderedRoundRect paints a rounded rectangle with a 1 px border.
func (p *Painter) BorderedRoundRect(r Rect, radius float64, fill, border color.NRGBA) {
	p.RoundRect(r, radius, border)
	b := math.Max(1, math.Round(p.Scale))
	p.RoundRect(Rect{r.X + b, r.Y + b, r.W - 2*b, r.H - 2*b}, radius-b, fill)
}

// Circle paints a filled circle.
func (p *Painter) Circle(cx, cy, radius float64, c color.NRGBA) {
	p.RoundRect(Rect{cx - radius, cy - radius, 2 * radius, 2 * radius}, radius, c)
}

// fillPath rasterizes a path drawn inside the bounding box r (plus a pixel
// of margin for anti-aliasing) and composites it with color c.
func (p *Painter) fillPath(r Rect, c color.NRGBA, path func(z *vector.Rasterizer, ox, oy float32)) {
	bx0, by0 := int(math.Floor(r.X))-1, int(math.Floor(r.Y))-1
	bx1, by1 := int(math.Ceil(r.Right()))+1, int(math.Ceil(r.Bottom()))+1
	box := image.Rect(bx0, by0, bx1, by1).Intersect(p.Img.Rect)
	if box.Empty() {
		return
	}
	p.ras.Reset(bx1-bx0, by1-by0)
	path(&p.ras, float32(bx0), float32(by0))
	p.ras.Draw(p.Img, box, image.NewUniform(c), image.Point{})
}

// Text draws a single line of text with its left edge at x, vertically
// centered on cy.
func (p *Painter) Text(face font.Face, s string, x, cy float64, c color.NRGBA) {
	m := face.Metrics()
	baseline := cy + float64(m.Ascent-m.Descent)/128 // (ascent - descent) / 2 in 26.6
	d := font.Drawer{
		Dst:  p.Img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.I(int(math.Round(baseline)))},
	}
	d.DrawString(s)
}

// TextCentered draws text centered horizontally on cx.
func (p *Painter) TextCentered(face font.Face, s string, cx, cy float64, c color.NRGBA) {
	p.Text(face, s, cx-Measure(face, s)/2, cy, c)
}

// Measure returns the advance width of s in pixels.
func Measure(face font.Face, s string) float64 {
	return float64(font.MeasureString(face, s)) / 64
}

// Ellipsize shortens s with a trailing "…" so it fits in width pixels.
func Ellipsize(face font.Face, s string, width float64) string {
	if Measure(face, s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		t := string(r) + "…"
		if Measure(face, t) <= width {
			return t
		}
	}
	return ""
}

func round(f float64) int { return int(math.Round(f)) }
