package main

import (
	"image"
	"math"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	xdraw "golang.org/x/image/draw"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
)

// The title bar is drawn by the app at the Windows 11 "tall" height, like
// Photos or Media Player: the window keeps its frame (borders, shadow,
// rounded corners, resizing, Snap) but not the system caption.
const (
	titleHeightDIP   = 48
	captionButtonDIP = 46
)

// Non-client hit test codes (winuser.h).
const (
	htTransparent = ^uintptr(0) // HTTRANSPARENT (-1)
	htClient      = 1
	htCaption     = 2
	htMinButton   = 8
	htMaxButton   = 9
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htClose       = 20
)

// Window messages and system commands.
const (
	wmNCCalcSize     = 0x0083
	wmNCHitTest      = 0x0084
	wmNCMouseMove    = 0x00A0
	wmNCLButtonDown  = 0x00A1
	wmNCLButtonUp    = 0x00A2
	wmNCLButtonDbl   = 0x00A3
	scMinimize       = 0xF020
	scMaximize       = 0xF030
	scRestore        = 0xF120
	scClose          = 0xF060
	smCYFrame        = 33
	smCXPaddedBorder = 92
	waInactive       = 0
)

// nccalcsizeParams is NCCALCSIZE_PARAMS (winuser.h).
type nccalcsizeParams struct {
	Rgrc  [3]win.RECT
	Lppos uintptr
}

// titleBar draws the title bar and tracks its caption buttons.
type titleBar struct {
	a       *app
	p       *fluent.Painter
	hover   uintptr // caption button under the mouse (htMinButton, htMaxButton, htClose) or 0
	pressed uintptr
	active  bool
	icon    appIcon
}

func isCaptionButton(code uintptr) bool {
	return code == htMinButton || code == htMaxButton || code == htClose
}

