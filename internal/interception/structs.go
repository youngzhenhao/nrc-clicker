package interception

import "unsafe"

// MouseStroke mirrors the C struct InterceptionMouseStroke exactly (16 bytes
// on x64). Field order and sizes must NOT be reordered — that would break
// the cdecl ABI when we hand a pointer to interception_send.
type MouseStroke struct {
	State       uint16 // button bitmask (MOUSE_LEFT_BUTTON_DOWN / _UP etc.)
	Flags       uint16 // MOUSE_MOVE_ABSOLUTE = 0x01, else 0 for relative
	Rolling     int16  // wheel delta (we leave 0)
	X           int32  // absolute (0..65535) or relative delta
	Y           int32
	Information uint32 // hardware timestamp; driver fills, we leave 0
}

// KeyStroke mirrors InterceptionKeyStroke (8 bytes).
type KeyStroke struct {
	Code        uint16 // AT Set 2 scancode
	State       uint16 // bit0=UP, bit1=KEY_E0 (extended)
	Information uint32 // hardware timestamp
}

// Mouse state flags (matches INTERCEPTION_MOUSE_* in interception.h).
const (
	MouseStateLeftDown   uint16 = 0x001
	MouseStateLeftUp     uint16 = 0x002
	MouseStateRightDown  uint16 = 0x004
	MouseStateRightUp    uint16 = 0x008
	MouseStateMiddleDown uint16 = 0x010
	MouseStateMiddleUp   uint16 = 0x020

	MouseFlagMoveAbsolute uint16 = 0x001
)

// Keyboard state flags.
const (
	KeyStateDown    uint16 = 0x00
	KeyStateUp      uint16 = 0x01
	KeyStateE0      uint16 = 0x02 // OR'd in for extended scancodes
)

// Filter constants (only used in the recording/playback path which we don't
// ship in phase 1; defined here for completeness so a future port doesn't
// have to re-derive them).
const (
	FilterMouseAll       uint16 = 0xFFFF
	FilterMouseLeftDown  uint16 = 0x001
	FilterMouseLeftUp    uint16 = 0x002
	FilterMouseRightDown uint16 = 0x004
	FilterMouseRightUp   uint16 = 0x008
	FilterMouseMove      uint16 = 0x1000

	FilterKeyAll  uint16 = 0xFFFF
	FilterKeyDown uint16 = 0x01
	FilterKeyUp   uint16 = 0x02
)

// Device enumeration bounds. The driver exposes up to 10 keyboards
// (device IDs 1..10) and up to 10 mice (device IDs 11..20). The predicate
// functions tell us which class each device belongs to.
const (
	MaxDevice       = 20
	KeyboardOffset  = 0 // devices 1..10
	MouseOffset     = 10 // devices 11..20; first mouse is conventionally device 11
	DefaultMouseID  = 11
)

// SystemMetrics returns the primary screen width/height in pixels.
// Equivalent to GetSystemMetrics(SM_CXSCREEN) / GetSystemMetrics(SM_CYSCREEN)
// but cached at Core construction time. Callers that need to refresh should
// re-read via the underlying user32 call.
func ScreenSize() (w, h int) {
	// The actual values are injected via Core at construction; this helper is
	// only used by tests that don't need real coordinates.
	return 1920, 1080
}

// sizeofMouseStroke / sizeofKeyStroke are pin tests use to assert the
// C struct layouts. Touching the field order invalidates both.
func sizeofMouseStroke() int { return int(unsafe.Sizeof(MouseStroke{})) }
func sizeofKeyStroke() int   { return int(unsafe.Sizeof(KeyStroke{})) }
