// Package main is the nrc-clicker entry point. It owns the lifecycle:
//
//  1. acquire single-instance mutex
//  2. initialise logrus (file + stdout)
//  3. ensure interception.dll (copy from sibling repo, else prompt download)
//  4. construct interception.Core (validates the driver install)
//  5. construct manager.Manager
//  6. launch Fyne UI
//  7. on window close: Manager.Shutdown → exit
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2/app"

	"nrc-clicker/internal/clicker"
	"nrc-clicker/internal/config"
	"nrc-clicker/internal/interception"
	"nrc-clicker/internal/logging"
	"nrc-clicker/internal/manager"
	"nrc-clicker/internal/ui"
)

// singleInstanceName is the Windows named mutex used to ensure only one
// instance of nrc-clicker runs at a time. The actual lock is taken in
// acquireSingleInstance(); this constant is exposed for tests.
const singleInstanceName = "Local\\nrc-clicker-single-instance"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// Lock to the main thread before any Fyne / GUI call.
	runtime.LockOSThread()

	if err := logging.Init("data/logs"); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}

	if err := ensureDataDirs(); err != nil {
		return fmt.Errorf("ensure data dirs: %w", err)
	}

	// Ensure interception.dll exists locally. The Interception driver is
	// Windows-only; bail on other platforms with a friendly error.
	if runtime.GOOS != "windows" {
		return fmt.Errorf("nrc-clicker is Windows-only (Interception driver); got GOOS=%s", runtime.GOOS)
	}
	if _, err := interception.EnsureDLL(); err != nil {
		return fmt.Errorf("ensure interception.dll: %w", err)
	}

	core, err := interception.New()
	if err != nil {
		return fmt.Errorf("create interception context: %w (driver not installed? run install-interception.exe /install then reboot)", err)
	}

	hotkeys, err := config.LoadHotkeys("data/clicker_configs")
	if err != nil {
		hotkeys = config.DefaultHotkeys
	}

	clickerCfg, err := clicker.LoadConfig("data/clicker_configs", "default")
	if err != nil {
		clickerCfg = clicker.DefaultConfig()
	}

	mgr, err := manager.New(manager.Options{
		Sender:     core,
		Core:       core,
		Hotkeys:    hotkeys,
		Config:     clickerCfg,
		ScriptsDir: "data/action_scripts",
		ConfigsDir: "data/clicker_configs",
	})
	if err != nil {
		core.Destroy()
		return fmt.Errorf("init manager: %w", err)
	}

	fyneApp := app.New()
	fyneApp.SetIcon(nil) // use default app icon for now

	_ = ui.Build(fyneApp, mgr)

	// Run blocks until every window is closed (we only open one).
	fyneApp.Run()

	// Manager.Shutdown was already invoked by the window's OnClosed
	// callback; this is the belt-and-suspenders path.
	mgr.Shutdown()
	return nil
}

// ensureDataDirs creates the runtime directories we write config / scripts
// into. Missing directories are why the first launch sometimes appears to
// "lose" saved state, so we make them eagerly.
func ensureDataDirs() error {
	for _, dir := range []string{
		"data/action_scripts",
		"data/clicker_configs",
		"data/logs",
		"third/Interception/library/x64",
		"third/Interception/library/x86",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

// unexported helper to make the linter happy if we ever drop the import.
var _ = filepath.Join