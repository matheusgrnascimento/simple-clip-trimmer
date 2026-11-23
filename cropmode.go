package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"log/slog"
	"math"
	"os"
	"path/filepath"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	xdraw "golang.org/x/image/draw"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/crop"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
)

// minCropSize is the smallest crop side, in source pixels.
const minCropSize = 32

// cropAspects are the aspect ratios offered while cropping. 0 is a free
// box; -1 is the source's own ratio.
var cropAspects = []struct {
	Name  string
	Ratio float64
}{
	{"Free", 0},
	{"Original", -1},
	{"16:9", 16.0 / 9},
	{"9:16", 9.0 / 16},
	{"1:1", 1},
	{"4:5", 4.0 / 5},
	{"4:3", 4.0 / 3},
}

// cropState is the state of crop mode, where the video area shows a still
// frame with the crop box over it.
type cropState struct {
	active bool
	box    crop.Box
	aspect int              // index in cropAspects
	prev   *image.Rectangle // crop before entering, restored on cancel

	frame     image.Image
	scaled    *image.RGBA     // frame scaled to scaledFor
	scaledFor image.Rectangle // display rectangle the scaled frame fits
	shotSeq   int

	drag      crop.Handle
	dragStart crop.Box
	dragX     float64
	dragY     float64
}

// frameSize returns the source picture size.
func (a *app) frameSize() (float64, float64) {
	return float64(a.info.Width), float64(a.info.Height)
}

// aspectRatio returns the locked width/height ratio, or 0 for a free box.
func (a *app) aspectRatio() float64 {
	r := cropAspects[a.cr.aspect].Ratio
	if r < 0 {
		fw, fh := a.frameSize()
		return fw / fh
	}
	return r
}

func (a *app) enterCrop() {
	if a.info == nil || a.exporting() || a.cr.active {
		return
	}
	if !a.paused {
		if err := a.player.SetProperty("pause", "yes"); err != nil {
			slog.Warn("pause for crop", "err", err)
		}
	}
	fw, fh := a.frameSize()
	a.cr = cropState{active: true, aspect: a.cr.aspect, prev: a.cropRect, box: crop.Full(fw, fh)}
	if a.cropRect != nil {
		a.cr.box = crop.FromRect(*a.cropRect)
	}
	// The still frame must show the whole picture, without the preview crop.
	a.setVideoCrop(nil)
	a.showVideo(false)
	a.grabFrame()
	a.stage.Invalidate()
	a.refresh()
}

// exitCrop leaves crop mode, applying the box or restoring the previous crop.
func (a *app) exitCrop(apply bool) {
	if !a.cr.active {
		return
	}
	if apply {
		fw, fh := a.frameSize()
		r := a.cr.box.Even(a.info.Width, a.info.Height)
		if r == crop.Full(fw, fh).Even(a.info.Width, a.info.Height) {
			a.cropRect = nil
		} else {
			a.cropRect = &r
		}
	} else {
		a.cropRect = a.cr.prev
	}
	if _, h := a.outputSize(); a.height >= h {
		a.height = 0 // never upscale: keep the cropped size
	}
	a.cr = cropState{aspect: a.cr.aspect}
	a.setVideoCrop(a.cropRect)
	a.showVideo(true)
	a.stage.Invalidate()
	a.refresh()
}

func (a *app) resetCrop() {
	fw, fh := a.frameSize()
	a.cr.box = crop.Full(fw, fh)
	a.cr.aspect = 0
	a.stage.Invalidate()
	a.refresh()
}

// outputSize returns the picture size before scaling: the crop, or the
// whole source.
func (a *app) outputSize() (int, int) {
	if a.cropRect != nil {
		return a.cropRect.Dx(), a.cropRect.Dy()
	}
	if a.info == nil {
		return 0, 0
	}
	return a.info.Width, a.info.Height
}

// setVideoCrop makes the mpv preview show only the crop (nil: everything).
func (a *app) setVideoCrop(r *image.Rectangle) {
	v := ""
	if r != nil {
		v = fmt.Sprintf("%dx%d+%d+%d", r.Dx(), r.Dy(), r.Min.X, r.Min.Y)
	}
	if err := a.player.SetProperty("video-crop", v); err != nil {
		slog.Warn("mpv video-crop", "value", v, "err", err)
	}
}

// showVideo shows or hides mpv's video window (a child of the video area).
func (a *app) showVideo(show bool) {
	child := win.GetWindow(win.HWND(a.stage.Handle()), win.GW_CHILD)
	if child == 0 {
		return
	}
	cmd := int32(win.SW_HIDE)
	if show {
		cmd = win.SW_SHOWNA
	}
	win.ShowWindow(child, cmd)
}

