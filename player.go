package main

import (
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/mpv"
)

// Property IDs for mpv.Observe.
const (
	obsTimePos = iota + 1
	obsDuration
	obsPause
)

// messagePrefix tags the script messages our mpv key bindings send.
const messagePrefix = "sct"

// initPlayer embeds mpv in the video area and starts its event loop.
func (a *app) initPlayer() error {
	p, err := mpv.New()
	if err != nil {
		return err
	}
	opts := [][2]string{
		{"wid", strconv.FormatUint(uint64(a.stage.Handle()), 10)},
		{"config", "no"},       // ignore the user's mpv.conf
		{"terminal", "no"},     // no console output
		{"idle", "yes"},        // stay alive without a file
		{"force-window", "no"}, // no video window (and no black box) until a file opens
		{"keep-open", "yes"},   // keep the last frame at the end of the file
		{"pause", "yes"},       // open paused on the first frame
		{"hwdec", "auto-safe"}, // d3d11va/nvdec
		{"hr-seek", "yes"},     // frame-accurate seeking
		{"osc", "no"},          // the app has its own controls
		{"osd-level", "0"},
		{"input-default-bindings", "no"}, // only the bindings set in bindKeys
		{"input-vo-keyboard", "yes"},
		{"input-cursor", "yes"},
		{"cursor-autohide", "no"},
		{"drag-and-drop", "no"}, // drops are handled by the main window
	}
	for _, o := range opts {
		if err := p.SetOption(o[0], o[1]); err != nil {
			p.Destroy()
			return err
		}
	}
	if err := p.Initialize(); err != nil {
		p.Destroy()
		return err
	}
	for _, o := range []struct {
		id     uint64
		name   string
		format mpv.Format
	}{
		{obsTimePos, "time-pos", mpv.FormatDouble},
		{obsDuration, "duration", mpv.FormatDouble},
		{obsPause, "pause", mpv.FormatFlag},
	} {
		if err := p.Observe(o.id, o.name, o.format); err != nil {
			p.Destroy()
			return err
		}
	}

	a.player = p
	a.bindKeys()
	a.evDone = make(chan struct{})
	go a.eventLoop()

	a.mw.Closing().Attach(a.onClosing)
	return nil
}

// bindKeys binds the shortcuts, and a click on the video, inside mpv: once
// the video window has the keyboard focus, its keys go to mpv's thread and
// never reach the walk message loop. Each binding sends a script message
// that eventLoop turns into an app action.
func (a *app) bindKeys() {
	bind := func(key, action string) {
		if err := a.player.Command("keybind", key, "script-message "+messagePrefix+" "+action); err != nil {
			slog.Warn("mpv keybind", "key", key, "err", err)
		}
	}
	for _, s := range shortcuts {
		bind(s.mpvKey, s.action)
	}
	bind("MBTN_LEFT", actPlayPause)
}

func (a *app) onClosing(canceled *bool, _ walk.CloseReason) {
	if a.exporting() {
		r := walk.MsgBox(a.mw, appName, "An export is running. Cancel it and exit?", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion)
		if r != win.IDYES {
			*canceled = true
			return
		}
		a.cancelExport()
	}
	a.closing.Store(true)
	a.player.Wakeup()
	<-a.evDone
	a.player.Destroy()
}

