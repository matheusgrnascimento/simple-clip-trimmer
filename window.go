package main

import (
	"fmt"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"github.com/matheusgrnascimento/simple-clip-trimmer/internal/media"
)

// buildWindow creates the main window and its widgets.
func (a *app) buildWindow() error {
	resNames := make([]string, len(media.Resolutions))
	for i, r := range media.Resolutions {
		resNames[i] = r.Name
	}
	encNames := make([]string, len(media.Encoders))
	for i, e := range media.Encoders {
		encNames[i] = e.Name
	}

	return MainWindow{
		AssignTo: &a.mw,
		Title:    appName,
		MinSize:  Size{Width: 900, Height: 600},
		Size:     Size{Width: 1200, Height: 820},
		Layout:   VBox{Spacing: 6},
		OnDropFiles: func(files []string) {
			if len(files) > 0 {
				a.load(files[0])
			}
		},
		MenuItems: a.menu(),
		Children: []Widget{
			// Top: open button and file information.
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{Text: "Open video...", OnClicked: a.openDialog},
					Label{AssignTo: &a.infoLbl, Text: "Drop a video on the window or click Open video.", EllipsisMode: EllipsisEnd},
				},
			},

			// Video area: mpv draws inside this window.
			CustomWidget{
				AssignTo:         &a.video,
				StretchFactor:    10,
				MinSize:          Size{Width: 320, Height: 180},
				Background:       SolidColorBrush{Color: walk.RGB(0, 0, 0)},
				ClearsBackground: true,
				Paint:            func(*walk.Canvas, walk.Rectangle) error { return nil },
			},

			// Transport.
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{AssignTo: &a.prevBtn, Text: "◀|", ToolTipText: "Previous frame (,)", MaxSize: Size{Width: 40}, OnClicked: func() { a.cmd("frame-back-step") }},
					PushButton{AssignTo: &a.playBtn, Text: "▶", ToolTipText: "Play/Pause (Space)", MaxSize: Size{Width: 50}, OnClicked: a.togglePlay},
					PushButton{AssignTo: &a.nextBtn, Text: "|▶", ToolTipText: "Next frame (.)", MaxSize: Size{Width: 40}, OnClicked: func() { a.cmd("frame-step") }},
					Slider{
						AssignTo:       &a.seek,
						StretchFactor:  1,
						Tracking:       true,
						ToolTipsHidden: true,
						OnValueChanged: a.onSliderChanged,
					},
					Label{AssignTo: &a.timeLbl, Text: "00:00.000 / 00:00.000", MinSize: Size{Width: 170}},
				},
			},

			// Trim.
			GroupBox{
				Title:  "Trim",
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &a.markInBtn, Text: "[ Set start (I)", OnClicked: a.setIn},
					Label{AssignTo: &a.inLbl, MinSize: Size{Width: 120}},
					PushButton{AssignTo: &a.markOutBtn, Text: "Set end (O) ]", OnClicked: a.setOut},
					Label{AssignTo: &a.outLbl, MinSize: Size{Width: 140}},
					PushButton{AssignTo: &a.clearBtn, Text: "Clear", OnClicked: a.clearMarks},
					CheckBox{AssignTo: &a.loopCB, Text: "Loop selection in preview", OnCheckedChanged: a.applyLoop},
					HSpacer{},
					Label{AssignTo: &a.cutLbl},
				},
			},

			// Output.
			GroupBox{
				Title:  "Output",
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Resolution:"},
					ComboBox{AssignTo: &a.resCB, Model: resNames, CurrentIndex: 2},
					Label{Text: "Encoder:"},
					ComboBox{AssignTo: &a.encCB, Model: encNames, CurrentIndex: 0},
					Label{Text: "Quality:"},
					ComboBox{AssignTo: &a.qualCB, Model: media.Qualities, CurrentIndex: 1},
					HSpacer{},
					PushButton{AssignTo: &a.exportBtn, Text: "Export...", MinSize: Size{Width: 110}, OnClicked: a.export},
				},
			},

			// Progress.
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					ProgressBar{AssignTo: &a.progress, MaxValue: progressScale, StretchFactor: 1},
					PushButton{AssignTo: &a.cancelBtn, Text: "Cancel", Enabled: false, OnClicked: a.cancel},
				},
			},
			Label{AssignTo: &a.statusLbl, Text: "Ready.", EllipsisMode: EllipsisEnd},
		},
	}.Create()
}