// grabFrame saves the current video frame with mpv and shows it as the
// still picture of crop mode. It runs off the UI thread; a newer request
// replaces an older one.
func (a *app) grabFrame() {
	a.cr.shotSeq++
	seq := a.cr.shotSeq
	path := filepath.Join(os.TempDir(), fmt.Sprintf("simple-clip-trimmer-%d-%d.jpg", os.Getpid(), seq))
	go func() {
		img, err := a.readFrame(path)
		a.mw.Synchronize(func() {
			if !a.cr.active || seq != a.cr.shotSeq {
				return
			}
			if err != nil {
				a.flash("Couldn't show the frame: " + err.Error())
				return
			}
			a.cr.frame, a.cr.scaled = img, nil
			a.stage.Invalidate()
		})
	}()
}

func (a *app) readFrame(path string) (image.Image, error) {
	// The format follows the file extension; "video" leaves out subtitles
	// and the OSD.
	if err := a.player.Command("screenshot-to-file", path, "video"); err != nil {
		return nil, err
	}
	defer os.Remove(path)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jpeg.Decode(f)
}

// cropDisplay returns where the frame is drawn in the video area and the
// scale from source pixels to device pixels.
func (a *app) cropDisplay(p *fluent.Painter) (fluent.Rect, float64) {
	fw, fh := a.frameSize()
	w, h := float64(p.Img.Rect.Dx()), float64(p.Img.Rect.Dy())
	m := p.S(24) // room for the handles at the edges
	k := math.Max(math.Min((w-2*m)/fw, (h-2*m)/fh), 0.01)
	dw, dh := math.Round(fw*k), math.Round(fh*k)
	return fluent.Rect{X: math.Round((w - dw) / 2), Y: math.Round((h - dh) / 2), W: dw, H: dh}, k
}

func (a *app) paintCrop(p *fluent.Painter) {
	pal := a.pal
	p.Fill(pal.Video)
	disp, k := a.cropDisplay(p)
	dr := image.Rect(int(disp.X), int(disp.Y), int(disp.Right()), int(disp.Bottom()))

	if a.cr.frame == nil {
		// The box already works while mpv saves the frame.
		p.TextCentered(a.fonts.Text(p.S(14)), "Loading the frame…", disp.X+disp.W/2, disp.CenterY(), pal.CropGuide)
	} else {
		if a.cr.scaled == nil || a.cr.scaledFor != dr {
			a.cr.scaled = image.NewRGBA(image.Rect(0, 0, dr.Dx(), dr.Dy()))
			xdraw.ApproxBiLinear.Scale(a.cr.scaled, a.cr.scaled.Rect, a.cr.frame, a.cr.frame.Bounds(), xdraw.Src, nil)
			a.cr.scaledFor = dr
		}
		xdraw.Draw(p.Img, dr, a.cr.scaled, image.Point{}, xdraw.Src)
	}

	// Box in device pixels.
	b := a.cr.box
	bx := fluent.Rect{X: disp.X + b.X*k, Y: disp.Y + b.Y*k, W: b.W * k, H: b.H * k}

	// Dim what will be cut off.
	p.FillRect(fluent.Rect{X: disp.X, Y: disp.Y, W: disp.W, H: bx.Y - disp.Y}, pal.Scrim)
	p.FillRect(fluent.Rect{X: disp.X, Y: bx.Bottom(), W: disp.W, H: disp.Bottom() - bx.Bottom()}, pal.Scrim)
	p.FillRect(fluent.Rect{X: disp.X, Y: bx.Y, W: bx.X - disp.X, H: bx.H}, pal.Scrim)
	p.FillRect(fluent.Rect{X: bx.Right(), Y: bx.Y, W: disp.Right() - bx.Right(), H: bx.H}, pal.Scrim)

	// Rule-of-thirds guides and the border.
	line := math.Max(1, math.Round(p.Scale))
	for i := 1; i <= 2; i++ {
		f := float64(i) / 3
		p.FillRect(fluent.Rect{X: bx.X + bx.W*f, Y: bx.Y, W: line, H: bx.H}, pal.CropGuide)
		p.FillRect(fluent.Rect{X: bx.X, Y: bx.Y + bx.H*f, W: bx.W, H: line}, pal.CropGuide)
	}
	p.FillRect(fluent.Rect{X: bx.X, Y: bx.Y, W: bx.W, H: line}, pal.CropLine)
	p.FillRect(fluent.Rect{X: bx.X, Y: bx.Bottom() - line, W: bx.W, H: line}, pal.CropLine)
	p.FillRect(fluent.Rect{X: bx.X, Y: bx.Y, W: line, H: bx.H}, pal.CropLine)
	p.FillRect(fluent.Rect{X: bx.Right() - line, Y: bx.Y, W: line, H: bx.H}, pal.CropLine)

	// Handles: corners always, edge midpoints only for a free box.
	pts := [][2]float64{{bx.X, bx.Y}, {bx.Right(), bx.Y}, {bx.X, bx.Bottom()}, {bx.Right(), bx.Bottom()}}
	if a.aspectRatio() == 0 {
		cx, cy := bx.X+bx.W/2, bx.Y+bx.H/2
		pts = append(pts, [2]float64{cx, bx.Y}, [2]float64{cx, bx.Bottom()}, [2]float64{bx.X, cy}, [2]float64{bx.Right(), cy})
	}
	s := p.S(12)
	for _, pt := range pts {
		p.BorderedRoundRect(fluent.Rect{X: pt[0] - s/2, Y: pt[1] - s/2, W: s, H: s}, p.S(3), pal.CropLine, pal.Scrim)
	}
}