// eventLoop runs on its own goroutine and forwards events to the UI thread.
func (a *app) eventLoop() {
	defer close(a.evDone)
	var lastPos time.Time
	paused := true
	for !a.closing.Load() {
		ev := a.player.WaitEvent(-1)
		switch ev.ID {
		case mpv.EventShutdown:
			return
		case mpv.EventPlaybackRestart:
			// A seek finished: crop mode shows the new frame.
			a.mw.Synchronize(func() {
				if a.cr.active {
					a.grabFrame()
				}
			})
		case mpv.EventClientMessage:
			if len(ev.Args) == 2 && ev.Args[0] == messagePrefix {
				action := ev.Args[1]
				a.mw.Synchronize(func() { a.do(action) })
			}
		case mpv.EventPropertyChange:
			switch ev.Name {
			case "time-pos":
				if ev.Format != mpv.FormatDouble {
					continue
				}
				// While playing, about 25 updates per second are enough for the UI.
				if !paused && time.Since(lastPos) < 40*time.Millisecond {
					continue
				}
				lastPos = time.Now()
				v := ev.Double
				a.mw.Synchronize(func() { a.onPos(v) })
			case "duration":
				if ev.Format == mpv.FormatDouble && ev.Double > 0 {
					v := ev.Double
					a.mw.Synchronize(func() { a.onDuration(v) })
				}
			case "pause":
				paused = ev.Flag
				v := ev.Flag
				a.mw.Synchronize(func() { a.onPause(v) })
			}
		}
	}
}

func (a *app) cmd(args ...string) {
	if a.info == nil {
		return
	}
	if err := a.player.Command(args...); err != nil {
		a.flash("Playback: " + err.Error())
	}
}

func (a *app) togglePlay() {
	if a.info == nil || a.cr.active {
		return
	}
	// Stopped at the end of the selection: play it again from its start.
	if a.paused && math.Abs(a.pos-a.cutEnd()) < 0.05 {
		a.cmd("seek", formatSeconds(a.markIn), "absolute+exact")
	}
	a.cmd("cycle", "pause")
}

func (a *app) onPos(t float64) {
	// While the user drags on the timeline, the playhead follows the mouse.
	if time.Since(a.lastUserSeek) < 300*time.Millisecond {
		return
	}
	// Playing from inside the selection stops at its end, so the preview
	// shows exactly what will be exported.
	if !a.paused && a.markOut > 0 && a.pos < a.markOut && t >= a.markOut {
		if err := a.player.SetProperty("pause", "yes"); err != nil {
			slog.Warn("pause at selection end", "err", err)
		}
		a.cmd("seek", formatSeconds(a.markOut), "absolute+exact")
		t = a.markOut
	}
	a.pos = t
	a.refresh()
}

func (a *app) onDuration(d float64) {
	a.duration = d
	a.refresh()
}

func (a *app) onPause(paused bool) {
	a.paused = paused
	// Updates are throttled while playing, so read the exact paused position.
	a.pos = a.posNow()
	a.refresh()
}

// seekTo moves the preview to t without waiting for mpv, for dragging.
func (a *app) seekTo(t float64) {
	if a.info == nil {
		return
	}
	a.pos = t
	a.lastUserSeek = time.Now()
	if err := a.player.CommandAsync("seek", formatSeconds(t), "absolute+exact"); err != nil {
		slog.Warn("seek", "err", err)
	}
	a.refresh()
}

// posNow reads the position from mpv, which is more precise than the last
// (throttled) update.
func (a *app) posNow() float64 {
	if t, err := a.player.GetDouble("time-pos"); err == nil {
		return t
	}
	return a.pos
}

func (a *app) setIn() {
	if a.info == nil {
		return
	}
	a.setInAt(a.posNow())
}

func (a *app) setInAt(t float64) {
	a.markIn = t
	if a.markOut > 0 && a.markOut-a.markIn < minSelection {
		a.markOut = 0
	}
	if a.cutEnd()-a.markIn < minSelection {
		a.markIn = 0
		a.flash("The start must come before the end.")
	}
	a.refresh()
}

func (a *app) setOut() {
	if a.info == nil {
		return
	}
	a.setOutAt(a.posNow())
}

func (a *app) setOutAt(t float64) {
	if t-a.markIn < minSelection {
		a.flash("The end must come after the start (" + media.FormatTime(a.markIn) + ").")
		return
	}
	a.markOut = t
	if t >= a.duration-0.001 {
		a.markOut = 0
	}
	a.refresh()
}

func (a *app) clearMarks() {
	a.markIn, a.markOut = 0, 0
	a.refresh()
}

func formatSeconds(s float64) string { return strconv.FormatFloat(s, 'f', 3, 64) }
