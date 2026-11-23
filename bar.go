package main

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/tailscale/walk"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
)

// minSelection is the shortest selection the handles allow, in seconds.
const minSelection = 0.1

// hitArea identifies an interactive part of the control bar.
type hitArea int

const (
	hitNone hitArea = iota
	hitPlay
	hitTrack
	hitStart
	hitEnd
	hitOpen
	hitHeight
	hitQuality
	hitExport
	hitResult
)

// barLayout holds where each part was drawn in the last paint, in pixels,
// for hit testing.
type barLayout struct {
	play, track, open, height, quality, export, result fluent.Rect
	startX, endX                                       float64
}

// bar is the custom-drawn control bar: the timeline row (play, times,
// trim handles) and the footer (open, file information or export progress,
// output options and Export).
type bar struct {
	a       *app
	p       *fluent.Painter
	l       barLayout
	hover   hitArea
	pressed hitArea
	drag    hitArea // hitStart, hitEnd or hitTrack while dragging
}

func (b *bar) paint(c *walk.Canvas, _ walk.Rectangle) error {
	a := b.a
	bounds := a.barW.ClientBoundsPixels()
	dpi := a.barW.DPI()
	p := fluent.NewPainter(b.p, bounds.Width, bounds.Height, float64(dpi)/96)
	b.p = p
	p.Fill(a.pal.Window)

	b.l = barLayout{}
	b.paintTimeline(p, float64(bounds.Width))
	b.paintFooter(p, float64(bounds.Width), float64(bounds.Height))
	return blit(c, p, dpi)
}

func (b *bar) paintTimeline(p *fluent.Painter, w float64) {
	a, pal := b.a, b.a.pal
	loaded := a.info != nil
	cy := p.S(32)
	caption := a.fonts.Text(p.S(12))

	// Play / pause.
	b.l.play = fluent.Rect{X: p.S(16), Y: cy - p.S(18), W: p.S(36), H: p.S(36)}
	fill, fg := pal.Control, pal.TextDisabled
	if loaded {
		fill, fg = b.accentFill(hitPlay), pal.OnAccent
	}
	p.Circle(b.l.play.X+b.l.play.W/2, cy, p.S(18), fill)
	glyph := fluent.IconPlay
	if !a.paused {
		glyph = fluent.IconPause
	}
	p.TextCentered(a.fonts.Icons(p.S(14)), glyph, b.l.play.X+b.l.play.W/2, cy, fg)

	// Times on both sides of the track. Digits are tabular in Segoe UI, so
	// measuring zeros gives a stable width while playing.
	timeW := fluent.Measure(caption, zeroDigits(media.FormatTime(a.duration)))
	curX := b.l.play.Right() + p.S(12)
	trackX := curX + timeW + p.S(12)
	trackR := w - p.S(16) - timeW - p.S(12)
	timeColor := pal.TextSecondary
	cur, total := "--:--", "--:--"
	if loaded {
		cur, total = media.FormatTime(a.pos), media.FormatTime(a.duration)
	}
	p.Text(caption, cur, curX, cy, timeColor)
	p.Text(caption, total, w-p.S(16)-fluent.Measure(caption, total), cy, timeColor)

	b.l.track = fluent.Rect{X: trackX, Y: cy - p.S(16), W: math.Max(trackR-trackX, 1), H: p.S(32)}
	rail := fluent.Rect{X: trackX, Y: cy - p.S(2), W: b.l.track.W, H: p.S(4)}
	p.RoundRect(rail, p.S(2), pal.Rail)

	if loaded && a.duration > 0 {
		xs, xe := b.xAt(a.markIn), b.xAt(a.cutEnd())
		b.l.startX, b.l.endX = xs, xe
		p.RoundRect(fluent.Rect{X: xs, Y: cy - p.S(12), W: xe - xs, H: p.S(24)}, p.S(4), pal.AccentBand)
		p.RoundRect(fluent.Rect{X: xs, Y: rail.Y, W: xe - xs, H: rail.H}, p.S(2), pal.Accent)
		b.paintHandle(p, xs, cy, hitStart)
		b.paintHandle(p, xe, cy, hitEnd)

		px := b.xAt(a.pos)
		p.RoundRect(fluent.Rect{X: px - p.S(1), Y: cy - p.S(14), W: p.S(2), H: p.S(28)}, p.S(1), pal.Text)
		p.Circle(px, cy-p.S(14), p.S(4), pal.Text)
	}

	// Selection summary, or a hint on how to trim.
	var note string
	switch {
	case !loaded:
		note = "No video open"
	case a.markIn == 0 && a.markOut == 0:
		note = "Drag the blue handles, or press I and O, to trim"
	default:
		note = fmt.Sprintf("Selection  %s – %s  ·  %s",
			media.FormatTime(a.markIn), media.FormatTime(a.cutEnd()), formatLength(a.cutEnd()-a.markIn))
	}
	p.Text(caption, fluent.Ellipsize(caption, note, b.l.track.W), trackX, p.S(62), pal.TextSecondary)
}