// toSource converts a mouse position in the video area to source pixels.
func (a *app) toSource(x, y int) (float64, float64, float64) {
	p := a.stagePainter
	if p == nil {
		return 0, 0, 1
	}
	disp, k := a.cropDisplay(p)
	return (float64(x) - disp.X) / k, (float64(y) - disp.Y) / k, k
}

func (a *app) cropMouseDown(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton || a.stagePainter == nil {
		return
	}
	sx, sy, k := a.toSource(x, y)
	h := crop.HitTest(a.cr.box, sx, sy, a.stagePainter.S(10)/k, a.aspectRatio() > 0)
	if h == crop.None {
		return
	}
	a.cr.drag, a.cr.dragStart, a.cr.dragX, a.cr.dragY = h, a.cr.box, sx, sy
}

func (a *app) cropMouseMove(x, y int, button walk.MouseButton) {
	sx, sy, k := a.toSource(x, y)
	if a.cr.drag != crop.None && button&walk.LeftButton == 0 {
		a.cr.drag = crop.None // the button was released outside the window
	}
	if a.cr.drag != crop.None {
		fw, fh := a.frameSize()
		a.cr.box = crop.Drag(a.cr.dragStart, a.cr.drag, sx-a.cr.dragX, sy-a.cr.dragY, a.aspectRatio(), fw, fh, minCropSize)
		a.stage.Invalidate()
		a.refresh()
		return
	}
	h := crop.None
	if a.stagePainter != nil {
		h = crop.HitTest(a.cr.box, sx, sy, a.stagePainter.S(10)/k, a.aspectRatio() > 0)
	}
	a.stage.SetCursor(handleCursor(h))
}

func (a *app) cropMouseUp(_, _ int, button walk.MouseButton) {
	if button == walk.LeftButton {
		a.cr.drag = crop.None
	}
}

func handleCursor(h crop.Handle) walk.Cursor {
	switch h {
	case crop.NW, crop.SE:
		return walk.CursorSizeNWSE()
	case crop.NE, crop.SW:
		return walk.CursorSizeNESW()
	case crop.N, crop.S:
		return walk.CursorSizeNS()
	case crop.E, crop.W:
		return walk.CursorSizeWE()
	case crop.Move:
		return walk.CursorSizeAll()
	}
	return walk.CursorArrow()
}

// aspectMenu picks the aspect ratio of the crop box.
func (a *app) aspectMenu(anchor fluent.Rect) {
	items := []fluent.MenuItem{{Header: true, Text: "Aspect ratio"}}
	for i, as := range cropAspects {
		it := fluent.MenuItem{ID: menuAspectBase + i, Text: as.Name, Checked: a.cr.aspect == i}
		switch {
		case as.Ratio == 0:
			it.Hint = "Any shape"
		case as.Ratio < 0:
			it.Hint = fmt.Sprintf("%d×%d", a.info.Width, a.info.Height)
		}
		items = append(items, it)
	}
	id := a.popup(anchor, items)
	if id < menuAspectBase || id >= menuAspectBase+len(cropAspects) {
		a.refresh()
		return
	}
	a.cr.aspect = id - menuAspectBase
	if r := a.aspectRatio(); r > 0 {
		fw, fh := a.frameSize()
		b := a.cr.box
		a.cr.box = crop.Fit(r, b.X+b.W/2, b.Y+b.H/2, fw, fh)
	}
	a.stage.Invalidate()
	a.refresh()
}

// cropSummary describes the crop box for the footer.
func (a *app) cropSummary() string {
	r := a.cr.box.Even(a.info.Width, a.info.Height)
	return fmt.Sprintf("%d × %d  ·  Drag the box or its corners. Enter applies, Esc cancels.", r.Dx(), r.Dy())
}
