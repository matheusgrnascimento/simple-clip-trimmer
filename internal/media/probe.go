package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Info holds what matters about a video file.
type Info struct {
	Path        string
	Duration    float64 // seconds
	Width       int
	Height      int
	FPS         float64
	VideoCodec  string
	PixFmt      string
	AudioTracks int
	AudioCodec  string
	BitRate     int64 // bits per second of the whole file
	Size        int64 // bytes
}

type probeOutput struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		PixFmt       string `json:"pix_fmt"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
		Size     string `json:"size"`
	} `json:"format"`
}

// Probe runs ffprobe and returns the file's information.
func Probe(ctx context.Context, path string) (Info, error) {
	ffprobe, err := FindTool("ffprobe")
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("ffprobe: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseProbe(path, out)
}

func parseProbe(path string, out []byte) (Info, error) {
	var po probeOutput
	if err := json.Unmarshal(out, &po); err != nil {
		return Info{}, fmt.Errorf("ffprobe json: %w", err)
	}

	info := Info{Path: path}
	info.Duration, _ = strconv.ParseFloat(po.Format.Duration, 64)
	info.BitRate, _ = strconv.ParseInt(po.Format.BitRate, 10, 64)
	info.Size, _ = strconv.ParseInt(po.Format.Size, 10, 64)

	gotVideo := false
	for _, s := range po.Streams {
		switch s.CodecType {
		case "video":
			if gotVideo {
				continue // skip cover art and other extra pictures
			}
			gotVideo = true
			info.VideoCodec = s.CodecName
			info.Width, info.Height = s.Width, s.Height
			info.PixFmt = s.PixFmt
			info.FPS = parseRate(s.AvgFrameRate)
			if info.FPS == 0 {
				info.FPS = parseRate(s.RFrameRate)
			}
		case "audio":
			info.AudioTracks++
			if info.AudioCodec == "" {
				info.AudioCodec = s.CodecName
			}
		}
	}
	if !gotVideo {
		return info, errors.New("the file has no video track")
	}
	return info, nil
}

// parseRate parses an ffprobe frame rate such as "60/1" or "29.97".
func parseRate(r string) float64 {
	num, den, ok := strings.Cut(r, "/")
	if !ok {
		v, _ := strconv.ParseFloat(r, 64)
		return v
	}
	n, _ := strconv.ParseFloat(num, 64)
	d, _ := strconv.ParseFloat(den, 64)
	if d == 0 {
		return 0
	}
	return n / d
}
