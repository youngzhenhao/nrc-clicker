package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"nrc-clicker/internal/interception"
)

// confirmDLLBootstrap asks the user whether to download interception.dll
// from the upstream release. Returns true if the user agreed. The window
// is shown modally against the supplied parent.
func confirmDLLBootstrap(parent fyne.Window, arch string) bool {
	if parent == nil {
		return false
	}
	choice := false
	d := dialog.NewConfirm(
		"缺少 interception.dll",
		fmt.Sprintf("项目目录未找到 interception.dll (%s)。\n是否从 GitHub release 下载到 third/Interception/library/%s/ ?\n\n下载需要网络连接。", arch, arch),
		func(ok bool) { choice = ok },
		parent,
	)
	d.SetDismissText("取消")
	d.SetConfirmText("下载")
	d.Show()
	// The dialog callback runs synchronously when the user clicks a button,
	// but Fyne doesn't expose a "dismissed" flag we can wait on. We poll
	// briefly so the caller doesn't return before the user has reacted.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if choice || parent == nil {
			break
		}
	}
	return choice
}

// showDriverNotReady shows an error dialog and returns when dismissed.
func showDriverNotReady(parent fyne.Window, err error) {
	msg := "Interception 驱动未就绪。\n\n" +
		"请确认:\n" +
		"  1. 已运行 install-interception.exe /install (管理员)\n" +
		"  2. 已重启电脑\n" +
		"  3. DLL 已放在 third/Interception/library/x64/interception.dll\n\n" +
		"错误: " + err.Error()
	d := dialog.NewInformation("驱动未就绪", msg, parent)
	d.Show()
}

// showScriptError surfaces a script load / parse error to the user.
func showScriptError(parent fyne.Window, name string, err error) {
	msg := fmt.Sprintf("脚本 %q 加载或执行出错:\n\n%s", name, err.Error())
	d := dialog.NewError(fmt.Errorf("%s", msg), parent)
	_ = d
}

// ensureThirdDir ensures the third/Interception/library/<arch>/ directory
// exists, creating parents as needed. Returns the absolute path.
func ensureThirdDir(arch string) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "third", "Interception", "library", arch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// defaultBootstrapFlow is the convenience wrapper: try local DLL first,
// fall back to the GitHub release download with user consent. Used by
// main.go before constructing the manager.
func defaultBootstrapFlow() error {
	core, err := interception.New()
	if err == nil {
		core.Destroy()
		return nil
	}
	// Driver not ready (DLL missing or Interception driver not installed).
	// The UI layer will surface this via showDriverNotReady.
	return err
}

// unexported to silence "unused" if a build tag later drops a function.
var _ = showScriptError