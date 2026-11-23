package main

import (
	"github.com/tailscale/walk"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
)

// paintStage paints the video area. Once a video is open, mpv's window
// covers it; before that it shows the empty state, which opens a file when
// clicked.
func (a *app) paintStage(c *walk.Canvas, _ walk.Rectangle) error {
	b := a.stage.ClientBoundsPixels()
	dpi := a.stage.DPI()
	p := fluent.NewPainter(a.stagePainter, b.Width, b.Height, float64(dpi)/96)
	a.stagePainter = p

	if a.info != nil {
		p.Fill(a.pal.Video)
		return blit(c, p, dpi)
	}

	p.Fill(a.pal.Stage)
	cx, cy := float64(b.Width)/2, float64(b.Height)/2
	p.TextCentered(a.fonts.Icons(p.S(40)), fluent.IconVideo, cx, cy-p.S(44), a.pal.TextSecondary)
	p.TextCentered(a.fonts.Text(p.S(20)), "Open a video", cx, cy+p.S(4), a.pal.Text)
	p.TextCentered(a.fonts.Text(p.S(14)), "Drop a file here, or click to browse", cx, cy+p.S(34), a.pal.TextSecondary)
	return blit(c, p, dpi)
}

func (a *app) onStageMouseMove(_, _ int, _ walk.MouseButton) {
	if a.info == nil {
		a.stage.SetCursor(walk.CursorHand())
	} else {
		a.stage.SetCursor(walk.CursorArrow())
	}
}

func (a *app) onStageMouseUp(_, _ int, button walk.MouseButton) {
	if button == walk.LeftButton && a.info == nil {
		a.openDialog()
	}
}
