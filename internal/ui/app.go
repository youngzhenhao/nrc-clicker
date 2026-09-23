// Package ui is the Fyne front-end for the nrc-clicker. It owns the main
// window, the three-column layout, the status poller, and the toast /
// dialog helpers.
//
// All widget mutations must happen on Fyne's main goroutine; use the
// safeSetLabel / safeSetEntry wrappers in mainthread.go or RunOnMain when
// firing updates from background goroutines.
package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"nrc-clicker/internal/manager"
)

// App is the long-lived UI state. One App per process.
type App struct {
	Fyne    fyne.App
	Window  fyne.Window
	Manager *manager.Manager

	statusPoller *statusPoller

	// Status panel widgets
	statusLabel     *widget.Label
	scriptLabel     *widget.Label
	countdownLabel  *widget.Label
	ignoredLabel    *widget.Label
	clickerCountLbl *widget.Label

	// Clicker param entries
	cxEntry   *widget.Entry
	cyEntry   *widget.Entry
	rEntry    *widget.Entry
	ivEntry   *widget.Entry
	holdEntry *widget.Entry
	jitEntry  *widget.Entry
	moveCheck *widget.Check

	// Script panel
	scriptList  *widget.List
	scripts     []string
	selectedIdx int
	lastTap     time.Time
	runBtn      *widget.Button
	pauseBtn    *widget.Button
	stopBtn     *widget.Button
	deleteBtn   *widget.Button
	refreshBtn  *widget.Button

	// Hotkey panel
	hotkeySelects map[string]*widget.Select
	saveBtn       *widget.Button
}

// Build constructs the App object, wires every callback, and returns it.
// The caller is responsible for invoking window.Show() and starting the
// Fyne event loop.
func Build(fyneApp fyne.App, mgr *manager.Manager) *App {
	fyneApp.Settings().SetTheme(newCustomTheme())

	a := &App{
		Fyne:    fyneApp,
		Manager: mgr,
	}
	a.Window = fyneApp.NewWindow("nrc-clicker")
	a.Window.Resize(fyne.NewSize(1100, 640))

	// --- left column ---
	a.statusLabel = widget.NewLabel("状态: 空闲")
	a.scriptLabel = widget.NewLabel("脚本: -")
	a.countdownLabel = widget.NewLabel("")
	a.ignoredLabel = widget.NewLabel("")
	a.clickerCountLbl = widget.NewLabel("点击数: 0")

	a.cxEntry = widget.NewEntry()
	a.cyEntry = widget.NewEntry()
	a.rEntry = widget.NewEntry()
	a.ivEntry = widget.NewEntry()
	a.holdEntry = widget.NewEntry()
	a.jitEntry = widget.NewEntry()
	a.moveCheck = widget.NewCheck("移动鼠标到目标点 (move_mouse)", func(b bool) {
		a.Manager.SetMoveMouse(b)
	})

	left := a.buildStatusPanel()

	// --- middle column ---
	a.scriptList = widget.NewList(
		func() int { return len(a.scripts) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i int, o fyne.CanvasObject) {
			if i < 0 || i >= len(a.scripts) {
				return
			}
			o.(*widget.Label).SetText(a.scripts[i])
		},
	)
	// Fyne v2 List exposes OnSelected only — we use a tap-time tracker so a
	// quick second tap on the same row triggers double-click run.
	a.scriptList.OnSelected = func(id widget.ListItemID) {
		now := time.Now()
		if a.selectedIdx == id && now.Sub(a.lastTap) < 400*time.Millisecond {
			a.lastTap = time.Time{}
			if id >= 0 && id < len(a.scripts) {
				a.runScriptByID(id)
			}
			return
		}
		a.selectedIdx = id
		a.lastTap = now
	}
	a.refreshBtn = widget.NewButton("↻ 刷新", func() { a.refreshScripts() })
	a.deleteBtn = widget.NewButton("🗑 删除", func() { a.deleteSelected() })
	a.runBtn = widget.NewButton("▶ 执行", func() { a.runSelected() })
	a.pauseBtn = widget.NewButton("⏸ 暂停", func() { a.togglePause() })
	a.stopBtn = widget.NewButton("⏹ 停止", func() { a.Manager.StopScript() })

	middle := a.buildScriptsPanel()

	// --- right column ---
	right := a.buildHotkeyPanel()

	content := container.NewGridWithColumns(3, left, middle, right)

	a.Window.SetContent(container.NewPadded(content))
	a.Window.SetOnClosed(func() {
		a.statusPoller.stop()
		a.Manager.Shutdown()
	})

	a.statusPoller = newStatusPoller(a)
	a.refreshScripts()
	a.populateFromStatus()
	return a
}

