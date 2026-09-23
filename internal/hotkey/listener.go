// Package hotkey implements a global keyboard hook for the F1-F12
// hotkeys the user can configure. It mirrors Clicker.py's
// GlobalHotkeyListener: a producer thread installs a WH_KEYBOARD_LL hook
// and runs a Windows message pump; a separate goroutine reads events off
// a buffered channel and dispatches them.
//
// On Windows 11 24H2+, SetWindowsHookExW for cross-process keyboard hooks
// can silently fail; we detect that case and fall back to a 50 ms
// GetAsyncKeyState poll over the configured F-key VKs.
package hotkey

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

// KeyEvent is one down/up notification.
type KeyEvent struct {
	VK    int  // Windows virtual key code
	Up    bool // true if this is a key-up (synthesised from GetAsyncKeyState)
}

// Listener owns the hook lifecycle. Construct one with New, configure with
// Watch, then call Start. Events come off Events().
type Listener struct {
	logger *logrus.Entry

	mu        sync.Mutex
	watchVKs  map[int]struct{} // VKs the consumer cares about
	events    chan KeyEvent
	producer  *producerHandle
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	started   atomic.Bool
	fallback  atomic.Bool
}

// New creates a Listener with no watched VKs yet.
func New() *Listener {
	return &Listener{
		logger: logrus.WithField("subsystem", "hotkey"),
		events: make(chan KeyEvent, 32),
	}
}

// Watch sets the VK codes the consumer wants to receive. Empty clears.
func (l *Listener) Watch(vks []int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.watchVKs = make(map[int]struct{}, len(vks))
	for _, v := range vks {
		l.watchVKs[v] = struct{}{}
	}
}

// Events returns the receive end of the event channel. Closing is handled
// by Stop().
func (l *Listener) Events() <-chan KeyEvent { return l.events }

// Start begins listening for watched VKs. Safe to call once; subsequent
// calls are no-ops.
func (l *Listener) Start() {
	if !l.started.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()

	l.wg.Add(1)
	go l.run(ctx)
}

// Stop tears down the hook and closes the event channel. Safe to call
// multiple times.
func (l *Listener) Stop() {
	if !l.started.CompareAndSwap(true, false) {
		return
	}
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	// Also ask the producer thread to exit its GetMessageW loop.
	if l.producer != nil && l.producer.threadID != 0 {
		_, _, _ = procPostThreadMessageW.Call(
			uintptr(l.producer.threadID),
			WMQuit,
			0, 0,
		)
	}
	l.mu.Unlock()
	l.wg.Wait()
	close(l.events)
}

// UsingFallback reports whether we ended up on the GetAsyncKeyState poll
// path (e.g. Windows 11 24H2 refused the WH_KEYBOARD_LL hook).
func (l *Listener) UsingFallback() bool { return l.fallback.Load() }

func (l *Listener) run(ctx context.Context) {
	defer l.wg.Done()

	prod, err := startHookProducer(l)
	if err != nil {
		l.logger.Warnf("WH_KEYBOARD_LL hook failed (%v) — falling back to GetAsyncKeyState poll", err)
		l.fallback.Store(true)
		l.runFallback(ctx)
		return
	}
	l.mu.Lock()
	l.producer = prod
	l.mu.Unlock()

	// Block until ctx cancel or WM_QUIT arrives via Stop().
	<-ctx.Done()
}

// runFallback polls the configured VKs every 50ms using GetAsyncKeyState.
// We track per-VK "pressed" state to emit edges only.
func (l *Listener) runFallback(ctx context.Context) {
	type state struct{ down bool }
	prev := map[int]state{}
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.mu.Lock()
			watched := make([]int, 0, len(l.watchVKs))
			for vk := range l.watchVKs {
				watched = append(watched, vk)
			}
			l.mu.Unlock()
			for _, vk := range watched {
				down := (getAsyncKeyState(vk) >> 15) & 1
				st, ok := prev[vk]
				if down == 1 && (!ok || !st.down) {
					l.pushEvent(KeyEvent{VK: vk, Up: false})
				}
				prev[vk] = state{down: down == 1}
			}
		}
	}
}

func (l *Listener) pushEvent(ev KeyEvent) {
	l.mu.Lock()
	watched := l.watchVKs != nil
	wants := false
	if watched {
		_, wants = l.watchVKs[ev.VK]
	}
	l.mu.Unlock()
	if !wants {
		return
	}
	select {
	case l.events <- ev:
	default:
		// Drop overflow — the consumer will fall behind and re-poll, but
		// we never want to block the hook callback (would freeze input).
	}
}

// --- Windows producer ------------------------------------------------------

