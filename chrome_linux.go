// [INPUT]: 依赖 XDG_CONFIG_HOME、HOME 与 PATH 上的 google-chrome / google-chrome-stable。
// [OUTPUT]: 提供 chromeDataDir（XDG 配置目录下的 google-chrome）与 chromeExecutable（PATH 上的 Chrome Stable）。
// [POS]: Linux 的平台路径事实；信任边界只有这三个环境变量，不查 Flatpak/Snap/Chromium，也不读注册表式的额外来源。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// chromeDataDir 返回 Linux 的 Chrome 用户数据根目录。
//
// XDG_CONFIG_HOME 为空或是相对路径时按未设置处理（XDG 规范要求忽略相对路径）。
func chromeDataDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "google-chrome"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户主目录: %w", err)
	}
	return filepath.Join(home, ".config", "google-chrome"), nil
}

// chromeExecutable 在 PATH 上找 Chrome Stable。
//
// 只认 Stable 的两个命令名：google-chrome 是发行包的主名，
// google-chrome-stable 是 Debian 系的别名。Chromium 与其他通道不在范围内。
func chromeExecutable() (string, error) {
	tried := []string{"google-chrome", "google-chrome-stable"}
	for _, name := range tried {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Chrome 可执行文件（已查找：%s）", joinList(tried))
}