// commitClickerParams reads the entry fields, parses them, and atomically
// updates the manager's config store.
func (a *App) commitClickerParams() {
	cfg := a.Manager.Status().Config
	cfg.CentreX = parseInt(a.cxEntry.Text, cfg.CentreX)
	cfg.CentreY = parseInt(a.cyEntry.Text, cfg.CentreY)
	cfg.Radius = parseInt(a.rEntry.Text, cfg.Radius)
	cfg.IntervalMs = parseInt(a.ivEntry.Text, cfg.IntervalMs)
	cfg.HoldMs = parseInt(a.holdEntry.Text, cfg.HoldMs)
	cfg.JitterRangeMs = parseInt(a.jitEntry.Text, cfg.JitterRangeMs)
	a.Manager.UpdateConfig(cfg)
	ShowToast(a.Fyne, "✓ 参数已保存", 2*time.Second)
}

// refreshScripts reloads the script list from disk.
func (a *App) refreshScripts() {
	names, err := a.Manager.ListScripts()
	if err != nil {
		ShowToast(a.Fyne, "❌ 列出脚本失败: "+err.Error(), 3*time.Second)
		return
	}
	a.scripts = names
	a.scriptList.Refresh()
}

// populateFromStatus initialises entries from the current Config.
func (a *App) populateFromStatus() {
	cfg := a.Manager.Status().Config
	safeSetEntry(a.cxEntry, intToStr(cfg.CentreX))
	safeSetEntry(a.cyEntry, intToStr(cfg.CentreY))
	safeSetEntry(a.rEntry, intToStr(cfg.Radius))
	safeSetEntry(a.ivEntry, intToStr(cfg.IntervalMs))
	safeSetEntry(a.holdEntry, intToStr(cfg.HoldMs))
	safeSetEntry(a.jitEntry, intToStr(cfg.JitterRangeMs))
	a.moveCheck.SetChecked(cfg.MoveMouse)
}

func (a *App) runSelected() {
	if a.selectedIdx < 0 || a.selectedIdx >= len(a.scripts) {
		ShowToast(a.Fyne, "请先选择脚本", 2*time.Second)
		return
	}
	a.runScriptByID(a.selectedIdx)
}

func (a *App) runScriptByID(id int) {
	name := a.scripts[id]
	if err := a.Manager.RunScript(name); err != nil {
		ShowToast(a.Fyne, "❌ 启动脚本失败: "+err.Error(), 3*time.Second)
	}
}

func (a *App) deleteSelected() {
	if a.selectedIdx < 0 || a.selectedIdx >= len(a.scripts) {
		ShowToast(a.Fyne, "请先选择脚本", 2*time.Second)
		return
	}
	name := a.scripts[a.selectedIdx]
	d := dialog.NewConfirm("删除脚本", "确定删除 "+name+" ?", func(ok bool) {
		if !ok {
			return
		}
		if err := a.Manager.DeleteScript(name); err != nil {
			ShowToast(a.Fyne, "❌ 删除失败: "+err.Error(), 3*time.Second)
		}
		a.selectedIdx = -1
		a.refreshScripts()
	}, a.Window)
	d.Show()
}

// togglePause mirrors manager.dispatchLoop priority.
func (a *App) togglePause() {
	st := a.Manager.Status()
	if st.ScriptRunning {
		if st.ScriptPaused {
			a.Manager.ResumeScript()
		} else {
			a.Manager.PauseScript()
		}
		return
	}
	a.Manager.ToggleClickerPause()
}

// buildStatusPanel returns the left column container.
func (a *App) buildStatusPanel() fyne.CanvasObject { return a.buildStatusPanelImpl() }

// buildScriptsPanel returns the middle column container.
func (a *App) buildScriptsPanel() fyne.CanvasObject { return a.buildScriptsPanelImpl() }

// buildHotkeyPanel returns the right column container.
func (a *App) buildHotkeyPanel() fyne.CanvasObject { return a.buildHotkeyPanelImpl() }

// parseInt returns the parsed non-negative int, or fallback on error / empty.
func parseInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// intToStr is a tiny non-fmt int formatter.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	if neg {
		out = "-" + out
	}
	return out
}
