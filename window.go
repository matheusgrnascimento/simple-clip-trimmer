package main

import (
	"image/color"
	"log/slog"
	"os"
	"time"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"
	"github.com/tailscale/win"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
)

// barHeightDIP is the height of the control bar: the timeline row and the
// footer with the file information, output options and Export.
const barHeightDIP = 124

// buildWindow creates the main window: the drawn title bar, the video area
// and the custom-drawn control bar. There is no menu bar; actions live in
// the bar and in keyboard shortcuts.
func (a *app) buildWindow() error {
	err := MainWindow{
		AssignTo:   &a.mw,
		Title:      appName,
		MinSize:    Size{Width: 720, Height: 480},
		Size:       Size{Width: 1200, Height: 800},
		Layout:     VBox{MarginsZero: true, SpacingZero: true},
		Background: SolidColorBrush{Color: walkColor(a.pal.Window)},
		OnDropFiles: func(files []string) {
			if len(files) > 0 {
				a.load(files[0])
			}
		},
		Children: []Widget{
			CustomWidget{
				AssignTo:            &a.titleW,
				MinSize:             Size{Height: titleHeightDIP},
				MaxSize:             Size{Height: titleHeightDIP},
				PaintPixels:         a.title.paint,
				PaintMode:           PaintNoErase,
				InvalidatesOnResize: true,
			},
			// No minimum height: walk's box layout sizes items with the
			// largest minimum first, and the video area must come last so it
			// takes all the height the fixed title and control bars leave.
			CustomWidget{
				AssignTo:            &a.stage,
				StretchFactor:       100,
				MinSize:             Size{Width: 320},
				PaintPixels:         a.paintStage,
				PaintMode:           PaintNoErase,
				InvalidatesOnResize: true,
				OnMouseDown:         a.onStageMouseDown,
				OnMouseMove:         a.onStageMouseMove,
				OnMouseUp:           a.onStageMouseUp,
			},
			CustomWidget{
				AssignTo:            &a.barW,
				MinSize:             Size{Height: barHeightDIP},
				MaxSize:             Size{Height: barHeightDIP},
				PaintPixels:         a.bar.paint,
				PaintMode:           PaintNoErase,
				InvalidatesOnResize: true,
				OnMouseDown:         a.bar.mouseDown,
				OnMouseMove:         a.bar.mouseMove,
				OnMouseUp:           a.bar.mouseUp,
			},
		},
	}.Create()
	if err != nil {
		return err
	}

	// walk always creates a menu bar and a toolbar, which Windows draws as
	// light strips even when they are empty. The app uses neither.
	win.SetMenu(a.mw.Handle(), 0)
	if tb := a.mw.ToolBar(); tb != nil {
		tb.SetVisible(false)
	}
	fluent.ApplyFrame(uintptr(a.mw.Handle()), a.pal)
	if err := fluent.Subclass(uintptr(a.mw.Handle()), a.windowMessage); err != nil {
		slog.Warn("subclass main window", "err", err)
	}
	if err := fluent.Subclass(uintptr(a.barW.Handle()), a.bar.message); err != nil {
		slog.Warn("subclass control bar", "err", err)
	}
	if err := a.installTitleBar(); err != nil {
		slog.Warn("title bar", "err", err)
	}
	a.setWindowIcon()
	a.addShortcuts()
	return nil
}

// windowMessage handles the drawn title bar's frame messages and follows
// theme and accent color changes of Windows.
func (a *app) windowMessage(msg uint32, wParam, lParam uintptr) (bool, uintptr) {
	if handled, r := a.frameMessage(msg, wParam, lParam); handled {
		return true, r
	}
	if msg == fluent.WMSettingChange && fluent.LParamString(lParam) == "ImmersiveColorSet" {
		a.applyTheme()
	}
	return false, 0
}

// themeEnv forces the light or dark theme ("light" or "dark"), to check
// both without changing the Windows setting.
const themeEnv = "SIMPLE_CLIP_TRIMMER_THEME"

// loadPalette returns the palette of the Windows theme, or of the theme
// forced by themeEnv.
func loadPalette() fluent.Palette {
	dark := fluent.SystemDark()
	switch os.Getenv(themeEnv) {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	return fluent.NewPalette(dark, fluent.SystemAccentPalette())
}

func (a *app) applyTheme() {
	a.pal = loadPalette()
	fluent.ApplyFrame(uintptr(a.mw.Handle()), a.pal)
	if brush, err := walk.NewSolidColorBrush(walkColor(a.pal.Window)); err == nil {
		a.mw.SetBackground(brush)
	}
	a.titleW.Invalidate()
	a.stage.Invalidate()
	a.refresh()
}

// addShortcuts registers the keyboard shortcuts of the window. The same
// keys are bound in mpv (see bindKeys) for when the video has the focus.
func (a *app) addShortcuts() {
	for _, s := range shortcuts {
		act := walk.NewAction()
		if err := act.SetShortcut(walk.Shortcut{Modifiers: s.mods, Key: s.key}); err != nil {
			slog.Warn("shortcut", "action", s.action, "err", err)
			continue
		}
		name := s.action
		act.Triggered().Attach(func() { a.do(name) })
		if err := a.mw.ShortcutActions().Add(act); err != nil {
			slog.Warn("shortcut", "action", s.action, "err", err)
		}
	}
}

// refresh repaints the control bar after a state change.
func (a *app) refresh() {
	if a.barW != nil {
		a.barW.Invalidate()
	}
}

// flash shows a short message in the footer for a few seconds.
func (a *app) flash(msg string) {
	a.note = msg
	a.noteSeq++
	seq := a.noteSeq
	a.refresh()
	time.AfterFunc(6*time.Second, func() {
		a.mw.Synchronize(func() {
			if a.noteSeq == seq {
				a.note = ""
				a.refresh()
			}
		})
	})
}

func (a *app) exporting() bool { return a.cancelExport != nil }

// cutEnd is the end of the selection, in seconds.
func (a *app) cutEnd() float64 {
	if a.markOut > 0 {
		return a.markOut
	}
	return a.duration
}

// blit draws a painter's image onto a canvas at the origin. It skips walk's
// bitmap conversion, which reads the image pixel by pixel and allocates for
// each one, and was most of the cost of a repaint.
func blit(c *walk.Canvas, p *fluent.Painter) error {
	return p.Blit(uintptr(c.HDC()))
}

func walkColor(c color.NRGBA) walk.Color { return walk.RGB(c.R, c.G, c.B) }
