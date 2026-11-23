package media

import "strconv"

// Codec is the output video format the user picks.
type Codec int

// Output codecs.
const (
	CodecH264 Codec = iota
	CodecHEVC
	CodecAV1
)

// Codecs in the order they appear in the interface.
var Codecs = []struct {
	ID   Codec
	Name string
	Hint string
}{
	{CodecH264, "H.264", "most compatible"},
	{CodecHEVC, "HEVC", "smaller files"},
	{CodecAV1, "AV1", "smallest files"},
}

// Encoder identifies an ffmpeg video encoder.
type Encoder int

// Supported encoders. NVENC runs on NVIDIA GPUs; the others on the CPU.
const (
	EncH264NVENC Encoder = iota
	EncHEVCNVENC
	EncAV1NVENC
	EncH264CPU // libx264
	EncHEVCCPU // libx265
	EncAV1CPU  // libsvtav1
)

// AllEncoders lists every supported encoder.
var AllEncoders = []Encoder{EncH264NVENC, EncHEVCNVENC, EncAV1NVENC, EncH264CPU, EncHEVCCPU, EncAV1CPU}

// Candidates returns the encoders for a codec, fastest first.
func Candidates(c Codec) []Encoder {
	switch c {
	case CodecHEVC:
		return []Encoder{EncHEVCNVENC, EncHEVCCPU}
	case CodecAV1:
		return []Encoder{EncAV1NVENC, EncAV1CPU}
	default:
		return []Encoder{EncH264NVENC, EncH264CPU}
	}
}

// Hardware reports whether the encoder runs on the GPU.
func (e Encoder) Hardware() bool {
	return e == EncH264NVENC || e == EncHEVCNVENC || e == EncAV1NVENC
}

// String returns the ffmpeg encoder name.
func (e Encoder) String() string {
	switch e {
	case EncHEVCNVENC:
		return "hevc_nvenc"
	case EncAV1NVENC:
		return "av1_nvenc"
	case EncH264CPU:
		return "libx264"
	case EncHEVCCPU:
		return "libx265"
	case EncAV1CPU:
		return "libsvtav1"
	default:
		return "h264_nvenc"
	}
}

// Quality is the size/quality preset.
type Quality int

// Quality presets.
const (
	QualityLight  Quality = iota // smaller file
	QualityMedium                // balanced
	QualityHigh                  // better picture
)

// Qualities in the order they appear in the interface.
var Qualities = []struct {
	ID   Quality
	Name string
	Hint string
}{
	{QualityLight, "Light", "smaller file"},
	{QualityMedium, "Medium", "balanced"},
	{QualityHigh, "High", "better picture"},
}

// Resolutions offered in the interface, by height. 0 keeps the original.
var Resolutions = []int{0, 1440, 1080, 720, 480}

// videoArgs returns the ffmpeg video encoder arguments for a quality.
func (e Encoder) videoArgs(q Quality) []string {
	pick := func(light, medium, high int) string {
		return strconv.Itoa([]int{light, medium, high}[q])
	}
	nvenc := func(cq string) []string {
		return []string{"-c:v", e.String(), "-preset", "p5", "-tune", "hq",
			"-rc", "vbr", "-cq", cq, "-b:v", "0"}
	}
	switch e {
	case EncH264NVENC:
		return append(nvenc(pick(30, 26, 22)), "-spatial-aq", "1", "-pix_fmt", "yuv420p")
	case EncHEVCNVENC:
		return append(nvenc(pick(32, 28, 24)), "-spatial-aq", "1", "-pix_fmt", "yuv420p", "-tag:v", "hvc1")
	case EncAV1NVENC:
		return append(nvenc(pick(38, 32, 27)), "-pix_fmt", "yuv420p")
	case EncHEVCCPU:
		return []string{"-c:v", "libx265", "-preset", "medium", "-crf", pick(31, 27, 23),
			"-x265-params", "log-level=error", "-pix_fmt", "yuv420p", "-tag:v", "hvc1"}
	case EncAV1CPU:
		return []string{"-c:v", "libsvtav1", "-preset", "8", "-crf", pick(40, 34, 28), "-pix_fmt", "yuv420p"}
	default: // EncH264CPU
		return []string{"-c:v", "libx264", "-preset", "medium", "-crf", pick(28, 23, 19), "-pix_fmt", "yuv420p"}
	}
}
