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
		a.flash("Wait for the export to finish before opening another video.")
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
		a.flash("Wait for the export to finish before opening another video.")
		return
	}
	a.flash("Opening " + filepath.Base(path) + "…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := media.Probe(ctx, path)
		a.mw.Synchronize(func() {
			if err != nil {
				a.flash("Couldn't open that file.")
				walk.MsgBox(a.mw, appName, "Couldn't read the video:\n"+err.Error(), walk.MsgBoxIconError)
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
	a.result = nil
	a.note = ""
	if a.cr.active {
		a.exitCrop(false)
	}
	a.cropRect = nil
	a.setVideoCrop(nil) // mpv keeps the crop across files
	if a.height >= info.Height {
		a.height = 0 // never upscale: keep the original
	}

	a.mw.SetTitle(filepath.Base(info.Path) + " - " + appName)
	if err := a.player.Command("loadfile", info.Path); err != nil {
		a.flash("Playback: " + err.Error())
	}
	a.stage.Invalidate()
	a.refresh()
}

// describe summarizes the file for the footer.
func describe(i media.Info) string {
	parts := []string{
		fmt.Sprintf("%d×%d", i.Width, i.Height),
		fmt.Sprintf("%.4g fps", i.FPS),
		codecName(i.VideoCodec),
	}
	switch i.AudioTracks {
	case 0:
		parts = append(parts, "no audio")
	case 1:
		parts = append(parts, "1 audio track")
	default:
		parts = append(parts, fmt.Sprintf("%d audio tracks", i.AudioTracks))
	}
	parts = append(parts, media.FormatSize(i.Size))
	if i.BitRate > 0 {
		parts = append(parts, fmt.Sprintf("%.1f Mb/s", float64(i.BitRate)/1e6))
	}
	return strings.Join(parts, "  ·  ")
}

// codecName turns an ffprobe codec name into its usual spelling.
func codecName(c string) string {
	switch c {
	case "h264":
		return "H.264"
	case "hevc":
		return "HEVC"
	case "av1":
		return "AV1"
	case "vp9":
		return "VP9"
	}
	return strings.ToUpper(c)
}
