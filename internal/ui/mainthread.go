package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// safeSetLabel updates a label's text on the Fyne main goroutine. Safe to
// call from any background goroutine (the hotkey dispatcher, the script
// runner, etc).
//
// Fyne 2.6+ exposes DoFromGoroutine for cross-thread UI updates. We use
// `wait=true` so that successive calls do not interleave widget
// mutations, with a hard ceiling of 500 ms to keep the dispatcher
// goroutine from stalling the clicker loop.
func safeSetLabel(l *widget.Label, text string) {
	if l == nil {
		return
	}
	dispatchLabel(l, text)
}

// safeSetEntry mirrors safeSetLabel but for Entry widgets.
func safeSetEntry(e *widget.Entry, text string) {
	if e == nil {
		return
	}
	dispatchEntry(e, text)
}

func dispatchLabel(l *widget.Label, text string) {
	app := fyne.CurrentApp()
	if app == nil {
		l.SetText(text)
		return
	}
	done := make(chan struct{})
	app.Driver().DoFromGoroutine(func() {
		l.SetText(text)
		close(done)
	}, true)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}

func dispatchEntry(e *widget.Entry, text string) {
	app := fyne.CurrentApp()
	if app == nil {
		e.SetText(text)
		return
	}
	done := make(chan struct{})
	app.Driver().DoFromGoroutine(func() {
		e.SetText(text)
		close(done)
	}, true)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}