// menu returns the menu bar. Its shortcuts work wherever the focus is.
func (a *app) menu() []MenuItem {
	return []MenuItem{
		Menu{
			Text: "&File",
			Items: []MenuItem{
				Action{Text: "&Open video...", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyO}, OnTriggered: a.openDialog},
				Action{Text: "&Export...", Shortcut: Shortcut{Modifiers: walk.ModControl, Key: walk.KeyE}, OnTriggered: a.export},
				Separator{},
				Action{Text: "E&xit", OnTriggered: func() { a.mw.Close() }},
			},
		},
		Menu{
			Text: "&Playback",
			Items: []MenuItem{
				Action{Text: "Play / Pause", Shortcut: Shortcut{Key: walk.KeySpace}, OnTriggered: a.togglePlay},
				Action{Text: "Previous frame", Shortcut: Shortcut{Key: walk.KeyOEMComma}, OnTriggered: func() { a.cmd("frame-back-step") }},
				Action{Text: "Next frame", Shortcut: Shortcut{Key: walk.KeyOEMPeriod}, OnTriggered: func() { a.cmd("frame-step") }},
				Action{Text: "Back 5 s", Shortcut: Shortcut{Key: walk.KeyLeft}, OnTriggered: func() { a.cmd("seek", "-5", "relative+exact") }},
				Action{Text: "Forward 5 s", Shortcut: Shortcut{Key: walk.KeyRight}, OnTriggered: func() { a.cmd("seek", "5", "relative+exact") }},
				Separator{},
				Action{Text: "Set start", Shortcut: Shortcut{Key: walk.KeyI}, OnTriggered: a.setIn},
				Action{Text: "Set end", Shortcut: Shortcut{Key: walk.KeyO}, OnTriggered: a.setOut},
			},
		},
	}
}

func (a *app) setStatus(format string, args ...any) {
	a.statusLbl.SetText(fmt.Sprintf(format, args...))
}

func (a *app) exporting() bool { return a.cancelExport != nil }

// updateControls refreshes enabled states and labels from the current state.
func (a *app) updateControls() {
	loaded := a.info != nil
	busy := a.exporting()
	for _, w := range []walk.Widget{a.playBtn, a.prevBtn, a.nextBtn, a.seek, a.markInBtn, a.markOutBtn, a.clearBtn, a.loopCB} {
		w.SetEnabled(loaded)
	}
	for _, w := range []walk.Widget{a.resCB, a.encCB, a.qualCB} {
		w.SetEnabled(!busy)
	}
	a.exportBtn.SetEnabled(loaded && !busy)
	a.cancelBtn.SetEnabled(busy)

	if a.paused {
		a.playBtn.SetText("▶")
	} else {
		a.playBtn.SetText("❚❚")
	}

	a.inLbl.SetText("Start: " + media.FormatTime(a.markIn))
	if a.markOut > 0 {
		a.outLbl.SetText("End: " + media.FormatTime(a.markOut))
	} else {
		a.outLbl.SetText("End: (end of video)")
	}
	if loaded {
		a.cutLbl.SetText("Selection: " + media.FormatTime(a.cutEnd()-a.markIn))
	} else {
		a.cutLbl.SetText("")
	}
}

// cutEnd is the end of the selection, in seconds.
func (a *app) cutEnd() float64 {
	if a.markOut > 0 {
		return a.markOut
	}
	return a.duration
}
