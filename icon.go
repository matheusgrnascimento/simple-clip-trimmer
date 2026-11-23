package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"log/slog"

	"github.com/tailscale/walk"
	xdraw "golang.org/x/image/draw"
)

// appIconPNG is the 256 px app icon, drawn in the title bar. Generate it
// with "go run ./tools/genicon".
//
//go:embed assets/icon.png
var appIconPNG []byte

// iconResourceID is the icon group tools/winres embeds in the .exe.
const iconResourceID = 1

// appIcon caches the app icon scaled to the title bar's icon size.
type appIcon struct {
	src    image.Image
	scaled *image.RGBA
}

// at returns the icon scaled to size×size pixels, or nil if it can't be read.
func (ic *appIcon) at(size int) *image.RGBA {
	if ic.src == nil {
		img, err := png.Decode(bytes.NewReader(appIconPNG))
		if err != nil {
			slog.Warn("app icon", "err", err)
			return nil
		}
		ic.src = img
	}
	if ic.scaled == nil || ic.scaled.Rect.Dx() != size {
		ic.scaled = image.NewRGBA(image.Rect(0, 0, size, size))
		xdraw.CatmullRom.Scale(ic.scaled, ic.scaled.Rect, ic.src, ic.src.Bounds(), xdraw.Src, nil)
	}
	return ic.scaled
}

// setWindowIcon gives the window the icon embedded in the .exe, for the
// taskbar and Alt+Tab. Without the resource (a plain go build) Windows keeps
// its default icon.
func (a *app) setWindowIcon() {
	icon, err := walk.NewIconFromResourceId(iconResourceID)
	if err != nil {
		slog.Info("no embedded app icon; build with build.ps1 to embed it", "err", err)
		return
	}
	if err := a.mw.SetIcon(icon); err != nil {
		slog.Warn("window icon", "err", err)
	}
}
