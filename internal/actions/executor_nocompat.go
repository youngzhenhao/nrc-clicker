//go:build !windows

package actions

import (
	"time"

	"github.com/sirupsen/logrus"
)

// Non-Windows stub for keybdFallback. The auto-clicker is Windows-only;
// the stub keeps `go build` happy on dev machines without losing the
// stub-keyword reference in executor.go.
func keybdFallback(vk uint16, hold time.Duration) {
	logrus.Warnf("keybdFallback called on non-Windows platform (vk=0x%X)", vk)
	time.Sleep(hold)
}
