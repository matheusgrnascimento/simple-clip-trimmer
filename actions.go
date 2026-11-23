package main

import (
	"github.com/tailscale/walk"
)

// Actions shared by the keyboard shortcuts, the mpv key bindings and the
// control bar.
const (
	actPlayPause = "play-pause"
	actPrevFrame = "prev-frame"
	actNextFrame = "next-frame"
	actBack      = "back"
	actForward   = "forward"
	actSetStart  = "set-start"
	actSetEnd    = "set-end"
	actOpen      = "open"
	actExport    = "export"
)

// shortcuts maps keys to actions. mpvKey is the same key in mpv's input
// syntax, bound while the video window has the keyboard focus.
var shortcuts = []struct {
	mods   walk.Modifiers
	key    walk.Key
	mpvKey string
	action string
}{
	{0, walk.KeySpace, "SPACE", actPlayPause},
	{0, walk.KeyOEMComma, ",", actPrevFrame},
	{0, walk.KeyOEMPeriod, ".", actNextFrame},
	{0, walk.KeyLeft, "LEFT", actBack},
	{0, walk.KeyRight, "RIGHT", actForward},
	{0, walk.KeyI, "i", actSetStart},
	{0, walk.KeyO, "o", actSetEnd},
	{walk.ModControl, walk.KeyO, "Ctrl+o", actOpen},
	{walk.ModControl, walk.KeyE, "Ctrl+e", actExport},
}

// do runs an action by name.
func (a *app) do(action string) {
	switch action {
	case actPlayPause:
		a.togglePlay()
	case actPrevFrame:
		a.cmd("frame-back-step")
	case actNextFrame:
		a.cmd("frame-step")
	case actBack:
		a.cmd("seek", "-5", "relative+exact")
	case actForward:
		a.cmd("seek", "5", "relative+exact")
	case actSetStart:
		a.setIn()
	case actSetEnd:
		a.setOut()
	case actOpen:
		a.openDialog()
	case actExport:
		a.export()
	}
}
