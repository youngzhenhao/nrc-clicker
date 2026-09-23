// Package interception wraps the Interception kernel-level input driver
// (https://github.com/oblitum/Interception) for use from Go.
//
// We dynamically load interception.dll via golang.org/x/sys/windows — no cgo.
// The package exposes two small interfaces (MouseSender, KeyboardSender) so
// that the clicker loop and the action executor can be unit-tested with a
// in-memory stub, without depending on the real driver.
//
// Lifecycle: callers construct a *Core with New(), drive it via the
// MouseSender / KeyboardSender methods, then call Destroy() once nothing
// else may issue sends.
package interception

import "errors"

// MouseSender is the surface the clicker and the click-action rely on.
//
// Coordinates are screen pixels; the implementation scales to the
// 0..65535 absolute range that interception_send expects.
type MouseSender interface {
	MoveAbs(x, y int) error
	Down() error
	Up() error
}

// KeyboardSender injects a single keystroke via the AT Set 2 scancode path
// (the path the driver uses internally). The scancode table lives in the
// internal/actions package; this surface stays generic on purpose so the
// interception package has no dependency on actions.
type KeyboardSender interface {
	// Send issues a single scancode event. state is INTERCEPTION_KEY_DOWN (0)
	// or INTERCEPTION_KEY_UP (1); extended toggles the KEY_E0 bit.
	Send(scancode uint16, state uint16, extended bool) error
}

// ErrNotReady is returned by Sender methods when the driver is not loaded
// or the device has not been enumerated yet.
var ErrNotReady = errors.New("interception: driver not ready")

// ErrNoMouseDevice is returned when EnsureMouseDevice fails to find any
// mouse device on the bus.
var ErrNoMouseDevice = errors.New("interception: no mouse device found")

// ErrNoKeyboardDevice is returned when SendKey cannot find a keyboard.
var ErrNoKeyboardDevice = errors.New("interception: no keyboard device found")
