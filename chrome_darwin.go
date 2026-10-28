// [INPUT]: 依赖标准库 os 与 macOS 的 Chrome 用户数据位置。
// [OUTPUT]: 提供 chromeDataDir：~/Library/Application Support/Google/Chrome。
// [POS]: macOS 的平台路径事实；启动仍走 launch_darwin.go 的 open，不需要 chromeExecutable。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// chromeDataDir 返回 macOS 的 Chrome 用户数据根目录。
func chromeDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法定位用户主目录: %w", err)
	}
	return filepath.Join(home, "Library/Application Support/Google/Chrome"), nil
}
