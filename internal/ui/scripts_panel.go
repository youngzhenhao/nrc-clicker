package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// buildScriptsPanelImpl composes the middle column: a List of saved
// scripts, double-click to run, plus the run/pause/stop/delete/refresh
// control row.
func (a *App) buildScriptsPanelImpl() fyne.CanvasObject {
	title := widget.NewLabel("【动作脚本】双击运行")
	help := widget.NewLabel("支持的类型: click / move / key / combo / wait / loop / timed")

	middle := container.NewVBox(
		title,
		help,
		container.NewBorder(nil, nil, nil, nil, a.scriptList),
		container.NewHBox(a.runBtn, a.pauseBtn, a.stopBtn),
		container.NewHBox(a.refreshBtn, a.deleteBtn),
	)
	return container.NewScroll(middle)
}
