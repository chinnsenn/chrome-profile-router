// [INPUT]: 依赖 chrome_linux.go 的 chromeExecutable、launch.go 的 chromeArgs/runDetached/launchGrace。
// [OUTPUT]: 提供 Linux 的 launchChrome：直启 Chrome Stable，独立会话，交接失败可见、长驻不等待。
// [POS]: Linux 的启动事实；参数与观察窗口的语义在 launch.go，平台差异只在这里。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os/exec"
	"syscall"
)

// launchChrome 直接启动 Chrome Stable，由 runDetached 判定这次交付是否失败。
func launchChrome(profileDir, target string) error {
	exe, err := chromeExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, chromeArgs(profileDir, target)...)
	detach(cmd)
	return runDetached(cmd, launchGrace)
}

// detach 让 Chrome 脱离本应用所在的会话：本应用交付完就退出，不能把浏览器
// 一起带走（Setsid 也避免终端关闭时的 SIGHUP 落到 Chrome 上）。
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
