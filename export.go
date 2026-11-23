package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
)

const progressScale = 1000

// checkTools checks for ffmpeg/ffprobe and, off the UI thread, which
// encoders work on this machine.
func (a *app) checkTools() {
	_, errFF := media.FindTool("ffmpeg")
	_, errFP := media.FindTool("ffprobe")
	if errFF != nil || errFP != nil {
		walk.MsgBox(a.mw, appName, "ffmpeg.exe and/or ffprobe.exe were not found.\nPut them next to the app or in PATH.", walk.MsgBoxIconWarning)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		missing := map[media.Encoder]bool{}
		for _, e := range media.Encoders {
			if !media.ProbeEncoder(ctx, e.ID) {
				missing[e.ID] = true
			}
		}
		a.mw.Synchronize(func() {
			a.encMissing = missing
			if missing[media.EncH264NVENC] && a.encCB.CurrentIndex() == 0 {
				a.encCB.SetCurrentIndex(len(media.Encoders) - 1) // x264 (CPU)
				a.setStatus("NVENC is not available on this machine; using x264 (CPU).")
			}
		})
	}()
}

func (a *app) cancel() {
	if a.cancelExport != nil {
		a.cancelExport()
	}
}

// export asks for the output file and runs ffmpeg off the UI thread.
func (a *app) export() {
	if a.info == nil || a.exporting() {
		return
	}
	if enc := media.Encoders[a.encCB.CurrentIndex()]; a.encMissing[enc.ID] {
		walk.MsgBox(a.mw, appName, enc.Name+" is not available on this machine (GPU or driver).\nPick another encoder.", walk.MsgBoxIconWarning)
		return
	}
	out, ok := a.askOutput()
	if !ok {
		return
	}

	job := media.Job{
		Input:     a.info.Path,
		Output:    out,
		Start:     a.markIn,
		End:       a.markOut,
		Height:    media.Resolutions[a.resCB.CurrentIndex()].Height,
		Encoder:   media.Encoders[a.encCB.CurrentIndex()].ID,
		Quality:   media.Quality(a.qualCB.CurrentIndex()),
		SrcHeight: a.info.Height,
	}
	total := a.cutEnd() - a.markIn
	srcSize := a.info.Size
	if a.info.Duration > 0 {
		// Size of the selected part of the source, for the before/after comparison.
		srcSize = int64(float64(a.info.Size) * total / a.info.Duration)
	}

	if !a.paused {
		if err := a.player.SetProperty("pause", "yes"); err != nil {
			slog.Warn("pause before export", "err", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelExport = cancel
	a.progress.SetValue(0)
	a.updateControls()
	a.setStatus("Exporting to %s...", filepath.Base(out))
	started := time.Now()

	go func() {
		err := media.Run(ctx, job, total, func(p media.Progress) {
			a.mw.Synchronize(func() { a.onProgress(p, started) })
		})
		a.mw.Synchronize(func() {
			cancel()
			a.cancelExport = nil
			a.updateControls()
			a.onExportDone(err, out, srcSize, started)
		})
	}()
}

// askOutput shows the save dialog and returns the chosen .mp4 path.
func (a *app) askOutput() (string, bool) {
	in := a.info.Path
	base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	dlg := walk.FileDialog{
		Title:          "Save video as",
		Filter:         "MP4 video (*.mp4)|*.mp4",
		InitialDirPath: filepath.Dir(in),
		FilePath:       base + "_trimmed.mp4",
		Flags:          win.OFN_OVERWRITEPROMPT,
	}
	if ok, err := dlg.ShowSave(a.mw); err != nil || !ok {
		return "", false
	}
	out := dlg.FilePath
	if !strings.EqualFold(filepath.Ext(out), ".mp4") {
		out += ".mp4"
	}
	if strings.EqualFold(absPath(in), absPath(out)) {
		walk.MsgBox(a.mw, appName, "The output file cannot be the input file.", walk.MsgBoxIconWarning)
		return "", false
	}
	return out, true
}

func (a *app) onProgress(p media.Progress, started time.Time) {
	a.progress.SetValue(int(p.Fraction * progressScale))
	eta := ""
	if p.Fraction > 0.02 && p.Fraction < 1 {
		elapsed := time.Since(started)
		left := time.Duration(float64(elapsed) * (1 - p.Fraction) / p.Fraction)
		eta = " · about " + left.Round(time.Second).String() + " left"
	}
	a.setStatus("Exporting... %.1f%% · speed %s%s", p.Fraction*100, p.Speed, eta)
}

func (a *app) onExportDone(err error, out string, srcSize int64, started time.Time) {
	switch {
	case errors.Is(err, context.Canceled):
		if rmErr := os.Remove(out); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			slog.Warn("remove partial output", "err", rmErr)
		}
		a.progress.SetValue(0)
		a.setStatus("Export canceled.")
	case err != nil:
		a.setStatus("Export failed.")
		walk.MsgBox(a.mw, appName, "ffmpeg failed:\n"+err.Error(), walk.MsgBoxIconError)
	default:
		a.progress.SetValue(progressScale)
		msg := fmt.Sprintf("Done in %s: %s", time.Since(started).Round(time.Second), filepath.Base(out))
		if st, err := os.Stat(out); err == nil {
			msg += " · " + media.FormatSize(st.Size())
			if srcSize > 0 {
				msg += fmt.Sprintf(" (was about %s, %.0f%% smaller)", media.FormatSize(srcSize), 100*(1-float64(st.Size())/float64(srcSize)))
			}
		}
		a.setStatus("%s", msg)
	}
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
