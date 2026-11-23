// Package crop holds the geometry of the crop box: hit testing, moving and
// resizing it with the mouse (free or with a fixed aspect ratio), and
// rounding it to the even sizes video encoders need.
//
// Every value is in source video pixels.
package crop

import (
	"image"
	"math"
)

// Box is a crop rectangle being edited, in source pixels.
type Box struct{ X, Y, W, H float64 }

// Right returns the right edge.
func (b Box) Right() float64 { return b.X + b.W }

// Bottom returns the bottom edge.
func (b Box) Bottom() float64 { return b.Y + b.H }

// Full returns the box covering the whole frame.
func Full(fw, fh float64) Box { return Box{0, 0, fw, fh} }

// FromRect converts a committed crop to a box.
func FromRect(r image.Rectangle) Box {
	return Box{float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())}
}

// Handle is the part of the box the mouse grabbed.
type Handle int

// Handles. The edges and corners are named after compass points.
const (
	None Handle = iota
	Move
	N
	S
	E
	W
	NE
	NW
	SE
	SW
)

// HitTest returns the handle at point (x, y). tol is the grab distance.
// With locked set (fixed aspect ratio) only the corners resize.
func HitTest(b Box, x, y, tol float64, locked bool) Handle {
	near := func(v, edge float64) bool { return math.Abs(v-edge) <= tol }
	inX := x >= b.X-tol && x <= b.Right()+tol
	inY := y >= b.Y-tol && y <= b.Bottom()+tol
	if !inX || !inY {
		return None
	}
	n, s := near(y, b.Y), near(y, b.Bottom())
	w, e := near(x, b.X), near(x, b.Right())
	switch {
	case n && w:
		return NW
	case n && e:
		return NE
	case s && w:
		return SW
	case s && e:
		return SE
	}
	if !locked {
		switch {
		case n:
			return N
		case s:
			return S
		case w:
			return W
		case e:
			return E
		}
	}
	if x > b.X && x < b.Right() && y > b.Y && y < b.Bottom() {
		return Move
	}
	return None
}

// Drag returns the box that results from dragging handle h of start by
// (dx, dy), kept inside the fw×fh frame and at least min pixels on each
// side. aspect is the width/height ratio to keep, or 0 for a free box.
func Drag(start Box, h Handle, dx, dy, aspect, fw, fh, min float64) Box {
	switch h {
	case Move:
		b := start
		b.X = clamp(start.X+dx, 0, fw-start.W)
		b.Y = clamp(start.Y+dy, 0, fh-start.H)
		return b
	case None:
		return start
	}
	if aspect > 0 {
		return dragLocked(start, h, dx, dy, aspect, fw, fh, min)
	}

	x0, y0, x1, y1 := start.X, start.Y, start.Right(), start.Bottom()
	switch h {
	case W, NW, SW:
		x0 = clamp(x0+dx, 0, x1-min)
	case E, NE, SE:
		x1 = clamp(x1+dx, x0+min, fw)
	}
	switch h {
	case N, NE, NW:
		y0 = clamp(y0+dy, 0, y1-min)
	case S, SE, SW:
		y1 = clamp(y1+dy, y0+min, fh)
	}
	return Box{x0, y0, x1 - x0, y1 - y0}
}

// dragLocked resizes from a corner keeping the aspect ratio, with the
// opposite corner fixed.
func dragLocked(start Box, h Handle, dx, dy, aspect, fw, fh, min float64) Box {
	// Anchor (fixed corner) and growth direction of the dragged corner.
	ax, sx := start.X, 1.0
	if h == NW || h == SW {
		ax, sx = start.Right(), -1
	}
	ay, sy := start.Y, 1.0
	if h == NW || h == NE {
		ay, sy = start.Bottom(), -1
	}

	// Follow whichever axis the mouse moved more along, in box terms.
	w := math.Max(start.W+sx*dx, (start.H+sy*dy)*aspect)

	// Room from the anchor to the frame edges in the growth direction.
	maxW := ax
	if sx > 0 {
		maxW = fw - ax
	}
	maxH := ay
	if sy > 0 {
		maxH = fh - ay
	}
	w = math.Min(w, math.Min(maxW, maxH*aspect))
	w = math.Max(w, math.Max(min, min*aspect))
	hgt := w / aspect

	b := Box{ax, ay, w, hgt}
	if sx < 0 {
		b.X = ax - w
	}
	if sy < 0 {
		b.Y = ay - hgt
	}
	return b
}

// Fit returns the largest box with the aspect ratio that fits in the frame,
// centered as close as possible to (cx, cy).
func Fit(aspect, cx, cy, fw, fh float64) Box {
	w, h := fw, fw/aspect
	if h > fh {
		w, h = fh*aspect, fh
	}
	return Box{clamp(cx-w/2, 0, fw-w), clamp(cy-h/2, 0, fh-h), w, h}
}

// Even rounds the box to the frame's pixel grid with even position and
// size (4:2:0 video needs even dimensions), staying inside the frame.
func (b Box) Even(fw, fh int) image.Rectangle {
	x := evenDown(b.X)
	y := evenDown(b.Y)
	w := evenNearest(b.W)
	h := evenNearest(b.H)
	w = max(2, min(w, fw-x-(fw-x)%2))
	h = max(2, min(h, fh-y-(fh-y)%2))
	return image.Rect(x, y, x+w, y+h)
}

func evenDown(v float64) int    { return int(math.Floor(v/2)) * 2 }
func evenNearest(v float64) int { return int(math.Round(v/2)) * 2 }

func clamp(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	return math.Min(math.Max(v, lo), hi)
}
