package fluent

import (
	"math"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tailscale/win"
)

// MenuItem is an entry of a flyout menu.
type MenuItem struct {
	ID        int    // returned when chosen; 0 for headers and separators
	Text      string // label
	Hint      string // secondary text on the right: a description or a shortcut
	Checked   bool   // the current choice of a group (accent pill), or a toggle that is on
	Toggle    bool   // an on/off item: Checked shows a check mark instead of the pill
	Disabled  bool
	Header    bool // a small section title
	Separator bool
}

func (it MenuItem) selectable() bool { return !it.Header && !it.Separator && !it.Disabled }

// Flyout layout in DIPs (Windows 11 MenuFlyout and ComboBox metrics).
const (
	flyPad      = 4  // around the items
	flyItemH    = 36 // item row
	flyHeaderH  = 30 // section title row
	flySepH     = 9  // separator row (1 px line)
	flyTextX    = 12 // text inset inside an item
	flyCheckW   = 28 // check mark column, when any item is a toggle
	flyHintGap  = 32 // between the text and the hint
	flyMinWidth = 180
	flyGap      = 4 // between the anchor and the flyout
)

// flyout is the state of the open flyout. Only one is open at a time, on
// the UI thread.
type flyout struct {
	hwnd    win.HWND
	items   []MenuItem
	rows    []Rect // item rectangles in client pixels
	pal     Palette
	fonts   *Fonts
	scale   float64
	painter *Painter
	hover   int // index of the highlighted item, -1 for none
	pressed bool
	result  int
	done    bool
	checkW  float64 // check mark column width, 0 if none
}

var (
	flyMu        sync.Mutex
	current      *flyout
	flyClassOnce sync.Once
	flyClassErr  error
	flyClassName = syscall.StringToUTF16Ptr("SimpleClipTrimmerFlyout")
)

