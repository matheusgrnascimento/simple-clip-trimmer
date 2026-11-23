package media

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Job describes one export.
type Job struct {
	Input   string
	Output  string
	Start   float64 // seconds; 0 = from the beginning
	End     float64 // seconds; 0 = to the end
	Height  int     // 0 = original resolution
	Encoder Encoder
	Quality Quality
	// SrcHeight avoids upscaling videos smaller than the target height.
	SrcHeight int
	// Crop keeps only this part of the picture, in source pixels (nil keeps
	// the whole picture). Its size must be even.
	Crop *image.Rectangle
	// NoAudio drops every audio track.
	NoAudio bool
}

// Args builds the ffmpeg command line (without the executable).
func (j Job) Args() []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-nostats", "-progress", "pipe:1"}

	// Hardware decoding when available; ffmpeg falls back to the CPU.
	args = append(args, "-hwaccel", "auto")

	// -ss before -i seeks fast; since the video is re-encoded the cut is exact.
	if j.Start > 0 {
		args = append(args, "-ss", formatSeconds(j.Start))
	}
	args = append(args, "-i", j.Input)
	if j.End > 0 && j.End > j.Start {
		args = append(args, "-t", formatSeconds(j.End-j.Start))
	}

	// Keep the main video and EVERY audio track (the NVIDIA app can record the
	// microphone and the system sound on separate tracks), unless the audio
	// is removed.
	args = append(args, "-map", "0:v:0")
	if !j.NoAudio {
		args = append(args, "-map", "0:a?")
	}

	// Crop first, then scale what is left.
	var filters []string
	srcHeight := j.SrcHeight
	if j.Crop != nil {
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d", j.Crop.Dx(), j.Crop.Dy(), j.Crop.Min.X, j.Crop.Min.Y))
		srcHeight = j.Crop.Dy()
	}
	if j.Height > 0 && (srcHeight == 0 || j.Height < srcHeight) {
		filters = append(filters, fmt.Sprintf("scale=-2:%d:flags=lanczos", j.Height))
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}

	args = append(args, j.Encoder.videoArgs(j.Quality)...)
	if j.NoAudio {
		args = append(args, "-an")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", []string{"128k", "160k", "192k"}[j.Quality])
	}
	args = append(args, "-movflags", "+faststart", j.Output)
	return args
}

// Progress is reported while exporting.
type Progress struct {
	Fraction float64 // 0..1
	Elapsed  float64 // seconds of output already processed
	Speed    string  // for example "3.2x"
}

// Run executes ffmpeg and calls onProgress periodically, from the calling
// goroutine. totalDur is the expected output duration, used for the fraction.
func Run(ctx context.Context, job Job, totalDur float64, onProgress func(Progress)) error {
	ffmpeg, err := FindTool("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, job.Args()...)
	hideWindow(cmd)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	readProgress(stdout, totalDur, onProgress)

	err = cmd.Wait()
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// readProgress parses the key=value blocks that "ffmpeg -progress" writes.
func readProgress(r io.Reader, totalDur float64, onProgress func(Progress)) {
	var p Progress
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "out_time_us", "out_time_ms": // both are in microseconds
			if us, err := strconv.ParseInt(val, 10, 64); err == nil && us >= 0 {
				p.Elapsed = float64(us) / 1e6
			}
		case "speed":
			p.Speed = strings.TrimSpace(val)
		case "progress":
			if totalDur > 0 {
				p.Fraction = min(p.Elapsed/totalDur, 1)
			}
			if val == "end" {
				p.Fraction = 1
			}
			if onProgress != nil {
				onProgress(p)
			}
		}
	}
}

// ProbeEncoder checks whether an encoder actually works on this machine
// (NVENC, for example, needs an NVIDIA GPU with a recent driver).
func ProbeEncoder(ctx context.Context, enc Encoder) bool {
	ffmpeg, err := FindTool("ffmpeg")
	if err != nil {
		return false
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=256x256:d=0.2",
		"-c:v", enc.String(), "-f", "null", "-")
	hideWindow(cmd)
	return cmd.Run() == nil
}

func formatSeconds(s float64) string {
	return strconv.FormatFloat(s, 'f', 3, 64)
}

// FormatTime formats seconds as H:MM:SS.mmm, or MM:SS.mmm under one hour.
func FormatTime(s float64) string {
	if s < 0 {
		s = 0
	}
	ms := int64(s*1000 + 0.5)
	h := ms / 3_600_000
	m := ms / 60_000 % 60
	sec := ms / 1000 % 60
	ms %= 1000
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, sec, ms)
	}
	return fmt.Sprintf("%02d:%02d.%03d", m, sec, ms)
}

// FormatSize formats a byte count as KB, MB, GB or TB.
func FormatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 3; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGT"[exp])
}
