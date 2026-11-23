package crop

import (
	"image"
	"math"
	"testing"
)

const fw, fh = 1920.0, 1080.0

func TestHitTest(t *testing.T) {
	b := Box{100, 100, 400, 300}
	cases := []struct {
		x, y   float64
		locked bool
		want   Handle
	}{
		{100, 100, false, NW},
		{503, 398, false, SE},
		{300, 101, false, N},
		{300, 101, true, Move}, // no edge handles with a fixed ratio
		{499, 250, false, E},
		{300, 250, false, Move},
		{50, 50, false, None},
		{600, 250, false, None},
	}
	for _, c := range cases {
		if got := HitTest(b, c.x, c.y, 6, c.locked); got != c.want {
			t.Errorf("HitTest(%v, %v, locked=%v) = %v, want %v", c.x, c.y, c.locked, got, c.want)
		}
	}
}

func TestDragMoveStaysInside(t *testing.T) {
	b := Drag(Box{100, 100, 400, 300}, Move, 5000, -5000, 0, fw, fh, 32)
	if b != (Box{fw - 400, 0, 400, 300}) {
		t.Errorf("move = %+v", b)
	}
}

func TestDragFree(t *testing.T) {
	start := Box{100, 100, 400, 300}
	if b := Drag(start, E, 50, 999, 0, fw, fh, 32); b != (Box{100, 100, 450, 300}) {
		t.Errorf("E = %+v", b)
	}
	if b := Drag(start, NW, -500, -500, 0, fw, fh, 32); b != (Box{0, 0, 500, 400}) {
		t.Errorf("NW past the frame = %+v", b)
	}
	if b := Drag(start, W, 1000, 0, 0, fw, fh, 32); b.W != 32 || b.Right() != 500 {
		t.Errorf("W past the opposite edge = %+v, want the minimum width", b)
	}
	if b := Drag(start, S, 0, 5000, 0, fw, fh, 32); b.Bottom() != fh {
		t.Errorf("S past the frame = %+v", b)
	}
}

func TestDragLocked(t *testing.T) {
	start := Box{100, 100, 320, 180} // 16:9
	aspect := 16.0 / 9
	for _, h := range []Handle{NE, NW, SE, SW} {
		for _, d := range [][2]float64{{80, 10}, {-50, -200}, {3000, 3000}, {-3000, -3000}} {
			b := Drag(start, h, d[0], d[1], aspect, fw, fh, 32)
			if math.Abs(b.W/b.H-aspect) > 1e-9 {
				t.Errorf("%v %v: ratio %v, want %v", h, d, b.W/b.H, aspect)
			}
			if b.X < -1e-9 || b.Y < -1e-9 || b.Right() > fw+1e-9 || b.Bottom() > fh+1e-9 {
				t.Errorf("%v %v: %+v leaves the frame", h, d, b)
			}
		}
	}
	// The opposite corner stays put.
	b := Drag(start, SE, 160, 0, aspect, fw, fh, 32)
	if b.X != 100 || b.Y != 100 || b.W != 480 {
		t.Errorf("SE = %+v, want the top-left corner fixed and width 480", b)
	}
	b = Drag(start, NW, -160, 0, aspect, fw, fh, 32)
	if b.Right() != 420 || b.Bottom() != 280 {
		t.Errorf("NW = %+v, want the bottom-right corner fixed", b)
	}
}

func TestFit(t *testing.T) {
	b := Fit(9.0/16, fw/2, fh/2, fw, fh)
	if b.H != fh || math.Abs(b.W-607.5) > 1e-9 || math.Abs(b.X-(fw-b.W)/2) > 1e-9 {
		t.Errorf("9:16 fit = %+v", b)
	}
	b = Fit(1, 0, 0, fw, fh)
	if b != (Box{0, 0, fh, fh}) {
		t.Errorf("1:1 near the corner = %+v", b)
	}
	if b := Fit(fw/fh, 10, 10, fw, fh); b != Full(fw, fh) {
		t.Errorf("same ratio = %+v, want the full frame", b)
	}
}

func TestEven(t *testing.T) {
	r := Box{101.4, 57, 607.5, 1080}.Even(1920, 1080)
	if r.Min.X%2 != 0 || r.Min.Y%2 != 0 || r.Dx()%2 != 0 || r.Dy()%2 != 0 {
		t.Errorf("Even = %v, want even position and size", r)
	}
	if r.Max.Y > 1080 || r.Max.X > 1920 {
		t.Errorf("Even = %v leaves the frame", r)
	}
	if got := Full(1920, 1080).Even(1920, 1080); got != image.Rect(0, 0, 1920, 1080) {
		t.Errorf("full frame = %v", got)
	}
	// An odd frame size still gives an even crop inside it.
	if got := Full(1279, 719).Even(1279, 719); got.Dx() != 1278 || got.Dy() != 718 {
		t.Errorf("odd frame = %v", got)
	}
}
