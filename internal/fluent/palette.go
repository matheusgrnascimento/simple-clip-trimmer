// Package fluent provides the Windows 11 look of the app: the theme
// palette, an anti-aliased painter for custom-drawn controls, the system
// fonts, native popup menus and the window frame (DWM) settings.
package fluent

import (
	"image/color"

	"golang.org/x/sys/windows/registry"
)

// Palette holds the colors of one theme, following the Windows 11 (Fluent)
// light and dark color sets.
type Palette struct {
	Dark bool

	Window  color.NRGBA // background of the control bar (Mica fallback color)
	Footer  color.NRGBA // footer layer
	Divider color.NRGBA // hairline between layers
	Stage   color.NRGBA // video area while no video is open

	Text          color.NRGBA
	TextSecondary color.NRGBA
	TextDisabled  color.NRGBA

	Control        color.NRGBA // rest fill of buttons and drop-downs
	ControlHover   color.NRGBA
	ControlPressed color.NRGBA
	ControlBorder  color.NRGBA
	SubtleHover    color.NRGBA // translucent hover of icon (ghost) buttons and menu items
	SubtlePressed  color.NRGBA // translucent pressed state of the same
	SubtleSelected color.NRGBA // background of the current choice in a menu
	Rail           color.NRGBA // timeline rail

	Flyout        color.NRGBA // menu (flyout) background
	FlyoutDivider color.NRGBA // separator line inside a menu

	Accent        color.NRGBA
	AccentHover   color.NRGBA
	AccentPressed color.NRGBA
	AccentBand    color.NRGBA // translucent accent of the trim selection
	OnAccent      color.NRGBA // text and icons on Accent

	// Over video, the same in both themes.
	Video     color.NRGBA // letterbox around the picture
	Scrim     color.NRGBA // dims the part of the picture a crop removes
	CropLine  color.NRGBA // crop box border and handles
	CropGuide color.NRGBA // rule-of-thirds guides inside the crop box
}

// Colors drawn over video, independent of the theme.
var (
	videoBlack = color.NRGBA{0x00, 0x00, 0x00, 0xFF}
	scrim      = color.NRGBA{0x00, 0x00, 0x00, 0x99}
	cropLine   = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	cropGuide  = color.NRGBA{0xFF, 0xFF, 0xFF, 0x59}
)

const (
	personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	accentKey      = `Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`
)

// Default accent colors of Windows 11 (the blue accent).
var (
	defaultAccentLight = color.NRGBA{0x00, 0x5F, 0xB8, 0xFF} // SystemAccentColorDark1
	defaultAccentDark  = color.NRGBA{0x60, 0xCD, 0xFF, 0xFF} // SystemAccentColorLight2
)

// LoadPalette reads the app theme (light or dark) and the accent color of
// the current user. Missing settings fall back to the Windows defaults.
func LoadPalette() Palette {
	return NewPalette(SystemDark(), SystemAccentPalette())
}

// SystemDark reports whether the user chose the dark app theme.
func SystemDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}

