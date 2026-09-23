package interception

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dll wraps a loaded interception.dll with cached Proc handles.
//
// We use syscall.LoadDLL rather than windows.NewLazyDLL because we want to
// resolve the path explicitly (the DLL is not on the system search path;
// it ships next to the executable).
type dll struct {
	handle *syscall.DLL

	createCtx   *syscall.Proc
	destroyCtx  *syscall.Proc
	send        *syscall.Proc
	isKeyboard  *syscall.Proc
	isMouse     *syscall.Proc
	isInvalid   *syscall.Proc
	setFilter   *syscall.Proc
	getFilter   *syscall.Proc
	wait        *syscall.Proc
	waitTimeout *syscall.Proc
	receive     *syscall.Proc

	// initError is a human-readable failure cause; empty when DLL + context
	// are both OK.
	initError string
}

// arch returns "x64" or "x86" matching the subdirectory naming the upstream
// Interception library uses.
func arch() string {
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		return "x64"
	}
	return "x86"
}

// DLL lookup candidates, tried in order. Mirrors InterceptionCore.py
// (dll_paths list).
func dllCandidatePaths() []string {
	exe, _ := os.Executable()
	exeDir := ""
	if exe != "" {
		exeDir = filepath.Dir(exe)
	}
	cwd, _ := os.Getwd()
	a := arch()

	return []string{
		filepath.Join(exeDir, "interception.dll"),
		filepath.Join("third", "Interception", "library", a, "interception.dll"),
		filepath.Join("third", "Interception", "library", "x64", "interception.dll"),
		filepath.Join("third", "Interception", "library", "x86", "interception.dll"),
		filepath.Join(exeDir, "third", "Interception", "library", a, "interception.dll"),
		filepath.Join(cwd, "interception.dll"),
	}
}

// loadDLL tries each candidate path and returns the first that opens.
func loadDLL() (*dll, error) {
	var tried []string
	for _, p := range dllCandidatePaths() {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		h, err := syscall.LoadDLL(p)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		d, perr := bindProcs(h)
		if perr != nil {
			_ = h.Release()
			tried = append(tried, fmt.Sprintf("%s: bind failed: %v", p, perr))
			continue
		}
		return d, nil
	}
	if len(tried) == 0 {
		return nil, fmt.Errorf("interception.dll not found in any of the %d known paths", len(dllCandidatePaths()))
	}
	return nil, fmt.Errorf("interception.dll present but failed to bind: %s", strings.Join(tried, "; "))
}

// bindProcs resolves every exported symbol the wrapper touches. Any missing
// symbol is a hard failure.
func bindProcs(h *syscall.DLL) (*dll, error) {
	names := []string{
		"interception_create_context",
		"interception_destroy_context",
		"interception_send",
		"interception_is_keyboard",
		"interception_is_mouse",
		"interception_is_invalid",
		"interception_set_filter",
		"interception_get_filter",
		"interception_wait",
		"interception_wait_with_timeout",
		"interception_receive",
	}
	procs := make(map[string]*syscall.Proc, len(names))
	for _, n := range names {
		p, err := h.FindProc(n)
		if err != nil {
			return nil, fmt.Errorf("missing symbol %s: %w", n, err)
		}
		procs[n] = p
	}
	return &dll{
		handle:      h,
		createCtx:   procs["interception_create_context"],
		destroyCtx:  procs["interception_destroy_context"],
		send:        procs["interception_send"],
		isKeyboard:  procs["interception_is_keyboard"],
		isMouse:     procs["interception_is_mouse"],
		isInvalid:   procs["interception_is_invalid"],
		setFilter:   procs["interception_set_filter"],
		getFilter:   procs["interception_get_filter"],
		wait:        procs["interception_wait"],
		waitTimeout: procs["interception_wait_with_timeout"],
		receive:     procs["interception_receive"],
	}, nil
}

func (d *dll) release() {
	if d == nil || d.handle == nil {
		return
	}
	_ = d.handle.Release()
	d.handle = nil
}

// strokePointer returns a uintptr pointing at the first element of the
// supplied slice. We use [1]Stroke rather than &slice[0] so the result is
// addressable without escape-analysis gymnastics.
func strokePointer(s []MouseStroke) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

func keyPointer(s []KeyStroke) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&s[0]))
}

// Predicate callbacks: interception_set_filter expects function pointers
// matching `int predicate(int device)`. Go's syscall package cannot hand a
// regular Go function pointer to a C library, so we go through
// windows.NewCallback which uses a small runtime trampoline.
//
// IMPORTANT: NewCallback must be called exactly once per process. Storing
// the returned uintptr keeps the underlying memory alive.

var (
	predicateOnce      sync.Once
	keyboardPredicate  uintptr
	mousePredicate     uintptr
	predicateErr       error
)

func ensurePredicates() (uintptr, uintptr, error) {
	predicateOnce.Do(func() {
		keyboardPredicate = windows.NewCallback(func(device uintptr) uintptr {
			d, ok := currentDLL()
			if !ok || d == nil || d.isKeyboard == nil {
				return 0
			}
			r, _, _ := d.isKeyboard.Call(device)
			return r
		})
		mousePredicate = windows.NewCallback(func(device uintptr) uintptr {
			d, ok := currentDLL()
			if !ok || d == nil || d.isMouse == nil {
				return 0
			}
			r, _, _ := d.isMouse.Call(device)
			return r
		})
		predicateErr = nil
	})
	return keyboardPredicate, mousePredicate, predicateErr
}

var (
	currentDLLMu sync.RWMutex
	currentDLLv  *dll
)

func setCurrentDLL(d *dll) {
	currentDLLMu.Lock()
	currentDLLv = d
	currentDLLMu.Unlock()
}

func currentDLL() (*dll, bool) {
	currentDLLMu.RLock()
	defer currentDLLMu.RUnlock()
	return currentDLLv, currentDLLv != nil
}
