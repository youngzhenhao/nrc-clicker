package interception

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// Core is the wrapper around an open interception context. It implements
// both MouseSender and KeyboardSender.
//
// Lifecycle:
//
//	c, err := New()
//	defer c.Destroy()
//	if err != nil { /* driver not installed, show installer UI */ }
//	c.MoveAbs(x, y); c.Down(); c.Up(); c.SendKey(...)
//
// All Send calls are safe to issue from any goroutine. The driver's
// internal send lock serialises the underlying interception_send.
type Core struct {
	d       *dll
	ctx     uintptr // opaque context returned by interception_create_context
	hasCtx  bool
	mouse   int    // device ID for mouse
	keybd   int    // device ID for keyboard (first found)
	screenW int    // SM_CXSCREEN
	screenH int    // SM_CYSCREEN

	mu          sync.Mutex // guards mouse/keybd enum and ctx usage
	initMessage string     // populated when isReady returns false
}

// New loads interception.dll, creates a context, and enumerates devices.
// If the driver is not installed it returns an error whose message is
// suitable for surfacing in a "please install driver" dialog.
func New() (*Core, error) {
	d, err := loadDLL()
	if err != nil {
		return nil, err
	}
	setCurrentDLL(d)

	c := &Core{d: d}
	c.screenW, c.screenH = readScreenSize()

	// Ensure predicates exist before any set_filter call. We don't actually
	// call set_filter in phase 1 (recording-only path), but creating them
	// eagerly keeps the DLL contract honest.
	if _, _, perr := ensurePredicates(); perr != nil {
		c.d.release()
		setCurrentDLL(nil)
		return nil, fmt.Errorf("create predicates: %w", perr)
	}

	r, _, _ := d.createCtx.Call()
	if r == 0 {
		c.d.release()
		setCurrentDLL(nil)
		c.initMessage = "interception_create_context returned NULL — driver not installed or computer not rebooted after install"
		return c, fmt.Errorf("%s", c.initMessage)
	}
	c.ctx = r
	c.hasCtx = true

	if err := c.ensureMouseDevice(); err != nil {
		// Non-fatal: fall back to DefaultMouseID so clicks still work on
		// common hardware. Surface a warning via LastError().
		c.mouse = DefaultMouseID
	}
	if err := c.ensureKeyboardDevice(); err != nil {
		// Same: non-fatal for phase 1.
		c.keybd = 0
	}
	return c, nil
}

// LastError returns a human-readable description of the most recent init
// failure (empty when everything is fine).
func (c *Core) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initMessage != "" {
		return c.initMessage
	}
	if c.d != nil && c.d.initError != "" {
		return c.d.initError
	}
	return ""
}

// IsReady reports whether MoveAbs/Down/Up/SendKey can be used.
func (c *Core) IsReady() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasCtx
}

// Destroy frees the context. Safe to call multiple times.
func (c *Core) Destroy() {
	c.mu.Lock()
	if c.hasCtx && c.d != nil && c.d.destroyCtx != nil {
		_, _, _ = c.d.destroyCtx.Call(c.ctx)
		c.hasCtx = false
	}
	c.mu.Unlock()
	if c.d != nil {
		c.d.release()
		setCurrentDLL(nil)
	}
}

// ScreenSize returns the cached screen dimensions in pixels.
func (c *Core) ScreenSize() (int, int) { return c.screenW, c.screenH }

// --- device enumeration --------------------------------------------------

func (c *Core) ensureMouseDevice() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.d == nil || c.d.isMouse == nil {
		return ErrNotReady
	}
	for dev := MouseOffset + 1; dev <= MouseOffset+10; dev++ {
		r, _, _ := c.d.isMouse.Call(uintptr(dev))
		if r != 0 {
			c.mouse = dev
			return nil
		}
	}
	return ErrNoMouseDevice
}

func (c *Core) ensureKeyboardDevice() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.d == nil || c.d.isKeyboard == nil {
		return ErrNotReady
	}
	for dev := KeyboardOffset + 1; dev <= KeyboardOffset+10; dev++ {
		r, _, _ := c.d.isKeyboard.Call(uintptr(dev))
		if r != 0 {
			c.keybd = dev
			return nil
		}
	}
	return ErrNoKeyboardDevice
}