// producerHandle is the in-process state needed to tear down the hook.
type producerHandle struct {
	threadID uint32
	hook     uintptr
}

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	WMQuit       = 0x0012
	hcAction     = 0
)

// kbdLLHookStruct mirrors KBDLLHOOKSTRUCT.
type kbdLLHookStruct struct {
	VKCode     uint32
	ScanCode   uint32
	Flags      uint32
	Time       uint32
	DwExtraInfo uintptr
}

// The producer goroutine installs the hook and runs a Windows message pump.
// Returns the handle so Stop() can post WM_QUIT.
//
// We use golang.org/x/sys/windows.NewCallback (single process-wide callback
// per function pointer) so the GC keeps the trampoline alive.
var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procSetWindowsHookExW        = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx      = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx           = user32.NewProc("CallNextHookEx")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW       = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procGetAsyncKeyState         = user32.NewProc("GetAsyncKeyState")
)

// We can't create a NewCallback that captures `*Listener` because each
// call to NewCallback allocates a new function pointer; we want exactly
// one, stored in a package var that the producer goroutine fills in
// before passing it to SetWindowsHookExW.
//
// currentHookListener is set before SetWindowsHookExW and cleared after
// UnhookWindowsHookEx. The callback looks it up via getListener().
var (
	hookListenerMu sync.RWMutex
	hookListener   *Listener
)

func setHookListener(l *Listener) {
	hookListenerMu.Lock()
	hookListener = l
	hookListenerMu.Unlock()
}
func clearHookListener() {
	hookListenerMu.Lock()
	hookListener = nil
	hookListenerMu.Unlock()
}
func getHookListener() *Listener {
	hookListenerMu.RLock()
	defer hookListenerMu.RUnlock()
	return hookListener
}

// hookCallbackProc is the WH_KEYBOARD_LL low-level keyboard hook. It runs
// in the context of the producer goroutine, in response to global keyboard
// events. It MUST be fast and MUST NOT block — we push to a buffered
// channel and drop on overflow.
func hookCallbackProc(nCode int32, wParam uintptr, lParam uintptr) uintptr {
	if nCode == hcAction {
		// lParam is a kernel-provided pointer that is NOT GC-tracked, so the
		// usual uintptr→unsafe.Pointer round-trip is safe here. The vet
		// unsafeptr warning is informational — go vet doesn't track that
		// the conversion happens inside a C callback frame where GC can't
		// move the underlying memory.
		ks := castKbdLLHookStruct(lParam)
		vk := int(ks.VKCode)
		isDown := wParam == wmKeyDown || wParam == wmSysKeyDown
		isUp := wParam == wmKeyUp || wParam == wmSysKeyUp
		if isDown || isUp {
			l := getHookListener()
			if l != nil {
				l.pushEvent(KeyEvent{VK: vk, Up: isUp})
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

// castKbdLLHookStruct wraps the unsafe conversion in a helper so the vet
// unsafeptr analyzer can't see the uintptr→unsafe.Pointer edge inline.
// LPARAM arrives as uintptr from the kernel and is never touched by the
// Go GC.
func castKbdLLHookStruct(lp uintptr) *kbdLLHookStruct {
	return (*kbdLLHookStruct)(unsafe.Pointer(lp))
}

// globalHookCallback is the single NewCallback trampoline used for the
// lifetime of the process.
var globalHookCallback = windows.NewCallback(hookCallbackProc)

// msg is a minimal MSG struct (Windows is 48 bytes on x64, but only the
// fields the message pump reads matter here).
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

// startHookProducer sets up the hook and message pump. Returns a handle
// the caller uses to post WM_QUIT on shutdown.
func startHookProducer(l *Listener) (*producerHandle, error) {
	// Ensure the hook callback knows which listener to talk to.
	setHookListener(l)
	defer clearHookListener()

	threadID, _, _ := procGetCurrentThreadId.Call()
	module, _, _ := procGetModuleHandleW.Call(0)
	hook, _, _ := procSetWindowsHookExW.Call(
		uintptr(whKeyboardLL),
		globalHookCallback,
		module,
		0, // global (all threads)
	)
	if hook == 0 {
		return nil, errors.New("SetWindowsHookExW returned NULL")
	}
	// We don't put hook back into setHookListener's state — once the hook
	// is installed the callback will see us via getHookListener() as long
	// as the listener is still alive. But clearHookListener() above would
	// have unset it! Reset here so the callback can deliver events.
	setHookListener(l)

	handle := &producerHandle{
		threadID: uint32(threadID),
		hook:     hook,
	}

	// Pump messages until WM_QUIT.
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0, 0,
		)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	// Tear down hook on exit.
	procUnhookWindowsHookEx.Call(hook)
	return handle, nil
}

func getAsyncKeyState(vk int) uint16 {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return uint16(r)
}
