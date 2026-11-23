package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tailscale/walk"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
)

func (a *app) openDialog() {
	if a.exporting() {
		return
	}
	dlg := walk.FileDialog{
		Title:  "Open video",
		Filter: "Videos (*.mp4;*.mkv;*.mov;*.webm;*.avi)|*.mp4;*.mkv;*.mov;*.webm;*.avi|All files (*.*)|*.*",
	}
	if a.info != nil {
		dlg.InitialDirPath = filepath.Dir(a.info.Path)
	} else if home, err := os.UserHomeDir(); err == nil {
		dlg.InitialDirPath = filepath.Join(home, "Videos")
	}
	if ok, err := dlg.ShowOpen(a.mw); err != nil || !ok {
		return
	}
	a.load(dlg.FilePath)
}

// load probes the file off the UI thread and then opens it in the player.
func (a *app) load(path string) {
	if a.exporting() {
		a.setStatus("Wait for the export to finish before opening another video.")
		return
	}
	a.setStatus("Reading %s...", filepath.Base(path))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := media.Probe(ctx, path)
		a.mw.Synchronize(func() {
			if err != nil {
				a.setStatus("Could not read the file.")
				walk.MsgBox(a.mw, appName, "Could not read the video:\n"+err.Error(), walk.MsgBoxIconError)
				return
			}
			a.opened(info)
		})
	}()
}

func (a *app) opened(info media.Info) {
	a.info = &info
	a.duration = info.Duration
	a.pos = 0
	a.markIn, a.markOut = 0, 0
	a.loopCB.SetChecked(false)
	a.seek.SetRange(0, int(info.Duration*sliderScale))
	a.setSlider(0)

	a.mw.SetTitle(appName + " - " + filepath.Base(info.Path))
	a.infoLbl.SetText(describe(info))
	a.progress.SetValue(0)
	if err := a.player.Command("loadfile", info.Path); err != nil {
		a.setStatus("mpv: %v", err)
	} else {
		a.setStatus("Ready. Space = play/pause · , and . = frame by frame · I/O = set start/end")
	}
	a.updateControls()
}

// describe summarizes the file for the information label.
func describe(i media.Info) string {
	parts := []string{
		filepath.Base(i.Path),
		fmt.Sprintf("%d×%d @ %.4g fps", i.Width, i.Height, i.FPS),
		i.VideoCodec + " / " + i.PixFmt,
	}
	switch i.AudioTracks {
	case 0:
		parts = append(parts, "no audio")
	case 1:
		parts = append(parts, "audio "+i.AudioCodec)
	default:
		parts = append(parts, fmt.Sprintf("%d audio tracks", i.AudioTracks))
	}
	parts = append(parts, media.FormatTime(i.Duration), media.FormatSize(i.Size))
	if i.BitRate > 0 {
		parts = append(parts, fmt.Sprintf("%.1f Mb/s", float64(i.BitRate)/1e6))
	}
	return strings.Join(parts, "  ·  ")
}