// --- MouseSender ----------------------------------------------------------

// MoveAbs moves the cursor to absolute screen coordinates (pixels).
func (c *Core) MoveAbs(x, y int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCtx || c.d == nil {
		return ErrNotReady
	}
	ix, iy := c.toAbsolute(x, y)
	stroke := MouseStroke{
		State: 0,
		Flags: MouseFlagMoveAbsolute,
		X:     int32(ix),
		Y:     int32(iy),
	}
	return c.sendMouse(stroke)
}

// Down issues a left mouse button press at the current cursor position.
func (c *Core) Down() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCtx || c.d == nil {
		return ErrNotReady
	}
	return c.sendMouse(MouseStroke{State: MouseStateLeftDown})
}

// Up issues a left mouse button release at the current cursor position.
func (c *Core) Up() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCtx || c.d == nil {
		return ErrNotReady
	}
	return c.sendMouse(MouseStroke{State: MouseStateLeftUp})
}

func (c *Core) sendMouse(s MouseStroke) error {
	if c.d.send == nil {
		return ErrNotReady
	}
	arr := [1]MouseStroke{s}
	device := uintptr(c.mouse)
	if device == 0 {
		device = uintptr(DefaultMouseID)
	}
	r, _, _ := c.d.send.Call(
		c.ctx,
		device,
		strokePointer(arr[:]),
		uintptr(1),
	)
	if r == 0 {
		return fmt.Errorf("interception_send mouse returned 0")
	}
	return nil
}

// toAbsolute scales screen pixel coordinates to the 0..65535 absolute range.
func (c *Core) toAbsolute(x, y int) (int, int) {
	w := c.screenW - 1
	h := c.screenH - 1
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if x < 0 {
		x = 0
	}
	if x > w {
		x = w
	}
	if y < 0 {
		y = 0
	}
	if y > h {
		y = h
	}
	return x * 65535 / w, y * 65535 / h
}

// --- KeyboardSender -------------------------------------------------------

// Send injects a single AT Set 2 scancode event.
//
// state must be KeyStateDown or KeyStateUp; extended toggles the KEY_E0 bit
// (used by arrow keys, numpad enter, etc).
func (c *Core) Send(scancode uint16, state uint16, extended bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasCtx || c.d == nil {
		return ErrNotReady
	}
	if c.keybd == 0 {
		// Lazy enumeration — covers the case where the keyboard was plugged
		// in after the program started.
		if err := c.ensureKeyboardDeviceLocked(); err != nil {
			return err
		}
	}
	st := state
	if extended {
		st |= KeyStateE0
	}
	stroke := KeyStroke{Code: scancode, State: st}
	if c.d.send == nil {
		return ErrNotReady
	}
	arr := [1]KeyStroke{stroke}
	r, _, _ := c.d.send.Call(
		c.ctx,
		uintptr(c.keybd),
		keyPointer(arr[:]),
		uintptr(1),
	)
	if r == 0 {
		return fmt.Errorf("interception_send keyboard returned 0")
	}
	return nil
}

// ensureKeyboardDeviceLocked is the inner form (caller already holds mu).
func (c *Core) ensureKeyboardDeviceLocked() error {
	if c.d == nil || c.d.isKeyboard == nil {
		return ErrNotReady
	}
	for dev := KeyboardOffset + 1; dev <= KeyboardOffset+10; dev++ {
		r, _, _ := c.d.isKeyboard.Call(uintptr(dev))
		if r != 0 {
			c.keybd = dev
			return nil
		}
	}
	return ErrNoKeyboardDevice
}

// --- platform helpers -----------------------------------------------------

// readScreenSize queries user32 for SM_CXSCREEN / SM_CYSCREEN. We use the
// user32 handle resolved via windows.NewLazySystemDLL so we don't pay a
// per-call lookup.
var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procGetSystemMetric = user32.NewProc("GetSystemMetrics")
)

func readScreenSize() (int, int) {
	w, _, _ := procGetSystemMetric.Call(0) // SM_CXSCREEN = 0
	h, _, _ := procGetSystemMetric.Call(1) // SM_CYSCREEN = 1
	if w == 0 {
		w = 1920
	}
	if h == 0 {
		h = 1080
	}
	return int(w), int(h)
}
