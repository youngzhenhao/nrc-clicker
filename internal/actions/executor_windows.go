//go:build windows

package actions

import (
	"time"

	"github.com/sirupsen/logrus"
)

// keybdFallback issues a single key via user32.keybd_event when the
// Interception Send path fails (typically because the driver is not
// installed). This is the same fallback ActionScript.py uses after a
// failed interception_send.
//
// We use syscalls directly rather than golang.org/x/sys/windows to keep
// the executor package free of OS imports outside the build tag.
var (
	procUser32 = newLazyDLL("user32.dll")
	procKeybd  = procUser32.newProc("keybd_event")
)

// keybd_event flags.
const keybdEventKeyUp = 0x0002

// keybdFallback presses and releases a virtual key without going through
// Interception. It silently logs an error on Windows-API failure rather
// than panicking — the executor continues running either way.
func keybdFallback(vk uint16, hold time.Duration) {
	if procKeybd == nil {
		logrus.Warnf("keybdFallback unavailable (user32.dll not loadable)")
		return
	}
	procKeybd.call(uintptr(vk), 0, 0, 0)
	time.Sleep(hold)
	procKeybd.call(uintptr(vk), 0, keybdEventKeyUp, 0)
}
