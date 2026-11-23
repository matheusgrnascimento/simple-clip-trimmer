// Package mpv is a minimal cgo binding for the libmpv client API.
//
// It covers only what the app needs: embedding the player in a window
// (the "wid" option), running commands, reading and writing properties and
// observing property changes (time-pos, duration, pause...).
package mpv

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/libmpv/include
#cgo LDFLAGS: -L${SRCDIR}/../../third_party/libmpv -lmpv
#include <stdlib.h>
#include <mpv/client.h>

static const char** sct_argv_new(int n) { return (const char**)calloc((size_t)n + 1, sizeof(char*)); }
static void sct_argv_set(const char** a, int i, char* s) { a[i] = s; }
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Format mirrors mpv_format for the property types used by the app.
type Format int

// Property formats.
const (
	FormatNone   Format = C.MPV_FORMAT_NONE
	FormatString Format = C.MPV_FORMAT_STRING
	FormatFlag   Format = C.MPV_FORMAT_FLAG
	FormatInt64  Format = C.MPV_FORMAT_INT64
	FormatDouble Format = C.MPV_FORMAT_DOUBLE
)

// EventID mirrors mpv_event_id.
type EventID int

// Event IDs.
const (
	EventNone            EventID = C.MPV_EVENT_NONE
	EventShutdown        EventID = C.MPV_EVENT_SHUTDOWN
	EventStartFile       EventID = C.MPV_EVENT_START_FILE
	EventEndFile         EventID = C.MPV_EVENT_END_FILE
	EventFileLoaded      EventID = C.MPV_EVENT_FILE_LOADED
	EventPlaybackRestart EventID = C.MPV_EVENT_PLAYBACK_RESTART
	EventPropertyChange  EventID = C.MPV_EVENT_PROPERTY_CHANGE
)

// Event is a Go copy of mpv_event, with the property value already decoded
// when the event is EventPropertyChange.
type Event struct {
	ID     EventID
	Error  error
	Name   string  // property name (EventPropertyChange)
	Format Format  // FormatNone when the property became unavailable
	Double float64 // value when Format == FormatDouble
	Flag   bool    // value when Format == FormatFlag
	Int    int64   // value when Format == FormatInt64
	Str    string  // value when Format == FormatString
}

// Player is a libmpv handle. Its methods are thread safe (the libmpv client
// API is), except WaitEvent, which must be called from a single goroutine.
type Player struct {
	h *C.mpv_handle
}

func mpvErr(code C.int) error {
	if code >= 0 {
		return nil
	}
	return errors.New(C.GoString(C.mpv_error_string(code)))
}

// New creates a handle that is not initialized yet. Set options with
// SetOption and then call Initialize.
func New() (*Player, error) {
	h := C.mpv_create()
	if h == nil {
		return nil, errors.New("mpv_create failed")
	}
	return &Player{h: h}, nil
}

// SetOption sets an option, before or after Initialize.
func (p *Player) SetOption(name, value string) error {
	cn, cv := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(cv))
	if err := mpvErr(C.mpv_set_option_string(p.h, cn, cv)); err != nil {
		return fmt.Errorf("option %s=%s: %w", name, value, err)
	}
	return nil
}

// Initialize starts the player.
func (p *Player) Initialize() error {
	return mpvErr(C.mpv_initialize(p.h))
}

// withArgv calls fn with a NULL-terminated C argv built from args.
func withArgv(args []string, fn func(**C.char) C.int) C.int {
	argv := C.sct_argv_new(C.int(len(args)))
	defer C.free(unsafe.Pointer(argv))
	for i, a := range args {
		cs := C.CString(a)
		defer C.free(unsafe.Pointer(cs))
		C.sct_argv_set(argv, C.int(i), cs)
	}
	return fn((**C.char)(unsafe.Pointer(argv)))
}

// Command runs a command synchronously, for example Command("loadfile", path).
func (p *Player) Command(args ...string) error {
	code := withArgv(args, func(argv **C.char) C.int {
		return C.mpv_command(p.h, (**C.char)(unsafe.Pointer(argv)))
	})
	if err := mpvErr(code); err != nil {
		return fmt.Errorf("command %v: %w", args, err)
	}
	return nil
}

// CommandAsync runs a command without blocking (useful for seeking while
// the user drags the position slider).
func (p *Player) CommandAsync(args ...string) error {
	code := withArgv(args, func(argv **C.char) C.int {
		return C.mpv_command_async(p.h, 0, (**C.char)(unsafe.Pointer(argv)))
	})
	return mpvErr(code)
}

// SetProperty sets a property from a string, for example SetProperty("pause", "yes").
func (p *Player) SetProperty(name, value string) error {
	cn, cv := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(cv))
	return mpvErr(C.mpv_set_property_string(p.h, cn, cv))
}

// GetDouble reads a numeric property.
func (p *Player) GetDouble(name string) (float64, error) {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	var v C.double
	if err := mpvErr(C.mpv_get_property(p.h, cn, C.MPV_FORMAT_DOUBLE, unsafe.Pointer(&v))); err != nil {
		return 0, err
	}
	return float64(v), nil
}

// Observe makes the player send EventPropertyChange when the property changes.
func (p *Player) Observe(id uint64, name string, format Format) error {
	cn := C.CString(name)
	defer C.free(unsafe.Pointer(cn))
	return mpvErr(C.mpv_observe_property(p.h, C.uint64_t(id), cn, C.mpv_format(format)))
}

// WaitEvent blocks until an event arrives or the timeout (seconds) expires.
// A negative timeout waits forever.
func (p *Player) WaitEvent(timeout float64) Event {
	e := C.mpv_wait_event(p.h, C.double(timeout))
	ev := Event{ID: EventID(e.event_id), Error: mpvErr(e.error)}
	if ev.ID != EventPropertyChange || e.data == nil {
		return ev
	}
	prop := (*C.mpv_event_property)(e.data)
	ev.Name = C.GoString(prop.name)
	ev.Format = Format(prop.format)
	if prop.data == nil {
		return ev
	}
	switch ev.Format {
	case FormatDouble:
		ev.Double = float64(*(*C.double)(prop.data))
	case FormatFlag:
		ev.Flag = *(*C.int)(prop.data) != 0
	case FormatInt64:
		ev.Int = int64(*(*C.int64_t)(prop.data))
	case FormatString:
		ev.Str = C.GoString(*(**C.char)(prop.data))
	}
	return ev
}

// Wakeup makes a blocked WaitEvent return EventNone immediately.
func (p *Player) Wakeup() {
	C.mpv_wakeup(p.h)
}

// Destroy shuts the player down. Do not call it while another goroutine is
// inside WaitEvent.
func (p *Player) Destroy() {
	if p.h != nil {
		C.mpv_terminate_destroy(p.h)
		p.h = nil
	}
}