func (b *bar) paintHandle(p *fluent.Painter, x, cy float64, h hitArea) {
	width, color := p.S(6), b.a.pal.Accent
	if b.hover == h || b.drag == h {
		width, color = p.S(8), b.a.pal.AccentHover
	}
	p.RoundRect(fluent.Rect{X: x - width/2, Y: cy - p.S(14), W: width, H: p.S(28)}, width/2, color)
}

func (b *bar) paintFooter(p *fluent.Painter, w, h float64) {
	a, pal := b.a, b.a.pal
	loaded := a.info != nil
	busy := a.exporting()
	text := a.fonts.Text(p.S(14))
	caption := a.fonts.Text(p.S(12))

	top := p.S(80)
	p.FillRect(fluent.Rect{X: 0, Y: top, W: w, H: h - top}, pal.Footer)
	p.FillRect(fluent.Rect{X: 0, Y: top, W: w, H: math.Max(1, math.Round(p.Scale))}, pal.Divider)
	cy := top + (h-top)/2
	btnH := p.S(32)

	// Open (icon button).
	b.l.open = fluent.Rect{X: p.S(10), Y: cy - btnH/2, W: p.S(40), H: btnH}
	if b.hover == hitOpen {
		p.RoundRect(b.l.open, p.S(4), pal.SubtleHover)
	}
	p.TextCentered(a.fonts.Icons(p.S(16)), fluent.IconOpenFile, b.l.open.X+b.l.open.W/2, cy, pal.Text)

	// Export / Cancel, from the right edge.
	label := "Export"
	if busy {
		label = "Cancel"
	}
	ew := math.Max(p.S(96), fluent.Measure(text, label)+p.S(32))
	b.l.export = fluent.Rect{X: w - p.S(16) - ew, Y: cy - btnH/2, W: ew, H: btnH}
	switch {
	case !loaded:
		p.BorderedRoundRect(b.l.export, p.S(4), pal.Control, pal.ControlBorder)
		p.TextCentered(text, label, b.l.export.X+ew/2, cy, pal.TextDisabled)
	case busy:
		b.paintControl(p, b.l.export, hitExport)
		p.TextCentered(text, label, b.l.export.X+ew/2, cy, pal.Text)
	default:
		p.RoundRect(b.l.export, p.S(4), b.accentFill(hitExport))
		p.TextCentered(text, label, b.l.export.X+ew/2, cy, pal.OnAccent)
	}

	// Output drop-downs.
	b.l.quality = b.paintDropDown(p, a.qualityLabel(), b.l.export.X-p.S(8), cy, hitQuality, !busy)
	b.l.height = b.paintDropDown(p, a.heightLabel(), b.l.quality.X-p.S(8), cy, hitHeight, !busy)

	// Middle: export progress, a message, the last result or the file info.
	x := b.l.open.Right() + p.S(8)
	maxW := b.l.height.X - p.S(16) - x
	switch {
	case busy && a.progress != nil:
		barW := math.Min(p.S(200), maxW*0.4)
		track := fluent.Rect{X: x, Y: cy - p.S(2), W: barW, H: p.S(4)}
		p.RoundRect(track, p.S(2), pal.ControlBorder)
		if f := a.progress.fraction; f > 0 {
			p.RoundRect(fluent.Rect{X: x, Y: track.Y, W: math.Max(barW*f, track.H), H: track.H}, p.S(2), pal.Accent)
		}
		tx := x + barW + p.S(12)
		p.Text(caption, fluent.Ellipsize(caption, a.progress.text(), maxW-barW-p.S(12)), tx, cy, pal.TextSecondary)
	case a.note != "":
		p.Text(caption, fluent.Ellipsize(caption, a.note, maxW), x, cy, pal.Text)
	case a.result != nil:
		link := "Show in folder"
		linkW := fluent.Measure(caption, link)
		msg := fluent.Ellipsize(caption, a.result.summary, maxW-linkW-p.S(16))
		p.Text(caption, msg, x, cy, pal.Text)
		lx := x + fluent.Measure(caption, msg) + p.S(16)
		b.l.result = fluent.Rect{X: lx, Y: cy - p.S(10), W: linkW, H: p.S(20)}
		linkColor := pal.Accent
		if b.hover == hitResult {
			linkColor = pal.AccentPressed
		}
		p.Text(caption, link, lx, cy, linkColor)
	case loaded:
		p.Text(caption, fluent.Ellipsize(caption, describe(*a.info), maxW), x, cy, pal.TextSecondary)
	default:
		p.Text(caption, "Open a video to start", x, cy, pal.TextSecondary)
	}
}