// ShowMenu opens a Windows 11 style flyout menu next to anchor (screen
// pixels) and returns the chosen item ID, or 0 when dismissed. Like a native
// menu it runs its own message loop until it closes: it closes on a choice,
// a click outside, Esc, or losing the mouse capture. Up/Down, Enter and
// Space work from the keyboard.
func ShowMenu(owner uintptr, anchor Rect, items []MenuItem, pal Palette, fonts *Fonts) int {
	flyClassOnce.Do(registerFlyoutClass)
	if flyClassErr != nil || len(items) == 0 {
		return 0
	}

	f := &flyout{items: items, pal: pal, fonts: fonts, hover: -1}
	f.scale = float64(win.GetDpiForWindow(win.HWND(owner))) / 96
	if f.scale <= 0 {
		f.scale = 1
	}
	w, h := f.layout()
	x, y := f.place(owner, anchor, w, h)

	const exStyle = win.WS_EX_TOOLWINDOW | win.WS_EX_TOPMOST | win.WS_EX_NOACTIVATE
	f.hwnd = win.CreateWindowEx(exStyle, flyClassName, nil, win.WS_POPUP,
		int32(x), int32(y), int32(w), int32(h), win.HWND(owner), 0, win.GetModuleHandle(nil), nil)
	if f.hwnd == 0 {
		return 0
	}
	flyMu.Lock()
	current = f
	flyMu.Unlock()
	defer func() {
		flyMu.Lock()
		current = nil
		flyMu.Unlock()
		win.DestroyWindow(f.hwnd)
	}()

	// Rounded corners, and the frame shadow DWM draws for windows with a
	// frame extended into the client area.
	corner := int32(dwmwcpRound)
	procDwmSetWindowAttribute.Call(uintptr(f.hwnd), dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
	margins := win.MARGINS{LeftWidth: 1, RightWidth: 1, TopHeight: 1, BottomHeight: 1}
	win.DwmExtendFrameIntoClientArea(f.hwnd, &margins)

	win.ShowWindow(f.hwnd, win.SW_SHOWNOACTIVATE)
	win.UpdateWindow(f.hwnd)
	win.SetCapture(f.hwnd)

	var msg win.MSG
	for !f.done {
		switch win.GetMessage(&msg, 0, 0, 0) {
		case 0: // WM_QUIT: close and let the app's loop see it
			win.PostQuitMessage(int32(msg.WParam))
			return 0
		case -1:
			return 0
		}
		switch msg.Message {
		case win.WM_KEYDOWN, win.WM_SYSKEYDOWN:
			f.key(msg.WParam)
			continue
		case win.WM_CHAR, win.WM_SYSCHAR, win.WM_KEYUP, win.WM_SYSKEYUP:
			continue // keys belong to the menu while it is open
		}
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
	if capture, _, _ := procGetCapture.Call(); win.HWND(capture) == f.hwnd {
		win.ReleaseCapture()
	}
	return f.result
}

var procGetCapture = user32.NewProc("GetCapture")

const (
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
	maNoActivate                = 3
)

func registerFlyoutClass() {
	wc := win.WNDCLASSEX{
		LpfnWndProc:   syscall.NewCallback(flyoutProc),
		HInstance:     win.GetModuleHandle(nil),
		HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
		LpszClassName: flyClassName,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	if win.RegisterClassEx(&wc) == 0 {
		flyClassErr = syscall.GetLastError()
	}
}

func flyoutProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	flyMu.Lock()
	f := current
	flyMu.Unlock()
	if f == nil || f.hwnd != hwnd {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	x := float64(int16(win.LOWORD(uint32(lParam))))
	y := float64(int16(win.HIWORD(uint32(lParam))))
	switch msg {
	case win.WM_PAINT:
		var ps win.PAINTSTRUCT
		hdc := win.BeginPaint(hwnd, &ps)
		f.paint(uintptr(hdc))
		win.EndPaint(hwnd, &ps)
		return 0
	case win.WM_MOUSEACTIVATE:
		return maNoActivate
	case win.WM_MOUSEMOVE:
		f.setHover(f.itemAt(x, y))
		return 0
	case win.WM_LBUTTONDOWN, win.WM_RBUTTONDOWN:
		if f.itemAt(x, y) < 0 && !f.inside(x, y) {
			f.done = true // a click outside closes the menu and is swallowed
		}
		f.pressed = true
		return 0
	case win.WM_LBUTTONUP, win.WM_RBUTTONUP:
		if i := f.itemAt(x, y); i >= 0 && f.pressed {
			f.choose(i)
		}
		f.pressed = false
		return 0
	case win.WM_CAPTURECHANGED:
		f.done = true
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (f *flyout) s(dip float64) float64 { return math.Round(dip * f.scale) }

// layout computes the item rows and returns the window size in pixels.
func (f *flyout) layout() (int, int) {
	text := f.fonts.Text(f.s(14))
	caption := f.fonts.Text(f.s(12))
	for _, it := range f.items {
		if it.Toggle {
			f.checkW = f.s(flyCheckW)
		}
	}
	contentW := f.s(flyMinWidth) - 2*f.s(flyPad)
	for _, it := range f.items {
		w := f.s(flyTextX)*2 + f.checkW
		switch {
		case it.Header:
			w += Measure(caption, it.Text)
		case it.Separator:
		default:
			w += Measure(text, it.Text)
			if it.Hint != "" {
				w += f.s(flyHintGap) + Measure(caption, it.Hint)
			}
		}
		contentW = math.Max(contentW, w)
	}
	y := f.s(flyPad)
	f.rows = make([]Rect, len(f.items))
	for i, it := range f.items {
		h := f.s(flyItemH)
		switch {
		case it.Header:
			h = f.s(flyHeaderH)
		case it.Separator:
			h = f.s(flySepH)
		}
		f.rows[i] = Rect{f.s(flyPad), y, contentW, h}
		y += h
	}
	return int(math.Ceil(contentW + 2*f.s(flyPad))), int(math.Ceil(y + f.s(flyPad)))
}

// place puts the flyout above the anchor (the controls sit at the bottom of
// the window), or below it when there is no room, inside the monitor's
// work area.
func (f *flyout) place(owner uintptr, anchor Rect, w, h int) (int, int) {
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	win.GetMonitorInfo(win.MonitorFromWindow(win.HWND(owner), win.MONITOR_DEFAULTTONEAREST), &mi)
	work := mi.RcWork
	gap := f.s(flyGap)

	y := anchor.Y - gap - float64(h)
	if y < float64(work.Top) {
		y = anchor.Bottom() + gap
	}
	x := anchor.X
	x = math.Min(x, float64(work.Right)-float64(w))
	x = math.Max(x, float64(work.Left))
	y = math.Min(y, float64(work.Bottom)-float64(h))
	y = math.Max(y, float64(work.Top))
	return int(x), int(y)
}

func (f *flyout) inside(x, y float64) bool {
	var r win.RECT
	win.GetClientRect(f.hwnd, &r)
	return x >= 0 && y >= 0 && x < float64(r.Right) && y < float64(r.Bottom)
}

func (f *flyout) itemAt(x, y float64) int {
	for i, r := range f.rows {
		if f.items[i].selectable() && r.Contains(x, y) {
			return i
		}
	}
	return -1
}

func (f *flyout) setHover(i int) {
	if i != f.hover {
		f.hover = i
		win.InvalidateRect(f.hwnd, nil, false)
	}
}

func (f *flyout) choose(i int) {
	f.result = f.items[i].ID
	f.done = true
}

// key handles the keyboard while the menu is open.
func (f *flyout) key(vk uintptr) {
	switch vk {
	case win.VK_ESCAPE, win.VK_MENU, win.VK_TAB:
		f.done = true
	case win.VK_RETURN, win.VK_SPACE:
		if f.hover >= 0 {
			f.choose(f.hover)
		}
	case win.VK_DOWN, win.VK_UP:
		step := 1
		if vk == win.VK_UP {
			step = -1
		}
		start := f.hover
		if start < 0 { // start from the current choice, as a combo box does
			for i, it := range f.items {
				if it.Checked && !it.Toggle {
					start = i - step
					break
				}
			}
			if start < 0 && step < 0 {
				start = len(f.items)
			}
		}
		for i := start + step; i >= 0 && i < len(f.items); i += step {
			if f.items[i].selectable() {
				f.setHover(i)
				return
			}
		}
	}
}

func (f *flyout) paint(hdc uintptr) {
	var cr win.RECT
	win.GetClientRect(f.hwnd, &cr)
	p := NewPainter(f.painter, int(cr.Right), int(cr.Bottom), f.scale)
	f.painter = p
	pal := f.pal
	p.Fill(pal.Flyout)

	text := f.fonts.Text(f.s(14))
	caption := f.fonts.Text(f.s(12))
	check := f.fonts.Icons(f.s(12))
	for i, it := range f.items {
		r := f.rows[i]
		cy := r.CenterY()
		switch {
		case it.Separator:
			p.FillRect(Rect{r.X - f.s(flyPad), math.Round(cy), r.W + 2*f.s(flyPad), math.Max(1, f.s(1))}, pal.FlyoutDivider)
			continue
		case it.Header:
			p.Text(caption, it.Text, r.X+f.s(flyTextX), cy+f.s(2), pal.TextSecondary)
			continue
		}

		switch {
		case i == f.hover && f.pressed:
			p.RoundRect(r, f.s(4), pal.SubtlePressed)
		case i == f.hover:
			p.RoundRect(r, f.s(4), pal.SubtleHover)
		case it.Checked && !it.Toggle:
			p.RoundRect(r, f.s(4), pal.SubtleSelected)
		}
		if it.Checked && !it.Toggle {
			// Accent pill of the current choice.
			p.RoundRect(Rect{r.X, cy - f.s(8), f.s(3), f.s(16)}, f.s(1.5), pal.Accent)
		}

		fg, hint := pal.Text, pal.TextSecondary
		if it.Disabled {
			fg, hint = pal.TextDisabled, pal.TextDisabled
		}
		x := r.X + f.s(flyTextX)
		if f.checkW > 0 {
			if it.Toggle && it.Checked {
				p.Text(check, IconCheckMark, x, cy, fg)
			}
			x += f.checkW
		}
		p.Text(text, it.Text, x, cy, fg)
		if it.Hint != "" {
			p.Text(caption, it.Hint, r.Right()-f.s(flyTextX)-Measure(caption, it.Hint), cy, hint)
		}
	}
	p.Blit(hdc)
}
