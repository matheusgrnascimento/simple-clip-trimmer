# Simple Clip Trimmer

Simple desktop app for trimming, resizing and compressing videos, such as the large recordings made by the NVIDIA app. The project is written in Go, uses native Win32 controls (`walk`) for the interface, libmpv for a hardware-accelerated preview and the FFmpeg command line tools to encode.

## Features

- open a video from the **Open video** button, by dropping it on the window or by passing it as an argument;
- file information on top: resolution, frame rate, codecs, number of audio tracks, duration, size and bit rate;
- preview with GPU decoding (D3D11VA/NVDEC) for H.264, HEVC and AV1, without depending on Windows codec extensions;
- frame-accurate navigation: position slider, frame by frame and 5 s steps;
- **Set start** / **Set end** marks for the part to keep, with an optional loop of the selection in the preview;
- output resolution (original, 1440p, 1080p, 720p or 480p, never upscaling), encoder and a Light/Medium/High quality preset;
- encoders: H.264, HEVC and AV1 with NVENC (NVIDIA GPU) or H.264 with x264 (CPU). The app checks at startup which ones work on the machine, falls back to x264 without NVENC, and warns before using an unavailable one;
- every audio track of the input is kept (the NVIDIA app can record the microphone on its own track);
- export with progress, speed and time left, and **Cancel**, which removes the partial file;
- at the end, the output size and how much smaller it is than the same part of the source.

## Requirements

- Go (tested with 1.27);
- GCC from MinGW-w64 (tested with WinLibs GCC 16.2, UCRT), needed by cgo for the libmpv binding;
- FFmpeg (`ffmpeg.exe` and `ffprobe.exe`) in `PATH` or next to the executable. NVENC encoders need an NVIDIA GPU; AV1 NVENC needs an RTX 40 series or newer;
- libmpv development files (`libmpv-2.dll`, `libmpv.dll.a` and headers) in `third_party\libmpv`.

### Windows

The `setup.ps1` script installs Go, GCC and FFmpeg with `winget` (skipping the ones already in `PATH`), downloads a pinned libmpv build into `third_party\libmpv` (about 30 MB) and builds the app:

```powershell
powershell -ExecutionPolicy Bypass -File .\setup.ps1
```

To use tools that are already installed, or to only download libmpv and build:

```powershell
.\setup.ps1 -SkipInstall
```

## Build

With the requirements above in place:

```powershell
.\build.ps1           # build\simple-clip-trimmer.exe, without a console window
.\build.ps1 -Console  # keeps the console, to see logs and panics
```

`build.ps1` builds with cgo, then runs `tools\setmanifest` to replace the default manifest that the MinGW GCC links into every executable with `app.manifest` (Common Controls v6 and DPI awareness, which `walk` requires), and copies `libmpv-2.dll` next to the executable.

## Running

```powershell
.\build\simple-clip-trimmer.exe
.\build\simple-clip-trimmer.exe "C:\Users\me\Videos\clip.mp4"
```

`build\` holds everything the app needs except FFmpeg: `simple-clip-trimmer.exe` and `libmpv-2.dll`. Copy `ffmpeg.exe` and `ffprobe.exe` there too to run it on a machine without FFmpeg in `PATH`.

### Shortcuts

| Key | Action |
|---|---|
| `Space` | play / pause |
| `,` / `.` | previous / next frame |
| `←` / `→` | back / forward 5 s |
| `I` / `O` | set start / end |
| `Ctrl+O` | open video |
| `Ctrl+E` | export |

## Export

The export runs `ffmpeg` with:

- `-ss <start>` before the input and `-t <length>`: a fast seek, and since the video is re-encoded the cut is exact to the frame;
- `-map 0:v:0 -map 0:a?`: the main video and every audio track;
- `scale=-2:<height>:flags=lanczos` when a smaller resolution is chosen;
- NVENC with `-preset p5 -tune hq -rc vbr -cq <n> -b:v 0`, or x264 with `-preset medium -crf <n>`, where the quality preset picks `n`;
- AAC audio at 128, 160 or 192 kb/s, `yuv420p` video and `+faststart` MP4 output, which plays everywhere.

## Tests

```powershell
go test ./...
```

The `internal/media` tests check the ffmpeg arguments, the ffprobe and progress parsers, and run real exports: they generate a 2560x1440 60 fps sample with two audio tracks and export a trimmed 720p part with every encoder available on the machine, checking resolution, duration and audio tracks, plus a canceled export. `go test -short ./...` skips the tests that run ffmpeg.

## License

MIT. See [LICENSE](LICENSE). libmpv and FFmpeg are not part of the repository and keep their own licenses (LGPL/GPL depending on the build).