// paintDropDown draws a drop-down button ending at right and returns its rect.
func (b *bar) paintDropDown(p *fluent.Painter, label string, right, cy float64, h hitArea, enabled bool) fluent.Rect {
	pal := b.a.pal
	text := b.a.fonts.Text(p.S(14))
	chevron := b.a.fonts.Icons(p.S(10))
	w := p.S(12) + fluent.Measure(text, label) + p.S(12) + fluent.Measure(chevron, fluent.IconChevronDown) + p.S(12)
	r := fluent.Rect{X: right - w, Y: cy - p.S(16), W: w, H: p.S(32)}
	fg := pal.Text
	if enabled {
		b.paintControl(p, r, h)
	} else {
		p.BorderedRoundRect(r, p.S(4), pal.Control, pal.ControlBorder)
		fg = pal.TextDisabled
	}
	p.Text(text, label, r.X+p.S(12), cy, fg)
	p.Text(chevron, fluent.IconChevronDown, r.Right()-p.S(12)-fluent.Measure(chevron, fluent.IconChevronDown), cy, pal.TextSecondary)
	return r
}

// paintControl draws a standard (non-accent) button background.
func (b *bar) paintControl(p *fluent.Painter, r fluent.Rect, h hitArea) {
	pal := b.a.pal
	fill := pal.Control
	switch {
	case b.pressed == h && b.hover == h:
		fill = pal.ControlPressed
	case b.hover == h:
		fill = pal.ControlHover
	}
	p.BorderedRoundRect(r, p.S(4), fill, pal.ControlBorder)
}

func (b *bar) accentFill(h hitArea) color.NRGBA {
	pal := b.a.pal
	switch {
	case b.pressed == h && b.hover == h:
		return pal.AccentPressed
	case b.hover == h:
		return pal.AccentHover
	}
	return pal.Accent
}

// xAt maps a time to an x position on the track.
func (b *bar) xAt(t float64) float64 {
	if b.a.duration <= 0 {
		return b.l.track.X
	}
	f := math.Min(math.Max(t/b.a.duration, 0), 1)
	return b.l.track.X + f*b.l.track.W
}

// timeAt maps an x position on the track to a time.
func (b *bar) timeAt(x float64) float64 {
	f := (x - b.l.track.X) / b.l.track.W
	return math.Min(math.Max(f, 0), 1) * b.a.duration
}

