//go:build windows

package actions

import (
	"syscall"
)

// tinyLazyDLL is a 30-line wrapper around syscall.LoadDLL for our two
// user32 entry points. We don't pull in golang.org/x/sys/windows just to
// call keybd_event; the package already has a transitive dependency via
// interception, but using it directly here would create an import cycle
// (interception → actions is the wrong direction).
type tinyLazyDLL struct {
	handle *syscall.DLL
	procs  map[string]*syscall.Proc
}

func newLazyDLL(name string) *tinyLazyDLL {
	h, err := syscall.LoadDLL(name)
	if err != nil {
		return &tinyLazyDLL{procs: map[string]*syscall.Proc{}}
	}
	return &tinyLazyDLL{handle: h, procs: map[string]*syscall.Proc{}}
}

func (d *tinyLazyDLL) newProc(name string) *tinyProc {
	p, ok := d.procs[name]
	if !ok {
		proc, err := d.handle.FindProc(name)
		if err != nil {
			return nil
		}
		d.procs[name] = proc
		p = proc
	}
	return &tinyProc{p: p}
}

// tinyProc wraps a syscall.Proc with a panic-free .call helper.
type tinyProc struct {
	p *syscall.Proc
}

func (t *tinyProc) call(args ...uintptr) {
	if t == nil || t.p == nil {
		return
	}
	_, _, _ = t.p.Call(args...)
}
