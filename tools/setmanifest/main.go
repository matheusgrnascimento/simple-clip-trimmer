// Command setmanifest replaces the manifest (RT_MANIFEST #1) of a built .exe.
//
// Why it exists: with cgo, the MinGW GCC links its own default manifest
// (without Common Controls v6), and ld refuses to merge it with a .syso of
// ours. So the app is built normally and the resource is replaced afterwards
// with BeginUpdateResource/UpdateResource.
//
// Usage: go run ./tools/setmanifest app.exe app.manifest
package main

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procBeginUpdateResource = kernel32.NewProc("BeginUpdateResourceW")
	procUpdateResource      = kernel32.NewProc("UpdateResourceW")
	procEndUpdateResource   = kernel32.NewProc("EndUpdateResourceW")
)

const (
	rtManifest = 24
	manifestID = 1      // CREATEPROCESS_MANIFEST_RESOURCE_ID
	langEnUS   = 0x0409 // same language as the MinGW default manifest, so it is replaced
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: setmanifest <file.exe> <file.manifest>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "setmanifest:", err)
		os.Exit(1)
	}
}

func run(exe, manifest string) error {
	data, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty manifest")
	}
	exeW, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	h, _, err := procBeginUpdateResource.Call(uintptr(unsafe.Pointer(exeW)), 0)
	if h == 0 {
		return fmt.Errorf("BeginUpdateResource: %w", err)
	}
	r, _, err := procUpdateResource.Call(h, rtManifest, manifestID, langEnUS,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	if r == 0 {
		procEndUpdateResource.Call(h, 1) // discard
		return fmt.Errorf("UpdateResource: %w", err)
	}
	if r, _, err := procEndUpdateResource.Call(h, 0); r == 0 {
		return fmt.Errorf("EndUpdateResource: %w", err)
	}
	return nil
}
