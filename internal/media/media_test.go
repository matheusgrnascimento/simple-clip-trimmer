package media

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestJobArgs(t *testing.T) {
	j := Job{Input: "in.mp4", Output: "out.mp4", Start: 2.5, End: 6, Height: 720,
		Encoder: EncHEVCNVENC, Quality: QualityHigh, SrcHeight: 1440}
	got := strings.Join(j.Args(), " ")
	for _, want := range []string{
		"-ss 2.500 -i in.mp4 -t 3.500",
		"-map 0:v:0 -map 0:a?",
		"-vf scale=-2:720:flags=lanczos",
		"-c:v hevc_nvenc",
		"-cq 24",
		"-b:a 192k",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, " out.mp4") {
		t.Errorf("output must be the last argument: %s", got)
	}
}

func TestJobArgsNoUpscaleNoCut(t *testing.T) {
	j := Job{Input: "in.mp4", Output: "out.mp4", Height: 1080, SrcHeight: 720, Encoder: EncH264CPU}
	args := j.Args()
	for _, flag := range []string{"-ss", "-t", "-vf"} {
		if slices.Contains(args, flag) {
			t.Errorf("unexpected %s in %v", flag, args)
		}
	}
}

func TestParseProbe(t *testing.T) {
	out := `{"streams":[
		{"codec_type":"video","codec_name":"hevc","width":2560,"height":1440,"pix_fmt":"yuv420p","avg_frame_rate":"60000/1001"},
		{"codec_type":"audio","codec_name":"aac"},
		{"codec_type":"audio","codec_name":"aac"},
		{"codec_type":"video","codec_name":"mjpeg","width":320,"height":180}],
		"format":{"duration":"12.5","bit_rate":"50000000","size":"78125000"}}`
	info, err := parseProbe("x.mp4", []byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 2560 || info.Height != 1440 || info.VideoCodec != "hevc" || info.AudioTracks != 2 {
		t.Errorf("unexpected info: %+v", info)
	}
	if math.Abs(info.FPS-59.94) > 0.01 || info.Duration != 12.5 || info.Size != 78125000 {
		t.Errorf("unexpected numbers: %+v", info)
	}
	if _, err := parseProbe("a.m4a", []byte(`{"streams":[{"codec_type":"audio"}],"format":{}}`)); err == nil {
		t.Error("a file without video must fail")
	}
}

func TestReadProgress(t *testing.T) {
	in := "out_time_us=1000000\nspeed=2.5x\nprogress=continue\nout_time_us=4000000\nspeed=3x\nprogress=end\n"
	var got []Progress
	readProgress(strings.NewReader(in), 4, func(p Progress) { got = append(got, p) })
	if len(got) != 2 {
		t.Fatalf("got %d updates, want 2", len(got))
	}
	if got[0].Fraction != 0.25 || got[0].Speed != "2.5x" {
		t.Errorf("first update: %+v", got[0])
	}
	if got[1].Fraction != 1 || got[1].Elapsed != 4 {
		t.Errorf("last update: %+v", got[1])
	}
}

func TestFormatTime(t *testing.T) {
	cases := map[float64]string{0: "00:00.000", 61.5: "01:01.500", 3725.042: "1:02:05.042", -1: "00:00.000"}
	for in, want := range cases {
		if got := FormatTime(in); got != want {
			t.Errorf("FormatTime(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := map[int64]string{512: "512 B", 1536: "1.5 KB", 5 << 30: "5.0 GB"}
	for in, want := range cases {
		if got := FormatSize(in); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", in, got, want)
		}
	}
}

// makeSample builds a 10 s 2560x1440 60 fps test video with two audio tracks,
// like the NVIDIA app does when the microphone is recorded separately.
func makeSample(t *testing.T, dir string) string {
	t.Helper()
	ffmpeg, err := FindTool("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not found")
	}
	out := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=2560x1440:r=60:d=10",
		"-f", "lavfi", "-i", "sine=f=440:d=10",
		"-f", "lavfi", "-i", "sine=f=880:d=10",
		"-map", "0", "-map", "1", "-map", "2",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p",
		"-c:a", "aac", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating sample: %v\n%s", err, b)
	}
	return out
}

func TestProbeAndExport(t *testing.T) {
	if testing.Short() {
		t.Skip("runs ffmpeg")
	}
	dir := t.TempDir()
	in := makeSample(t, dir)

	info, err := Probe(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 2560 || info.Height != 1440 || math.Abs(info.FPS-60) > 0.01 || info.AudioTracks != 2 {
		t.Fatalf("unexpected probe result: %+v", info)
	}

	for _, enc := range Encoders {
		t.Run(enc.Name, func(t *testing.T) {
			if !ProbeEncoder(context.Background(), enc.ID) {
				t.Skip("encoder not available on this machine")
			}
			out := filepath.Join(dir, "out.mp4")
			job := Job{Input: in, Output: out, Start: 2.5, End: 6, Height: 720,
				Encoder: enc.ID, Quality: QualityMedium, SrcHeight: info.Height}

			var last Progress
			start := time.Now()
			if err := Run(context.Background(), job, job.End-job.Start, func(p Progress) { last = p }); err != nil {
				t.Fatal(err)
			}
			if last.Fraction != 1 {
				t.Errorf("final progress = %v, want 1", last.Fraction)
			}

			got, err := Probe(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			if got.Width != 1280 || got.Height != 720 {
				t.Errorf("resolution %dx%d, want 1280x720", got.Width, got.Height)
			}
			if math.Abs(got.Duration-3.5) > 0.1 {
				t.Errorf("duration %.3f, want 3.5", got.Duration)
			}
			if got.AudioTracks != 2 {
				t.Errorf("audio tracks = %d, want 2", got.AudioTracks)
			}
			st, err := os.Stat(out)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %s in %v", enc.Name, FormatSize(st.Size()), time.Since(start).Round(time.Millisecond))
		})
	}
}

func TestRunCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("runs ffmpeg")
	}
	dir := t.TempDir()
	in := makeSample(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	job := Job{Input: in, Output: filepath.Join(dir, "c.mp4"), Encoder: EncH264CPU, Quality: QualityHigh}
	time.AfterFunc(200*time.Millisecond, cancel)
	if err := Run(ctx, job, 10, nil); err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
