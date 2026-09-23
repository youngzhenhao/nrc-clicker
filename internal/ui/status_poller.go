package ui

import (
	"time"
)

// statusPoller reads Manager.Status() at 5 Hz and pushes updates to Fyne
// widgets via the safeSetLabel wrapper. It also surfaces toast
// notifications for state transitions (script started / stopped, etc).
type statusPoller struct {
	app    *App
	stopCh chan struct{}
}

func newStatusPoller(a *App) *statusPoller {
	sp := &statusPoller{
		app:    a,
		stopCh: make(chan struct{}),
	}
	go sp.run()
	return sp
}

func (sp *statusPoller) stop() {
	select {
	case <-sp.stopCh:
		// already stopped
	default:
		close(sp.stopCh)
	}
}

func (sp *statusPoller) run() {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	var prevScriptRunning, prevClickerRunning bool

	for {
		select {
		case <-sp.stopCh:
			return
		case <-t.C:
		}
		s := sp.app.Manager.Status()

		// Status label summarises "what's running" in one line.
		status := "空闲"
		switch {
		case s.SimpleClickerRunning && s.ScriptRunning:
			status = "连点器 + 脚本 同时运行（异常）"
		case s.SimpleClickerRunning && s.SimpleClickerPaused:
			status = "连点器运行中（已暂停）"
		case s.SimpleClickerRunning:
			status = "连点器运行中"
		case s.ScriptRunning && s.ScriptPaused:
			status = "脚本运行中（已暂停）"
		case s.ScriptRunning:
			status = "脚本运行中"
		}
		safeSetLabel(sp.app.statusLabel, "状态: "+status)
		safeSetLabel(sp.app.scriptLabel, "脚本: "+orDash(s.ScriptName))
		safeSetLabel(sp.app.clickerCountLbl, "点击数: "+intToStr(int(s.ClickerCount)))
		safeSetLabel(sp.app.ignoredLabel, "忽略移动次数: "+intToStr(int(s.IgnoredMoves)))

		if s.CountdownRemaining > 0 {
			safeSetLabel(sp.app.countdownLabel, "⏳ "+s.CountdownLabel+" ("+intToStr(s.CountdownRemaining)+"s)")
		} else {
			safeSetLabel(sp.app.countdownLabel, "")
		}

		// Detect edges for toasts.
		if s.ScriptRunning && !prevScriptRunning {
			ShowToast(sp.app.Fyne, "▶ 脚本已启动: "+s.ScriptName, 2*time.Second)
		}
		if !s.ScriptRunning && prevScriptRunning {
			ShowToast(sp.app.Fyne, "⏹ 脚本已停止", 2*time.Second)
		}
		if s.SimpleClickerRunning && !prevClickerRunning {
			ShowToast(sp.app.Fyne, "▶ 连点器已启动", 2*time.Second)
		}
		if !s.SimpleClickerRunning && prevClickerRunning {
			ShowToast(sp.app.Fyne, "⏹ 连点器已停止", 2*time.Second)
		}
		prevScriptRunning = s.ScriptRunning
		prevClickerRunning = s.SimpleClickerRunning
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
