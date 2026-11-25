# AGENTS.md

Guidelines for AI agents and contributors working in this repository.

## Project overview

Simple Clip Trimmer is a Windows desktop app that trims, resizes and compresses videos, such as recordings from the NVIDIA app.

- Language: Go.
- UI: a native Win32 window via `github.com/tailscale/walk` holding two custom-drawn widgets (video area and control bar) in a Windows 11 (Fluent) style. Native dialogs and popup menus.
- Preview: libmpv, embedded in a child window through cgo (`internal/mpv`).
- Encoding: the `ffmpeg` and `ffprobe` command line tools, run with `os/exec`. The app never links FFmpeg.
- Build: cgo with the MinGW-w64 GCC (WinLibs).

### Layout

- `main.go`: app entry point and the shared `app` state.
- `window.go`: window, theme, shortcuts and shared helpers (`refresh`, `flash`, `blit`).
- `titlebar.go`: the drawn title bar: removing the system caption (`WM_NCCALCSIZE`), hit testing (`WM_NCHITTEST`) and the caption buttons.
- `bar.go`: the control bar: painting, layout, hit testing, timeline dragging and clicks.
- `stage.go`: the video area and its empty state.
- `cropmode.go`: crop mode: the still frame from mpv, the crop box overlay and mouse handling, aspect ratios, and the mpv preview crop.
- `actions.go`: the named actions shared by shortcuts, mpv key bindings and the bar.
- `player.go`: mpv setup, key bindings, event loop, playback and trim marks.
- `file.go`: opening and probing files.
- `export.go`: encoder checks, output menus, save dialog and the export flow.
- `internal/fluent/`: Windows 11 palette (theme and accent color), anti-aliased painter, Windows fonts, flyout menus (`ShowMenu`), DWM frame and window subclassing.
- `internal/crop/`: crop box geometry in source pixels (pure, tested).
- `internal/mpv/`: minimal libmpv client API binding.
- `internal/media/`: codecs and encoders, ffprobe parsing, ffmpeg arguments, progress and cancellation.
- `tools/winres/`: writes the Windows resources of the built `.exe`: `app.manifest` in place of the MinGW default manifest, and the app icon.
- `tools/genicon/` and `assets/`: the app icon, generated from code (`go run ./tools/genicon` writes `assets/icon.ico` and `assets/icon.png`); `icon.go` draws it in the title bar and sets the window icon. Change the icon in the generator, never by editing the image files.
- `setup.ps1`: Windows environment setup (tools, libmpv) and build. `build.ps1`: build only, into `build\Simple Clip Trimmer.exe`.
- `VERSION`: the single source of the version (the exe's version resource and the MSI name and version).
- `installer/`: the WiX MSI (`Package.wxs`) and `build-installer.ps1`, which writes it to `dist\`.
- `third_party/libmpv/`: libmpv DLL, import library and headers, downloaded by `setup.ps1` and not committed.

## Language

All content must be written in English: code, identifiers, comments, doc comments, log and error messages, UI text, commit messages, documentation and scripts.

## Build and verify

Requires Go, GCC (MinGW-w64), FFmpeg in `PATH` and libmpv in `third_party/libmpv` (see `README.md` and `setup.ps1`).

```powershell
gofmt -l .
go vet ./...
go test ./...
.\build.ps1
```

`gofmt -l .` must print nothing. Run formatting, vet and tests before every commit. Fix warnings instead of silencing them. `go test -short ./...` skips the tests that run ffmpeg.

## Coding conventions

- Follow idiomatic Go and the existing file and package structure. Keep files and packages small and single-purpose.
- Return errors with context (`fmt.Errorf("...: %w", err)`). No `panic` in non-test code unless the invariant is documented in a comment.
- Never block the UI thread. Run ffprobe, ffmpeg and other slow work on goroutines and come back with `Synchronize`. Only touch widgets and `app` state on the UI thread.
- Keep cgo confined to `internal/mpv`. Free every C string and array you allocate. Other Win32 calls go through `golang.org/x/sys/windows` or `github.com/tailscale/win` (no cgo).
- Draw custom UI with `fluent.Painter`: take colors only from the `Palette` (never hard-coded), give sizes in DIPs through `Painter.S`, and keep both light and dark themes working (`SIMPLE_CLIP_TRIMMER_THEME` forces one to check it).
- Prefer fewer controls: a new option goes into an existing drop-down or the timeline's context menu before it gets its own button. Show choices with `fluent.ShowMenu` (headers, hints, the current choice checked) and messages with native dialogs.
- Keep the drawn title bar native: the window's hit testing must keep returning the caption, the top edge and the caption button codes, so dragging, resizing, the system menu and Snap layouts keep working.
- Every action needs a keyboard path: add it to `actions.go`, so it is bound both in the window and in mpv.
- Start ffmpeg and ffprobe with `hideWindow` so no console window flashes.
- Use `log/slog` for logging, not `fmt.Println` or `log`.
- Add tests for new logic. Keep pure logic (argument building, parsing) testable without ffmpeg; mark tests that run ffmpeg with `testing.Short()` skips.
- Do not add dependencies without a clear need.
- Keep changes scoped. No unrelated refactors in a feature or fix commit.

## Commits

Make atomic commits: each commit contains one logical change, builds, and passes tests on its own.

- Do not mix features, fixes, refactors, formatting and docs in one commit. Split them.
- Commit only the files that belong to the change. Stage deliberately (`git add -p`), not with `git add -A`.
- Use Conventional Commits: `type(scope): summary`.
  - Types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `chore`, `style`.
  - Summary in imperative mood, lowercase, no trailing period, 72 characters or fewer.
  - Add a body when the reason for the change is not obvious.
- Do not commit build output (`build/`, `*.exe`) or downloaded dependencies (`third_party/`).
- Do not rewrite history that has been pushed. Do not force push.

## Media handling

- Never modify or overwrite the input video. Refuse an output path equal to the input.
- Remove partial output files when an export is canceled.
- Keep every audio track of the input unless the user chooses otherwise (the NVIDIA app can record the microphone on its own track).
- Check that an encoder works on the machine (`ProbeEncoder`) before offering it as the default.
