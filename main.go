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

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/mpv"
)

const appName = "Simple Clip Trimmer"

// app holds the window, its widgets and the editing state. Every field is
// only touched on the UI thread, except player (thread safe) and closing.
type app struct {
	mw    *walk.MainWindow
	video *walk.CustomWidget

	playBtn, prevBtn, nextBtn       *walk.PushButton
	markInBtn, markOutBtn, clearBtn *walk.PushButton
	exportBtn, cancelBtn            *walk.PushButton
	loopCB                          *walk.CheckBox
	seek                            *walk.Slider
	timeLbl, infoLbl, inLbl, outLbl *walk.Label
	cutLbl, statusLbl               *walk.Label
	resCB, encCB, qualCB            *walk.ComboBox
	progress                        *walk.ProgressBar

	player  *mpv.Player
	evDone  chan struct{}
	closing atomic.Bool

	info           *media.Info
	duration, pos  float64
	paused         bool
	markIn         float64
	markOut        float64 // 0 = to the end
	updatingSlider bool
	lastUserSeek   time.Time

	cancelExport context.CancelFunc
	encMissing   map[media.Encoder]bool // encoders that do not work on this machine
}

func main() {
	wapp, err := walk.InitApp()
	if err != nil {
		slog.Error("init walk", "err", err)
		os.Exit(1)
	}
	a := &app{paused: true}
	if err := a.buildWindow(); err != nil {
		walk.MsgBox(nil, appName, "Could not create the window:\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	if err := a.initPlayer(); err != nil {
		walk.MsgBox(a.mw, appName, "Could not start the player (libmpv-2.dll):\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	a.checkTools()
	a.updateControls()

	if len(os.Args) > 1 {
		a.load(os.Args[1])
	}
	wapp.Run()
}
