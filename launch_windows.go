// [INPUT]: 依赖 chrome_windows.go 的 chromeExecutable、launch.go 的 chromeArgs/runDetached/launchGrace。
// [OUTPUT]: 提供 Windows 的 launchChrome：直启 chrome.exe，交接失败可见、长驻不等待。
// [POS]: Windows 的启动事实；参数与观察窗口的语义在 launch.go，平台差异只在这里。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os/exec"
)

// launchChrome 直接启动 chrome.exe，由 runDetached 判定这次交付是否失败。
func launchChrome(profileDir, target string) error {
	exe, err := chromeExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, chromeArgs(profileDir, target)...)
	return runDetached(cmd, launchGrace)
}
