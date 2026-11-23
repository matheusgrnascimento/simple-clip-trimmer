package main

import (
	"github.com/tailscale/walk"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
)

// paintStage paints the video area. While a video plays, mpv's window
// covers it. Before a video is open it shows the empty state, which opens a
// file when clicked; in crop mode it shows the still frame and crop box.
func (a *app) paintStage(c *walk.Canvas, _ walk.Rectangle) error {
	b := a.stage.ClientBoundsPixels()
	dpi := a.stage.DPI()
	p := fluent.NewPainter(a.stagePainter, b.Width, b.Height, float64(dpi)/96)
	a.stagePainter = p

	switch {
	case a.cr.active:
		a.paintCrop(p)
	case a.info != nil:
		p.Fill(a.pal.Video)
	default:
		p.Fill(a.pal.Stage)
		cx, cy := float64(b.Width)/2, float64(b.Height)/2
		p.TextCentered(a.fonts.Icons(p.S(40)), fluent.IconVideo, cx, cy-p.S(44), a.pal.TextSecondary)
		p.TextCentered(a.fonts.Text(p.S(20)), "Open a video", cx, cy+p.S(4), a.pal.Text)
		p.TextCentered(a.fonts.Text(p.S(14)), "Drop a file here, or click to browse", cx, cy+p.S(34), a.pal.TextSecondary)
	}
	return blit(c, p, dpi)
}

func (a *app) onStageMouseDown(x, y int, button walk.MouseButton) {
	if a.cr.active {
		a.cropMouseDown(x, y, button)
	}
}

func (a *app) onStageMouseMove(x, y int, button walk.MouseButton) {
	switch {
	case a.cr.active:
		a.cropMouseMove(x, y, button)
	case a.info == nil:
		a.stage.SetCursor(walk.CursorHand())
	default:
		a.stage.SetCursor(walk.CursorArrow())
	}
}

func (a *app) onStageMouseUp(x, y int, button walk.MouseButton) {
	switch {
	case a.cr.active:
		a.cropMouseUp(x, y, button)
	case button == walk.LeftButton && a.info == nil:
		a.openDialog()
	}
}
