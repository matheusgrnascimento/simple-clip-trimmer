# Simple Clip Trimmer

Simple desktop app for trimming, resizing and compressing videos, such as the large recordings made by the NVIDIA app. The project is written in Go, uses a native Win32 window (`walk`) with a custom-drawn, Windows 11 style control bar, libmpv for a hardware-accelerated preview and the FFmpeg command line tools to encode.

## Features

- Windows 11 look: light/dark theme and accent color following the system (and updating when they change), Segoe UI Variable and Segoe Fluent Icons, and anti-aliased controls;
- a tall (48 px) title bar drawn by the app, like Photos or Media Player, with Windows 11 minimize, maximize and close buttons. The window keeps its native frame: dragging, double-click to maximize, the top edge and corners resize, the system menu, and the maximize button reports itself to Windows as one (`HTMAXBUTTON`), which is what Snap layouts look for;
- drop-down menus in the Windows 11 flyout style: rounded corners and shadow, roomy rows with hover, section headers, an accent pill on the current choice, a check mark for on/off options, and hints on the right (the output size of each resolution, what each format is for, shortcuts). They work with the mouse and with ↑/↓, Enter and Esc;
- few controls: the video, one timeline row and one footer. There is no menu bar;
- open a video by clicking the empty video area, with the open button in the footer, by dropping it on the window or by passing it as an argument;
- file information in the footer: resolution, frame rate, codec, number of audio tracks, size and bit rate;
- preview with GPU decoding (D3D11VA/NVDEC) for H.264, HEVC and AV1, without depending on Windows codec extensions. Click the video to play or pause;
- a **timeline** with the playhead and two blue **handles** that set the part to keep. Drag a handle to move it (the preview shows that frame), click or drag elsewhere on the track to seek, or right-click it for **Set start here**, **Set end here** and **Keep the whole video**. Below it, the selection's start, end and length;
- playing from inside the selection stops at its end, so the preview shows exactly what will be exported; Play at the end of the selection plays it again from its start;
- frame-accurate navigation: frame by frame and 5 s steps from the keyboard;
- **Crop** (footer button or `C`) to reframe the picture: the video area shows the current frame with a crop box over it. Drag the box to move it and its corners (or edges) to resize it, pick an aspect ratio (Free, Original, 16:9, 9:16, 1:1, 4:5 or 4:3), and move along the timeline to check the framing on other frames. **Done** (or `Enter`) applies it and **Esc** cancels; after that the preview shows only the cropped picture and the button shows its size. The crop is rounded to even sizes, which the encoders need;
- two output drop-downs: the resolution (original, 1440p, 1080p, 720p or 480p, never upscaling the picture or the crop) and the quality (Light, Medium or High) with the format (H.264, most compatible; HEVC, smaller files; AV1, smallest files) and **Remove audio**;
- each format uses the NVIDIA GPU encoder (NVENC) when it works on the machine and falls back to the CPU encoder (x264, x265 or SVT-AV1) otherwise; the app checks this at startup and marks CPU-only formats in the menu;
- every audio track of the input is kept (the NVIDIA app can record the microphone on its own track), unless **Remove audio** is on;
- **Export** turns into **Cancel** while exporting, with progress, speed and time left in the footer. Canceling removes the partial file;
- at the end, the footer shows the output size, how much smaller it is than the same part of the source, and a **Show in folder** link.

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

`build.ps1` builds with cgo, then runs `tools\winres` to write the Windows resources: `app.manifest` in place of the default manifest that the MinGW GCC links into every executable (Common Controls v6 and DPI awareness, which `walk` requires) and the app icon. Then it copies `libmpv-2.dll` next to the executable.

## Icon

The app icon is a blue rounded square (the same gradient as Simple RTSP Viewer's) with a white play triangle between two yellow trim brackets. It is drawn from code by `tools\genicon`, which renders each size of `assets\icon.ico` (16 to 256 px) directly with 4×4 supersampling, so the small sizes stay sharp, and writes the 256 px image to `assets\icon.png`:

```powershell
go run ./tools/genicon
```

`build.ps1` embeds `icon.ico` in the executable (Explorer, the Start menu and shortcuts) and the app sets it as the window icon (taskbar and Alt+Tab); the title bar draws `icon.png`, which is embedded in the binary.

## Running

```powershell
.\build\simple-clip-trimmer.exe
.\build\simple-clip-trimmer.exe "C:\Users\me\Videos\clip.mp4"
```

`build\` holds everything the app needs except FFmpeg: `simple-clip-trimmer.exe` and `libmpv-2.dll`. Copy `ffmpeg.exe` and `ffprobe.exe` there too to run it on a machine without FFmpeg in `PATH`.

### Shortcuts

The shortcuts work both when the window and when the video has the keyboard focus (they are also bound inside mpv, whose video window runs on its own thread).

| Key | Action |
|---|---|
| `Space` or click on the video | play / pause |
| `,` / `.` | previous / next frame |
| `←` / `→` | back / forward 5 s |
| `I` / `O` | set start / end at the playhead |
| `C` | crop the picture (in crop mode: apply) |
| `Enter` / `Esc` | in crop mode: apply / cancel |
| `Ctrl+O` | open video |
| `Ctrl+E` | export |

### Theme

The app follows the Windows light/dark app theme and accent color. To check one theme without changing the Windows setting:

| Variable | Effect |
|---|---|
| `SIMPLE_CLIP_TRIMMER_THEME` | `light` or `dark` forces that theme |

```powershell
$env:SIMPLE_CLIP_TRIMMER_THEME = "light"; .\build\simple-clip-trimmer.exe
```

## Export

The export runs `ffmpeg` with:

- `-ss <start>` before the input and `-t <length>`: a fast seek, and since the video is re-encoded the cut is exact to the frame;
- `-map 0:v:0 -map 0:a?`: the main video and every audio track (with **Remove audio**, only the video and `-an`);
- `crop=<w>:<h>:<x>:<y>` when the picture is cropped, then `scale=-2:<height>:flags=lanczos` when a smaller resolution is chosen;
- NVENC with `-preset p5 -tune hq -rc vbr -cq <n> -b:v 0`, or on the CPU x264/x265 with `-preset medium -crf <n>` or SVT-AV1 with `-preset 8 -crf <n>`, where the quality preset picks `n`;
- AAC audio at 128, 160 or 192 kb/s, `yuv420p` video and `+faststart` MP4 output, which plays everywhere.

## Tests

```powershell
go test ./...
```

The `internal/media` tests check the ffmpeg arguments, the ffprobe and progress parsers, and run real exports: they generate a 2560x1440 60 fps sample with two audio tracks and export a trimmed 720p part with every encoder available on the machine (GPU and CPU), checking resolution, duration and audio tracks, a cropped 9:16 export without audio, and a canceled export. `go test -short ./...` skips the tests that run ffmpeg.

Two benchmarks in `internal/fluent` measure the cost of a repaint at 100% and 150% display scaling: copying a painted frame to a window (into an off-screen bitmap), and drawing one crop-mode frame:

```powershell
go test -run '^$' -bench . -benchmem ./internal/fluent/
```

The `internal/crop` tests check the crop box geometry: hit testing, moving and resizing it (free and with a fixed aspect ratio) without leaving the frame, fitting an aspect ratio and rounding to even sizes.

The `internal/fluent` tests check the theme palette (accent color from the Windows accent palette), the anti-aliased shapes and the Windows fonts (loading, measuring and ellipsizing text).

## License

MIT. See [LICENSE](LICENSE). libmpv and FFmpeg are not part of the repository and keep their own licenses (LGPL/GPL depending on the build).
