package main

import (
	"strconv"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/mpv"
)

const sliderScale = 1000 // the position slider works in milliseconds

// Property IDs for mpv.Observe.
const (
	obsTimePos = iota + 1
	obsDuration
	obsPause
)

// initPlayer embeds mpv in the video area and starts its event loop.
func (a *app) initPlayer() error {
	p, err := mpv.New()
	if err != nil {
		return err
	}
	opts := [][2]string{
		{"wid", strconv.FormatUint(uint64(a.video.Handle()), 10)},
		{"config", "no"},        // ignore the user's mpv.conf
		{"terminal", "no"},      // no console output
		{"idle", "yes"},         // stay alive without a file
		{"force-window", "yes"}, // black window even without a video
		{"keep-open", "yes"},    // keep the last frame at the end of the file
		{"pause", "yes"},        // open paused on the first frame
		{"hwdec", "auto-safe"},  // d3d11va/nvdec
		{"hr-seek", "yes"},      // frame-accurate seeking
		{"osc", "no"},           // the app has its own controls
		{"osd-level", "0"},
		{"input-default-bindings", "no"},
		{"input-vo-keyboard", "no"},
		{"input-cursor", "no"},
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
	a.evDone = make(chan struct{})
	go a.eventLoop()

	a.mw.Closing().Attach(a.onClosing)
	return nil
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
		a.setStatus("mpv: %v", err)
	}
}

func (a *app) togglePlay() {
	if a.info == nil {
		return
	}
	// At the end of the selection, play starts over from its beginning.
	if a.paused && a.pos >= a.cutEnd()-0.05 {
		a.cmd("seek", formatSeconds(a.markIn), "absolute+exact")
	}
	a.cmd("cycle", "pause")
}

func (a *app) onPos(t float64) {
	a.pos = t
	a.timeLbl.SetText(media.FormatTime(t) + " / " + media.FormatTime(a.duration))
	// Do not fight the user while they drag the slider.
	if time.Since(a.lastUserSeek) < 300*time.Millisecond {
		return
	}
	a.setSlider(t)
}

func (a *app) onDuration(d float64) {
	a.duration = d
	a.seek.SetRange(0, int(d*sliderScale))
	a.updateControls()
}

func (a *app) onPause(paused bool) {
	a.paused = paused
	// Updates are throttled while playing, so read the exact paused position.
	a.onPos(a.posNow())
	a.updateControls()
}

// setSlider moves the slider without triggering a seek.
func (a *app) setSlider(t float64) {
	a.updatingSlider = true
	a.seek.SetValue(int(t * sliderScale))
	a.updatingSlider = false
}

func (a *app) onSliderChanged() {
	if a.updatingSlider || a.info == nil {
		return
	}
	a.lastUserSeek = time.Now()
	t := float64(a.seek.Value()) / sliderScale
	if err := a.player.CommandAsync("seek", formatSeconds(t), "absolute+exact"); err != nil {
		a.setStatus("mpv: %v", err)
	}
}

// posNow reads the position from mpv, which is more precise than the last
// (throttled) UI update.
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
	a.markIn = a.posNow()
	if a.markOut > 0 && a.markOut <= a.markIn {
		a.markOut = 0
	}
	a.applyLoop()
	a.updateControls()
}

func (a *app) setOut() {
	if a.info == nil {
		return
	}
	t := a.posNow()
	if t <= a.markIn {
		a.setStatus("The end must come after the start (%s).", media.FormatTime(a.markIn))
		return
	}
	a.markOut = t
	a.applyLoop()
	a.updateControls()
}

func (a *app) clearMarks() {
	a.markIn, a.markOut = 0, 0
	a.applyLoop()
	a.updateControls()
}

// applyLoop uses mpv's A-B loop to repeat the selection in the preview.
func (a *app) applyLoop() {
	if a.player == nil {
		return
	}
	from, to := "no", "no"
	if a.loopCB.Checked() && a.info != nil {
		from, to = formatSeconds(a.markIn), formatSeconds(a.cutEnd())
	}
	if err := a.player.SetProperty("ab-loop-a", from); err != nil {
		a.setStatus("mpv: %v", err)
	}
	if err := a.player.SetProperty("ab-loop-b", to); err != nil {
		a.setStatus("mpv: %v", err)
	}
}

func formatSeconds(s float64) string { return strconv.FormatFloat(s, 'f', 3, 64) }
