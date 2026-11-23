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

	procCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	procAppendMenuW           = user32.NewProc("AppendMenuW")
	procTrackPopupMenuEx      = user32.NewProc("TrackPopupMenuEx")
	procDestroyMenu           = user32.NewProc("DestroyMenu")
	procClientToScreen        = user32.NewProc("ClientToScreen")
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

// ApplyFrame makes the title bar follow the palette: dark or light frame
// and a caption color that blends with the window.
func ApplyFrame(hwnd uintptr, p Palette) {
	var dark int32
	if p.Dark {
		dark = 1
	}
	procDwmSetWindowAttribute.Call(hwnd, dwmwaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&dark)), 4)
	caption := COLORREF(p.Window)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaCaptionColor, uintptr(unsafe.Pointer(&caption)), 4)
}

// AllowDarkMenus lets native popup menus follow the system dark mode. It
// uses the undocumented uxtheme SetPreferredAppMode (ordinal 135) and
// FlushMenuThemes (ordinal 136), present since Windows 10 1903; on older
// systems it does nothing.
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

// MenuItem is an entry of a popup menu. ID 0 with Separator set adds a
// separator line.
type MenuItem struct {
	ID        int
	Text      string
	Checked   bool
	Disabled  bool
	Separator bool
}

// Menu flags (winuser.h).
const (
	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfSeparator = 0x0800

	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
	tpmVertical    = 0x0040
)

type point struct{ X, Y int32 }

type tpmParams struct {
	Size    uint32
	Exclude struct{ Left, Top, Right, Bottom int32 }
}

// PopupMenu shows a native menu above the client rectangle anchor of hwnd
// (in client pixels) and returns the chosen item ID, or 0 if dismissed.
func PopupMenu(hwnd uintptr, anchor Rect, items []MenuItem) (int, error) {
	m, _, err := procCreatePopupMenu.Call()
	if m == 0 {
		return 0, fmt.Errorf("CreatePopupMenu: %w", err)
	}
	defer procDestroyMenu.Call(m)

	for _, it := range items {
		if it.Separator {
			procAppendMenuW.Call(m, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if it.Checked {
			flags |= mfChecked
		}
		if it.Disabled {
			flags |= mfGrayed
		}
		text, err := windows.UTF16PtrFromString(it.Text)
		if err != nil {
			return 0, err
		}
		procAppendMenuW.Call(m, flags, uintptr(it.ID), uintptr(unsafe.Pointer(text)))
	}

	tl := point{int32(anchor.X), int32(anchor.Y)}
	br := point{int32(anchor.Right()), int32(anchor.Bottom())}
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&tl)))
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&br)))
	params := tpmParams{Size: uint32(unsafe.Sizeof(tpmParams{}))}
	params.Exclude.Left, params.Exclude.Top = tl.X, tl.Y
	params.Exclude.Right, params.Exclude.Bottom = br.X, br.Y

	// Open upward from the anchor (the controls sit at the bottom of the
	// window); TPMPARAMS keeps the menu from covering the anchor if Windows
	// has to flip it.
	id, _, _ := procTrackPopupMenuEx.Call(m,
		tpmLeftAlign|tpmBottomAlign|tpmVertical|tpmReturnCmd|tpmNoNotify,
		uintptr(tl.X), uintptr(tl.Y), hwnd, uintptr(unsafe.Pointer(&params)))
	return int(id), nil
}

// WM_MOUSELEAVE and friends.
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
// cursor leaves it.
func TrackMouseLeave(hwnd uintptr) {
	t := trackMouseEvent{Flags: tmeLeave, Hwnd: hwnd}
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