func (b *bar) hitTest(x, y float64) hitArea {
	a, l := b.a, b.l
	loaded := a.info != nil
	if loaded && y >= l.track.Y && y < l.track.Bottom() && b.p != nil {
		grab := b.p.S(8)
		ds, de := math.Abs(x-l.startX), math.Abs(x-l.endX)
		if ds <= grab || de <= grab {
			if ds < de || (ds == de && x < l.startX) {
				return hitStart
			}
			return hitEnd
		}
	}
	switch {
	case loaded && l.play.Contains(x, y):
		return hitPlay
	case loaded && l.track.Contains(x, y):
		return hitTrack
	case l.open.Contains(x, y):
		return hitOpen
	case !a.exporting() && l.height.Contains(x, y):
		return hitHeight
	case !a.exporting() && l.quality.Contains(x, y):
		return hitQuality
	case loaded && l.export.Contains(x, y):
		return hitExport
	case a.result != nil && a.note == "" && !a.exporting() && l.result.Contains(x, y):
		return hitResult
	}
	return hitNone
}

func (b *bar) mouseDown(x, y int, button walk.MouseButton) {
	h := b.hitTest(float64(x), float64(y))
	if button == walk.RightButton {
		if h == hitTrack || h == hitStart || h == hitEnd {
			b.a.trackMenu(b.timeAt(float64(x)), fluent.Rect{X: float64(x), Y: b.l.track.Y, W: 1, H: b.l.track.H})
		}
		return
	}
	if button != walk.LeftButton {
		return
	}
	b.pressed = h
	switch h {
	case hitStart, hitEnd, hitTrack:
		b.drag = h
		b.dragTo(float64(x))
	}
	b.a.refresh()
}

func (b *bar) mouseMove(x, y int, button walk.MouseButton) {
	if b.drag != hitNone && button&walk.LeftButton == 0 {
		b.drag = hitNone // the button was released outside the window
	}
	if b.drag != hitNone {
		b.dragTo(float64(x))
		return
	}
	h := b.hitTest(float64(x), float64(y))
	if h != b.hover {
		b.hover = h
		b.a.refresh()
	}
	switch h {
	case hitStart, hitEnd:
		b.a.barW.SetCursor(walk.CursorSizeWE())
	case hitNone:
		b.a.barW.SetCursor(walk.CursorArrow())
	default:
		b.a.barW.SetCursor(walk.CursorHand())
	}
	fluent.TrackMouseLeave(uintptr(b.a.barW.Handle()))
}

func (b *bar) mouseUp(x, y int, button walk.MouseButton) {
	if button != walk.LeftButton {
		return
	}
	pressed := b.pressed
	b.pressed = hitNone
	if b.drag != hitNone {
		b.drag = hitNone
		b.a.refresh()
		return
	}
	if h := b.hitTest(float64(x), float64(y)); h == pressed {
		b.click(h)
	}
	b.a.refresh()
}

// message handles WM_MOUSELEAVE, which walk does not forward.
func (b *bar) message(msg uint32, _, _ uintptr) (bool, uintptr) {
	if msg == fluent.WMMouseLeave && b.drag == hitNone && b.hover != hitNone {
		b.hover = hitNone
		b.a.refresh()
	}
	return false, 0
}

// dragTo moves the dragged handle, or the playhead, to x.
func (b *bar) dragTo(x float64) {
	a := b.a
	t := b.timeAt(x)
	switch b.drag {
	case hitStart:
		a.markIn = math.Max(0, math.Min(t, a.cutEnd()-minSelection))
		t = a.markIn
	case hitEnd:
		t = math.Min(a.duration, math.Max(t, a.markIn+minSelection))
		a.markOut = t
		if t >= a.duration-0.001 {
			a.markOut = 0 // dragged to the end: keep "to the end"
		}
	}
	a.seekTo(t)
}

func (b *bar) click(h hitArea) {
	a := b.a
	switch h {
	case hitPlay:
		a.do(actPlayPause)
	case hitOpen:
		a.do(actOpen)
	case hitHeight:
		a.heightMenu(b.l.height)
	case hitQuality:
		a.qualityMenu(b.l.quality)
	case hitExport:
		if a.exporting() {
			a.cancel()
		} else {
			a.do(actExport)
		}
	case hitResult:
		a.showInFolder()
	}
}

// zeroDigits replaces every digit with 0, for stable text widths.
func zeroDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return '0'
		}
		return r
	}, s)
}

// formatLength formats a duration in seconds for the selection summary.
func formatLength(s float64) string {
	d := time.Duration(math.Round(s*10)) * 100 * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
	return d.Round(time.Second).String()
}
