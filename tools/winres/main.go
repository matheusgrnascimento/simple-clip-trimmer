// Command winres writes the Windows resources of the built .exe: the
// application manifest (RT_MANIFEST #1) and the app icon (RT_GROUP_ICON #1
// with its RT_ICON images), which Explorer, the Start menu and shortcuts
// show.
//
// Why after the build: with cgo, the MinGW GCC links its own
// "default-manifest" (without Common Controls v6) and ld refuses to merge it
// with a .syso of ours. So the app is built normally and the resources are
// written afterwards with BeginUpdateResource/UpdateResource.
//
// Usage: go run ./tools/winres -manifest app.manifest -icon assets/icon.ico app.exe
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
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
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
	manifestID  = 1      // CREATEPROCESS_MANIFEST_RESOURCE_ID
	iconGroupID = 1      // the first icon group is the one Explorer shows
	langEnUS    = 0x0409 // same language as the MinGW default manifest, so it is replaced
	langNeutral = 0
)

func main() {
	manifest := flag.String("manifest", "", "application manifest to embed")
	icon := flag.String("icon", "", ".ico file to embed as the app icon")
	flag.Parse()
	if flag.NArg() != 1 || (*manifest == "" && *icon == "") {
		fmt.Fprintln(os.Stderr, "usage: winres [-manifest app.manifest] [-icon app.ico] file.exe")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *manifest, *icon); err != nil {
		fmt.Fprintln(os.Stderr, "winres:", err)
		os.Exit(1)
	}
}

// resource is one entry to write.
type resource struct {
	typ, id, lang uintptr
	data          []byte
}

func run(exe, manifestPath, iconPath string) error {
	var res []resource
	if manifestPath != "" {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return err
		}
		res = append(res, resource{rtManifest, manifestID, langEnUS, data})
	}
	if iconPath != "" {
		data, err := os.ReadFile(iconPath)
		if err != nil {
			return err
		}
		icons, err := iconResources(data)
		if err != nil {
			return fmt.Errorf("%s: %w", iconPath, err)
		}
		res = append(res, icons...)
	}
	return update(exe, res)
}

// iconResources splits an .ico file into RT_ICON images and the
// RT_GROUP_ICON directory that refers to them by ID.
func iconResources(ico []byte) ([]resource, error) {
	le := binary.LittleEndian
	if len(ico) < 6 || le.Uint16(ico[2:]) != 1 {
		return nil, errors.New("not an icon file")
	}
	n := int(le.Uint16(ico[4:]))
	if len(ico) < 6+16*n {
		return nil, errors.New("truncated icon directory")
	}
	var group bytes.Buffer
	binary.Write(&group, le, [3]uint16{0, 1, uint16(n)})
	var res []resource
	for i := range n {
		e := ico[6+16*i : 6+16*(i+1)]
		size, offset := le.Uint32(e[8:]), le.Uint32(e[12:])
		if int(offset)+int(size) > len(ico) {
			return nil, fmt.Errorf("image %d is out of the file", i)
		}
		id := uint16(i + 1)
		res = append(res, resource{rtIcon, uintptr(id), langNeutral, ico[offset : offset+size]})
		// GRPICONDIRENTRY: the ICONDIRENTRY fields with a 2-byte ID instead
		// of the 4-byte file offset.
		group.Write(e[:12])
		binary.Write(&group, le, id)
	}
	return append(res, resource{rtGroupIcon, iconGroupID, langNeutral, group.Bytes()}), nil
}

func update(exe string, res []resource) error {
	exeW, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	h, _, err := procBeginUpdateResource.Call(uintptr(unsafe.Pointer(exeW)), 0)
	if h == 0 {
		return fmt.Errorf("BeginUpdateResource: %w", err)
	}
	for _, r := range res {
		ok, _, err := procUpdateResource.Call(h, r.typ, r.id, r.lang,
			uintptr(unsafe.Pointer(&r.data[0])), uintptr(len(r.data)))
		if ok == 0 {
			procEndUpdateResource.Call(h, 1) // discard
			return fmt.Errorf("UpdateResource(type %d, id %d): %w", r.typ, r.id, err)
		}
	}
	if ok, _, err := procEndUpdateResource.Call(h, 0); ok == 0 {
		return fmt.Errorf("EndUpdateResource: %w", err)
	}
	return nil
}
