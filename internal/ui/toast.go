package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ShowToast displays a transient borderless notification. Safe to call
// from any goroutine — the work is marshalled to Fyne's main thread
// via Driver.DoFromGoroutine.
//
// We create a small top-level window (no decorations) per toast and let
// the garbage collector reclaim it after the duration elapses. Fyne
// doesn't expose exact screen coordinates on every platform, so we use
// CenterOnScreen and accept the OS default placement.
func ShowToast(app fyne.App, text string, duration time.Duration) {
	if app == nil {
		return
	}
	fn := func() {
		w := app.NewWindow("") // empty title keeps the chrome minimal
		w.SetFixedSize(true)
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		w.SetContent(container.NewVBox(label))
		w.Resize(fyne.NewSize(360, 80))
		w.CenterOnScreen()
		w.Show()
		time.AfterFunc(duration, func() {
			cur := fyne.CurrentApp()
			if cur == nil {
				return
			}
			cur.Driver().DoFromGoroutine(func() { w.Close() }, false)
		})
	}
	app.Driver().DoFromGoroutine(fn, false)
}