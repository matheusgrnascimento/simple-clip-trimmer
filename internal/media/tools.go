// Package media wraps the ffprobe and ffmpeg command line tools.
package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// createNoWindow stops ffmpeg and ffprobe from opening a console window when
// the app is built with -H windowsgui.
const createNoWindow = 0x08000000

// FindTool looks for an executable next to the app's .exe first, then in PATH.
func FindTool(name string) (string, error) {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), name+".exe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return exec.LookPath(name)
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