// installTitleBar removes the system caption and routes hit testing so the
// drawn title bar behaves like the native one.
func (a *app) installTitleBar() error {
	main := uintptr(a.mw.Handle())
	// The title widget and its parent let the main window answer hit tests
	// over the title bar (caption, buttons, top edge).
	transparent := func(msg uint32, _, lParam uintptr) (bool, uintptr) {
		if msg == wmNCHitTest && a.title.hitScreen(lParam) != htClient {
			return true, htTransparent
		}
		return false, 0
	}
	if err := fluent.Subclass(uintptr(a.titleW.Handle()), transparent); err != nil {
		return err
	}
	if parent := win.GetParent(a.titleW.Handle()); parent != 0 && uintptr(parent) != main {
		if err := fluent.Subclass(uintptr(parent), transparent); err != nil {
			return err
		}
	}
	a.mw.TitleChanged().Attach(func() { a.titleW.Invalidate() })
	a.title.active = true

	// Recalculate the frame now that WM_NCCALCSIZE drops the caption.
	win.SetWindowPos(a.mw.Handle(), 0, 0, 0, 0, 0,
		win.SWP_FRAMECHANGED|win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
	return nil
}

// frameMessage handles the main window's frame messages for the drawn
// title bar. It returns false for messages it leaves to the window.
func (a *app) frameMessage(msg uint32, wParam, lParam uintptr) (bool, uintptr) {
	main := uintptr(a.mw.Handle())
	t := a.title
	switch msg {
	case wmNCCalcSize:
		if wParam == 0 {
			return false, 0
		}
		// Let Windows compute the side and bottom borders, then give the
		// caption's space back to the client area.
		params := *(**nccalcsizeParams)(unsafe.Pointer(&lParam))
		top := params.Rgrc[0].Top
		r := fluent.DefSubclass(main, msg, wParam, lParam)
		params.Rgrc[0].Top = top
		if win.IsZoomed(a.mw.Handle()) {
			// A maximized window hangs its border off the screen.
			params.Rgrc[0].Top += a.frameThickness()
		}
		return true, r

	case wmNCHitTest:
		if r := fluent.DefSubclass(main, msg, wParam, lParam); r != htClient {
			return true, r // side and bottom borders
		}
		return true, t.hitScreen(lParam)

	case wmNCMouseMove:
		code := wParam
		if !isCaptionButton(code) {
			code = 0
		}
		if code != t.hover {
			t.hover = code
			a.titleW.Invalidate()
		}
		if code != 0 {
			fluent.TrackNCMouseLeave(main)
			return true, 0
		}

	case fluent.WMNCMouseLeave:
		if t.hover != 0 || t.pressed != 0 {
			t.hover, t.pressed = 0, 0
			a.titleW.Invalidate()
		}

	case wmNCLButtonDown, wmNCLButtonDbl:
		// Handle the buttons here: the default handling would draw classic
		// caption buttons over the window.
		if isCaptionButton(wParam) {
			t.pressed = wParam
			a.titleW.Invalidate()
			return true, 0
		}

	case wmNCLButtonUp:
		if isCaptionButton(wParam) {
			if t.pressed == wParam {
				a.captionCommand(wParam)
			}
			t.pressed = 0
			a.titleW.Invalidate()
			return true, 0
		}

	case win.WM_ACTIVATE:
		t.active = win.LOWORD(uint32(wParam)) != waInactive
		a.titleW.Invalidate()

	case win.WM_SIZE:
		a.titleW.Invalidate() // maximize and restore change the glyph
	}
	return false, 0
}

// captionCommand runs a caption button through the system commands, so
// Windows animates it as usual.
func (a *app) captionCommand(code uintptr) {
	cmd := uintptr(scClose)
	switch code {
	case htMinButton:
		cmd = scMinimize
	case htMaxButton:
		cmd = scMaximize
		if win.IsZoomed(a.mw.Handle()) {
			cmd = scRestore
		}
	}
	win.PostMessage(a.mw.Handle(), win.WM_SYSCOMMAND, cmd, 0)
}

// frameThickness is the resize border height in pixels at the window's DPI.
func (a *app) frameThickness() int32 {
	dpi := uint32(a.mw.DPI())
	return win.GetSystemMetricsForDpi(smCYFrame, dpi) + win.GetSystemMetricsForDpi(smCXPaddedBorder, dpi)
}

// hitScreen hit-tests a screen point (as packed in WM_NCHITTEST's lParam)
// against the title bar.
func (t *titleBar) hitScreen(lParam uintptr) uintptr {
	a := t.a
	pt := win.POINT{X: int32(int16(win.LOWORD(uint32(lParam)))), Y: int32(int16(win.HIWORD(uint32(lParam))))}
	win.ScreenToClient(a.mw.Handle(), &pt)
	var rc win.RECT
	win.GetClientRect(a.mw.Handle(), &rc)
	s := float64(a.mw.DPI()) / 96
	h := int32(math.Round(titleHeightDIP * s))
	if pt.Y < 0 || pt.Y >= h {
		return htClient
	}
	if !win.IsZoomed(a.mw.Handle()) && pt.Y < a.frameThickness() {
		switch {
		case pt.X < a.frameThickness():
			return htTopLeft
		case pt.X >= rc.Right-a.frameThickness():
			return htTopRight
		}
		return htTop
	}
	bw := int32(math.Round(captionButtonDIP * s))
	switch {
	case pt.X >= rc.Right-bw:
		return htClose
	case pt.X >= rc.Right-2*bw:
		return htMaxButton
	case pt.X >= rc.Right-3*bw:
		return htMinButton
	}
	return htCaption
}

func (t *titleBar) paint(c *walk.Canvas, _ walk.Rectangle) error {
	a := t.a
	b := a.titleW.ClientBoundsPixels()
	p := fluent.NewPainter(t.p, b.Width, b.Height, float64(a.titleW.DPI())/96)
	t.p = p
	pal := a.pal
	p.Fill(pal.Window)
	w, cy := float64(b.Width), float64(b.Height)/2

	fg := pal.Text
	if !t.active {
		fg = pal.TextDisabled
	}

	// App icon and title.
	iconSize := int(p.S(16))
	if icon := t.icon.at(iconSize); icon != nil {
		pt := image.Pt(int(p.S(16)), int(math.Round(cy))-iconSize/2)
		xdraw.Draw(p.Img, image.Rectangle{Min: pt, Max: pt.Add(icon.Rect.Size())}, icon, image.Point{}, xdraw.Over)
	}
	bw := p.S(captionButtonDIP)
	caption := a.fonts.Text(p.S(12))
	p.Text(caption, fluent.Ellipsize(caption, a.mw.Title(), w-3*bw-p.S(64)), p.S(48), cy, fg)

	// Caption buttons, from the right.
	glyphs := []struct {
		code  uintptr
		glyph string
	}{
		{htClose, fluent.IconChromeClose},
		{htMaxButton, fluent.IconChromeMaximize},
		{htMinButton, fluent.IconChromeMinimize},
	}
	if win.IsZoomed(a.mw.Handle()) {
		glyphs[1].glyph = fluent.IconChromeRestore
	}
	icons := a.fonts.Icons(p.S(10))
	for i, g := range glyphs {
		r := fluent.Rect{X: w - float64(i+1)*bw, Y: 0, W: bw, H: float64(b.Height)}
		gc := fg
		switch {
		case g.code == htClose && t.hover == htClose && t.pressed == htClose:
			p.FillRect(r, pal.ClosePressed)
			gc = pal.OnClose
		case g.code == htClose && t.hover == htClose:
			p.FillRect(r, pal.CloseHover)
			gc = pal.OnClose
		case t.hover == g.code && t.pressed == g.code:
			p.FillRect(r, pal.SubtlePressed)
		case t.hover == g.code:
			p.FillRect(r, pal.SubtleHover)
		}
		p.TextCentered(icons, g.glyph, r.X+bw/2, cy, gc)
	}
	return blit(c, p)
}
