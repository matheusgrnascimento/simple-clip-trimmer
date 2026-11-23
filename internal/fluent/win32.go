package fluent

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32     = windows.NewLazySystemDLL("user32.dll")
	dwmapi     = windows.NewLazySystemDLL("dwmapi.dll")
	comctl32   = windows.NewLazySystemDLL("comctl32.dll")
	uxthemeDLL = windows.NewLazySystemDLL("uxtheme.dll")

	procTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procSetWindowSubclass     = comctl32.NewProc("SetWindowSubclass")
	procDefSubclassProc       = comctl32.NewProc("DefSubclassProc")
)

// DWM window attributes (dwmapi.h).
const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaCaptionColor         = 35
)

// ApplyFrame makes the window frame follow the palette: dark or light
// borders and shadow, and a caption color that blends with the window.
func ApplyFrame(hwnd uintptr, p Palette) {
	var dark int32
	if p.Dark {
		dark = 1
	}
	procDwmSetWindowAttribute.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&dark)), 4)
	caption := COLORREF(p.Window)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaCaptionColor, uintptr(unsafe.Pointer(&caption)), 4)
}

// AllowDarkMenus lets native menus (the window's system menu) follow the
// system dark mode. It uses the undocumented uxtheme SetPreferredAppMode
// (ordinal 135) and FlushMenuThemes (ordinal 136), present since Windows 10
// 1903; on older systems it does nothing.
func AllowDarkMenus() {
	if err := uxthemeDLL.Load(); err != nil {
		return
	}
	h := windows.Handle(uxthemeDLL.Handle())
	setPreferredAppMode, err := windows.GetProcAddressByOrdinal(h, 135)
	if err != nil {
		return
	}
	const allowDark = 1
	syscall.SyscallN(setPreferredAppMode, allowDark)
	if flush, err := windows.GetProcAddressByOrdinal(h, 136); err == nil {
		syscall.SyscallN(flush)
	}
}

// Window messages walk does not name.
const (
	WMMouseLeave    = 0x02A3
	WMSettingChange = 0x001A
	tmeLeave        = 0x0002
)

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Hwnd      uintptr
	HoverTime uint32
}

// TrackMouseLeave asks Windows to send WM_MOUSELEAVE to hwnd when the
// cursor leaves its client area.
func TrackMouseLeave(hwnd uintptr) { trackMouse(hwnd, tmeLeave) }

func trackMouse(hwnd uintptr, flags uint32) {
	t := trackMouseEvent{Flags: flags, Hwnd: hwnd}
	t.Size = uint32(unsafe.Sizeof(t))
	procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
}

// SubclassFunc handles a window message before the window's own procedure.
// It returns true when the message was fully handled.
type SubclassFunc func(msg uint32, wParam, lParam uintptr) (handled bool, result uintptr)

var (
	subclassMu       sync.Mutex
	subclassHandlers = map[uintptr]SubclassFunc{}
	subclassProc     = syscall.NewCallback(func(hwnd, msg, wParam, lParam, id, ref uintptr) uintptr {
		subclassMu.Lock()
		h := subclassHandlers[hwnd]
		subclassMu.Unlock()
		if h != nil {
			if handled, r := h(uint32(msg), wParam, lParam); handled {
				return r
			}
		}
		r, _, _ := procDefSubclassProc.Call(hwnd, msg, wParam, lParam)
		return r
	})
)

// Subclass routes the messages of hwnd through fn (comctl32 window
// subclassing, which coexists with the subclassing walk does).
func Subclass(hwnd uintptr, fn SubclassFunc) error {
	subclassMu.Lock()
	subclassHandlers[hwnd] = fn
	subclassMu.Unlock()
	if r, _, err := procSetWindowSubclass.Call(hwnd, subclassProc, 1, 0); r == 0 {
		return fmt.Errorf("SetWindowSubclass: %w", err)
	}
	return nil
}

// LParamString reads the string a message passes as a pointer in lParam
// (for example WM_SETTINGCHANGE).
func LParamString(lParam uintptr) string {
	if lParam == 0 {
		return ""
	}
	p := *(**uint16)(unsafe.Pointer(&lParam))
	return windows.UTF16PtrToString(p)
}
