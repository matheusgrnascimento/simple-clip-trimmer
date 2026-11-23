package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/fluent"
	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
)

// exportProgress is the state of the running export.
type exportProgress struct {
	fraction float64
	speed    string
	started  time.Time
}

func (p *exportProgress) text() string {
	s := fmt.Sprintf("Exporting  %.0f%%", p.fraction*100)
	if p.speed != "" {
		s += "  ·  " + p.speed
	}
	if p.fraction > 0.02 && p.fraction < 1 {
		elapsed := time.Since(p.started)
		left := time.Duration(float64(elapsed) * (1 - p.fraction) / p.fraction)
		s += "  ·  about " + left.Round(time.Second).String() + " left"
	}
	return s
}

// exportResult is the outcome of the last finished export.
type exportResult struct {
	path    string
	summary string
}

// checkTools checks for ffmpeg/ffprobe and, off the UI thread, picks the
// fastest working encoder for each codec.
func (a *app) checkTools() {
	_, errFF := media.FindTool("ffmpeg")
	_, errFP := media.FindTool("ffprobe")
	if errFF != nil || errFP != nil {
		walk.MsgBox(a.mw, appName, "ffmpeg.exe and ffprobe.exe weren't found.\nPut them next to the app or in PATH.", walk.MsgBoxIconWarning)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found := map[media.Codec]media.Encoder{}
		for _, c := range media.Codecs {
			for _, e := range media.Candidates(c.ID) {
				if media.ProbeEncoder(ctx, e) {
					found[c.ID] = e
					break
				}
			}
		}
		a.mw.Synchronize(func() {
			a.encoders = found
			if e, ok := found[media.CodecH264]; ok && !e.Hardware() {
				a.flash("No NVIDIA encoder found, so exports use the CPU (slower).")
			}
			a.refresh()
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
	if a.cr.active {
		a.exitCrop(true)
	}
	if a.encoders == nil {
		a.flash("Still checking the encoders. Try again in a moment.")
		return
	}
	enc, ok := a.encoders[a.codec]
	if !ok {
		walk.MsgBox(a.mw, appName, codecLabel(a.codec)+" isn't available in this ffmpeg build or on this GPU.\nPick another format.", walk.MsgBoxIconWarning)
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
		Height:    a.height,
		Encoder:   enc,
		Quality:   a.quality,
		SrcHeight: a.info.Height,
		Crop:      a.cropRect,
		NoAudio:   a.noAudio,
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
	a.progress = &exportProgress{started: time.Now()}
	a.result = nil
	a.note = ""
	a.refresh()

	go func() {
		err := media.Run(ctx, job, total, func(p media.Progress) {
			a.mw.Synchronize(func() {
				if a.progress != nil {
					a.progress.fraction, a.progress.speed = p.Fraction, p.Speed
					a.refresh()
				}
			})
		})
		a.mw.Synchronize(func() {
			started := a.progress.started
			cancel()
			a.cancelExport = nil
			a.progress = nil
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
		walk.MsgBox(a.mw, appName, "The output file can't be the video you're editing.", walk.MsgBoxIconWarning)
		return "", false
	}
	return out, true
}

func (a *app) onExportDone(err error, out string, srcSize int64, started time.Time) {
	switch {
	case errors.Is(err, context.Canceled):
		if rmErr := os.Remove(out); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			slog.Warn("remove partial output", "err", rmErr)
		}
		a.flash("Export canceled.")
	case err != nil:
		a.flash("Export failed.")
		walk.MsgBox(a.mw, appName, "ffmpeg failed:\n"+err.Error(), walk.MsgBoxIconError)
	default:
		summary := fmt.Sprintf("Saved %s in %s", filepath.Base(out), time.Since(started).Round(time.Second))
		if st, err := os.Stat(out); err == nil {
			summary += "  ·  " + media.FormatSize(st.Size())
			if srcSize > 0 {
				summary += fmt.Sprintf(", %.0f%% smaller", 100*(1-float64(st.Size())/float64(srcSize)))
			}
		}
		a.result = &exportResult{path: out, summary: summary}
	}
	a.refresh()
}

// showInFolder opens Explorer with the exported file selected.
func (a *app) showInFolder() {
	if a.result == nil {
		return
	}
	cmd := exec.Command("explorer.exe")
	// Explorer needs the path quoted after "/select,", which exec.Command's
	// argument quoting does not produce.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + a.result.path + `"`}
	if err := cmd.Start(); err != nil {
		a.flash("Couldn't open the folder.")
		return
	}
	go func() { _ = cmd.Wait() }()
}

// Menu item IDs (0 means the menu was dismissed).
const (
	menuQualityBase = 100
	menuCodecBase   = 200
	menuHeightBase  = 300
	menuSetStart    = 401
	menuSetEnd      = 402
	menuReset       = 403
	menuNoAudio     = 501
	menuAspectBase  = 600
)

func (a *app) qualityMenu(anchor fluent.Rect) {
	items := []fluent.MenuItem{{Header: true, Text: "Quality"}}
	for _, q := range media.Qualities {
		items = append(items, fluent.MenuItem{
			ID:      menuQualityBase + int(q.ID),
			Text:    q.Name,
			Hint:    sentence(q.Hint),
			Checked: a.quality == q.ID,
		})
	}
	items = append(items, fluent.MenuItem{Separator: true}, fluent.MenuItem{Header: true, Text: "Format"})
	for _, c := range media.Codecs {
		hint := sentence(c.Hint)
		enc, ok := a.encoders[c.ID]
		switch {
		case a.encoders != nil && !ok:
			hint = "Not available"
		case a.encoders != nil && !enc.Hardware():
			hint += " · CPU"
		}
		items = append(items, fluent.MenuItem{
			ID:       menuCodecBase + int(c.ID),
			Text:     c.Name,
			Hint:     hint,
			Checked:  a.codec == c.ID,
			Disabled: a.encoders != nil && !ok,
		})
	}
	items = append(items,
		fluent.MenuItem{Separator: true},
		fluent.MenuItem{ID: menuNoAudio, Text: "Remove audio", Toggle: true, Checked: a.noAudio},
	)
	id := a.popup(anchor, items)
	switch {
	case id == menuNoAudio:
		a.noAudio = !a.noAudio
	case id >= menuCodecBase && id < menuCodecBase+len(media.Codecs):
		a.codec = media.Codec(id - menuCodecBase)
	case id >= menuQualityBase && id < menuQualityBase+len(media.Qualities):
		a.quality = media.Quality(id - menuQualityBase)
	}
	a.refresh()
}

func (a *app) heightMenu(anchor fluent.Rect) {
	items := []fluent.MenuItem{{Header: true, Text: "Resolution"}}
	ow, oh := a.outputSize() // the crop, or the whole source
	for _, h := range media.Resolutions {
		it := fluent.MenuItem{ID: menuHeightBase + h, Text: heightName(h), Checked: a.height == h}
		switch {
		case oh == 0:
		case h == 0:
			it.Hint = fmt.Sprintf("%d×%d", ow, oh)
		case h >= oh:
			it.Disabled = true // would not be smaller than the source
		default:
			it.Hint = fmt.Sprintf("%d×%d", scaledWidth(ow, oh, h), h)
		}
		items = append(items, it)
	}
	if id := a.popup(anchor, items); id >= menuHeightBase {
		a.height = id - menuHeightBase
	}
	a.refresh()
}

// scaledWidth is the even output width ffmpeg's scale=-2:h gives.
func scaledWidth(w, h, toH int) int {
	return int(math.Round(float64(w)*float64(toH)/float64(h)/2)) * 2
}

// trackMenu is the right-click menu of the timeline.
func (a *app) trackMenu(t float64, anchor fluent.Rect) {
	items := []fluent.MenuItem{
		{ID: menuSetStart, Text: "Set start here", Hint: "I"},
		{ID: menuSetEnd, Text: "Set end here", Hint: "O"},
		{Separator: true},
		{ID: menuReset, Text: "Keep the whole video", Disabled: a.markIn == 0 && a.markOut == 0},
	}
	switch a.popup(anchor, items) {
	case menuSetStart:
		a.setInAt(t)
	case menuSetEnd:
		a.setOutAt(t)
	case menuReset:
		a.clearMarks()
	}
}

// popup opens a flyout menu above anchor, given in control bar pixels.
func (a *app) popup(anchor fluent.Rect, items []fluent.MenuItem) int {
	tl := win.POINT{X: int32(anchor.X), Y: int32(anchor.Y)}
	win.ClientToScreen(a.barW.Handle(), &tl)
	screen := fluent.Rect{X: float64(tl.X), Y: float64(tl.Y), W: anchor.W, H: anchor.H}
	id := fluent.ShowMenu(uintptr(a.mw.Handle()), screen, items, a.pal, a.fonts)
	// The menu ran its own message loop; drop the hover/pressed state.
	a.bar.hover, a.bar.pressed = hitNone, hitNone
	return id
}

// sentence capitalizes the first letter, for hints shown on their own.
func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func (a *app) qualityLabel() string {
	s := media.Qualities[a.quality].Name + "  ·  " + codecLabel(a.codec)
	if a.noAudio {
		s += "  ·  No audio"
	}
	return s
}

func (a *app) heightLabel() string { return heightName(a.height) }

func heightName(h int) string {
	if h == 0 {
		return "Original"
	}
	return fmt.Sprintf("%dp", h)
}

func codecLabel(c media.Codec) string { return media.Codecs[c].Name }

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
