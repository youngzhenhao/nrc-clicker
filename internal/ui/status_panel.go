package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// buildStatusPanelImpl composes the left column: status readouts, clicker
// parameter form, simple-clicker run/pause/stop buttons.
func (a *App) buildStatusPanelImpl() fyne.CanvasObject {
	saveBtn := widget.NewButton("💾 保存参数", func() { a.commitClickerParams() })

	startClickerBtn := widget.NewButton("▶ 启动连点器", func() {
		a.commitClickerParams()
		a.Manager.ToggleClicker()
	})

	togglePauseBtn := widget.NewButton("⏸ 暂停/继续", func() {
		a.Manager.ToggleClickerPause()
	})

	stopClickerBtn := widget.NewButton("⏹ 停止连点器", func() { a.Manager.ToggleClicker() })

	scriptPauseBtn := widget.NewButton("⏸ 暂停脚本", func() { a.togglePause() })
	scriptStopBtn := widget.NewButton("⏹ 停止脚本", func() { a.Manager.StopScript() })

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "中心 X", Widget: a.cxEntry},
			{Text: "中心 Y", Widget: a.cyEntry},
			{Text: "半径 (px)", Widget: a.rEntry},
			{Text: "间隔 (ms)", Widget: a.ivEntry},
			{Text: "按住 (ms)", Widget: a.holdEntry},
			{Text: "抖动 (ms)", Widget: a.jitEntry},
		},
	}

	left := container.NewVBox(
		widget.NewLabel("【状态】"),
		a.statusLabel,
		a.scriptLabel,
		a.countdownLabel,
		a.clickerCountLbl,
		a.ignoredLabel,
		widget.NewSeparator(),
		widget.NewLabel("【连点器参数】"),
		form,
		a.moveCheck,
		saveBtn,
		widget.NewSeparator(),
		widget.NewLabel("【简单连点器控制】"),
		container.NewHBox(startClickerBtn, togglePauseBtn, stopClickerBtn),
		widget.NewSeparator(),
		widget.NewLabel("【脚本控制】"),
		container.NewHBox(scriptPauseBtn, scriptStopBtn),
	)
	return container.NewScroll(left)
}