// SystemAccentPalette returns the user's "AccentPalette" registry value, or
// nil if it is not set.
func SystemAccentPalette() []byte {
	k, err := registry.OpenKey(registry.CURRENT_USER, accentKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue("AccentPalette")
	if err != nil {
		return nil
	}
	return b
}

// NewPalette builds the palette for a theme. accentPalette is the
// "AccentPalette" registry value: 8 colors of 4 bytes (R, G, B, A), from
// the lightest shade (index 0, Light3) to the darkest (index 6, Dark3).
func NewPalette(dark bool, accentPalette []byte) Palette {
	accent := defaultAccentLight
	index := 4 // Dark1: accent fills in the light theme
	if dark {
		accent = defaultAccentDark
		index = 1 // Light2: accent fills in the dark theme
	}
	if len(accentPalette) >= 32 {
		b := accentPalette[index*4:]
		accent = color.NRGBA{b[0], b[1], b[2], 0xFF}
	}

	if dark {
		return Palette{
			Dark:           true,
			Window:         rgb(0x20, 0x20, 0x20),
			Footer:         rgb(0x2A, 0x2A, 0x2A),
			Divider:        rgb(0x1A, 0x1A, 0x1A),
			Stage:          rgb(0x1C, 0x1C, 0x1C),
			Text:           rgb(0xFF, 0xFF, 0xFF),
			TextSecondary:  rgb(0xC5, 0xC5, 0xC5),
			TextDisabled:   rgb(0x78, 0x78, 0x78),
			Control:        rgb(0x37, 0x37, 0x37),
			ControlHover:   rgb(0x3C, 0x3C, 0x3C),
			ControlPressed: rgb(0x32, 0x32, 0x32),
			ControlBorder:  rgb(0x40, 0x40, 0x40),
			SubtleHover:    color.NRGBA{0xFF, 0xFF, 0xFF, 0x0F},
			SubtlePressed:  color.NRGBA{0xFF, 0xFF, 0xFF, 0x0A},
			SubtleSelected: color.NRGBA{0xFF, 0xFF, 0xFF, 0x0B},
			Rail:           rgb(0x9A, 0x9A, 0x9A),
			Flyout:         rgb(0x2C, 0x2C, 0x2C),
			FlyoutDivider:  rgb(0x3D, 0x3D, 0x3D),
			Accent:         accent,
			AccentHover:    mix(accent, rgb(0x20, 0x20, 0x20), 0.10),
			AccentPressed:  mix(accent, rgb(0x20, 0x20, 0x20), 0.20),
			AccentBand:     color.NRGBA{accent.R, accent.G, accent.B, 0x40},
			OnAccent:       rgb(0x00, 0x00, 0x00),
			Video:          videoBlack,
			Scrim:          scrim,
			CropLine:       cropLine,
			CropGuide:      cropGuide,
		}
	}
	return Palette{
		Window:         rgb(0xF3, 0xF3, 0xF3),
		Footer:         rgb(0xFB, 0xFB, 0xFB),
		Divider:        rgb(0xE5, 0xE5, 0xE5),
		Stage:          rgb(0xE8, 0xE8, 0xE8),
		Text:           rgb(0x1B, 0x1B, 0x1B),
		TextSecondary:  rgb(0x5F, 0x5F, 0x5F),
		TextDisabled:   rgb(0xA0, 0xA0, 0xA0),
		Control:        rgb(0xFF, 0xFF, 0xFF),
		ControlHover:   rgb(0xF6, 0xF6, 0xF6),
		ControlPressed: rgb(0xF0, 0xF0, 0xF0),
		ControlBorder:  rgb(0xDC, 0xDC, 0xDC),
		SubtleHover:    color.NRGBA{0x00, 0x00, 0x00, 0x0A},
		SubtlePressed:  color.NRGBA{0x00, 0x00, 0x00, 0x06},
		SubtleSelected: color.NRGBA{0x00, 0x00, 0x00, 0x07},
		Rail:           rgb(0x8A, 0x8A, 0x8A),
		Flyout:         rgb(0xF9, 0xF9, 0xF9),
		FlyoutDivider:  rgb(0xE5, 0xE5, 0xE5),
		Accent:         accent,
		AccentHover:    mix(accent, rgb(0xF3, 0xF3, 0xF3), 0.10),
		AccentPressed:  mix(accent, rgb(0xF3, 0xF3, 0xF3), 0.20),
		AccentBand:     color.NRGBA{accent.R, accent.G, accent.B, 0x30},
		OnAccent:       rgb(0xFF, 0xFF, 0xFF),
		Video:          videoBlack,
		Scrim:          scrim,
		CropLine:       cropLine,
		CropGuide:      cropGuide,
	}
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 0xFF} }

// mix blends a toward b by t (0 = a, 1 = b).
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xFF}
}

// COLORREF converts a color to the Win32 0x00BBGGRR format.
func COLORREF(c color.NRGBA) uint32 {
	return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16
}
