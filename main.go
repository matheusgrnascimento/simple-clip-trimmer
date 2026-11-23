// Simple Clip Trimmer trims, resizes and compresses videos with ffmpeg,
// with a native, hardware-accelerated preview through libmpv.
package main

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/tailscale/walk"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/mpv"
)

const appName = "Simple Clip Trimmer"

// app holds the window and the editing state. Every field is only touched
// on the UI thread, except player (thread safe) and closing.
type app struct {
	mw    *walk.MainWindow
	stage *walk.CustomWidget // video area; mpv draws in a child window of it
	barW  *walk.CustomWidget // custom-drawn control bar
	bar   *bar

	pal          fluent.Palette
	fonts        *fluent.Fonts
	stagePainter *fluent.Painter

	player  *mpv.Player
	evDone  chan struct{}
	closing atomic.Bool

	info          *media.Info
	duration, pos float64
	paused        bool
	markIn        float64
	markOut       float64 // 0 = to the end
	lastUserSeek  time.Time

	height   int // output height, 0 = original
	quality  media.Quality
	codec    media.Codec
	noAudio  bool                          // drop every audio track
	encoders map[media.Codec]media.Encoder // fastest working encoder per codec; nil until checked

	cancelExport context.CancelFunc
	progress     *exportProgress // running export
	result       *exportResult   // last finished export
	note         string          // short message shown in the footer
	noteSeq      int
}

func main() {
	// Must run before any window or menu exists.
	fluent.AllowDarkMenus()

	wapp, err := walk.InitApp()
	if err != nil {
		slog.Error("init walk", "err", err)
		os.Exit(1)
	}
	fonts, err := fluent.LoadFonts()
	if err != nil {
		walk.MsgBox(nil, appName, "Could not load the Windows fonts:\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	a := &app{paused: true, fonts: fonts, pal: loadPalette(), height: 1080, quality: media.QualityMedium}
	a.bar = &bar{a: a}
	if err := a.buildWindow(); err != nil {
		walk.MsgBox(nil, appName, "Could not create the window:\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	if err := a.initPlayer(); err != nil {
		walk.MsgBox(a.mw, appName, "Could not start the player (libmpv-2.dll):\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	a.checkTools()

	if len(os.Args) > 1 {
		a.load(os.Args[1])
	}
	wapp.Run()
}
