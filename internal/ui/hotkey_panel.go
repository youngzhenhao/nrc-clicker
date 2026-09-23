package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"nrc-clicker/internal/config"
)

// buildHotkeyPanelImpl composes the right column: one Select dropdown per
// hotkey role + a Save button. Phase 1 only honours the pause_resume hotkey
// but we render the full set so users can configure them ahead of the
// recording phase.
func (a *App) buildHotkeyPanelImpl() fyne.CanvasObject {
	a.hotkeySelects = make(map[string]*widget.Select, len(config.DefaultHotkeys))

	roles := []string{"pause_resume", "start_recording", "stop_recording", "cancel_recording", "mark_anchor"}

	rows := make([]fyne.CanvasObject, 0, len(roles)+2)
	rows = append(rows, widget.NewLabel("【热键配置】"), widget.NewSeparator())
	for _, role := range roles {
		label := widget.NewLabel(config.HotkeyLabels[role])
		sel := widget.NewSelect(fkeyOptions, nil)
		sel.SetSelected(a.Manager.Status().Hotkeys[role])
		a.hotkeySelects[role] = sel
		rows = append(rows, container.NewBorder(nil, nil, label, nil, sel))
	}

	a.saveBtn = widget.NewButton("💾 保存热键", func() {
		updates := config.Hotkeys{}
		for role, sel := range a.hotkeySelects {
			v := sel.Selected
			if v == "" {
				v = config.DefaultHotkeys[role]
			}
			updates[role] = v
		}
		if err := a.Manager.SetHotkeys(updates); err != nil {
			ShowToast(a.Fyne, "❌ "+err.Error(), 3*time.Second)
			return
		}
		ShowToast(a.Fyne, "✓ 热键配置已保存", 2*time.Second)
	})
	rows = append(rows, a.saveBtn)

	right := container.NewVBox(rows...)
	return container.NewScroll(right)
}

// fkeyOptions is a package-level cache of the F1..F12 dropdown options.
var fkeyOptions = func() []string {
	out := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		out = append(out, "F"+intToStr(i))
	}
	return out
}()
