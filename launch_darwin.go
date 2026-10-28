// [INPUT]: 依赖 /usr/bin/open 与 Chrome 的 bundle id com.google.Chrome。
// [OUTPUT]: 提供 macOS 的 launchChrome：按 bundle id 拉起 Chrome 新实例，带超时，参数透传。
// [POS]: macOS 的启动事实；路径定位交给 LaunchServices，所以这里不解析 chrome 可执行文件。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// launchChrome 使用带超时的 Context 调用 /usr/bin/open 拉起 Chrome。
func launchChrome(profileDir, target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), launchTimeout)
	defer cancel()

	args := []string{
		"-n",                      // 新实例，允许同一 Profile 多开窗口
		"-b", "com.google.Chrome", // 按 bundle id 定位，避免中文环境下的名称匹配问题
		"--args", // 之后的参数透传给 Chrome
	}
	args = append(args, chromeArgs(profileDir, target)...)

	cmd := exec.CommandContext(ctx, "/usr/bin/open", args...)
	out, err := cmd.CombinedOutput()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("启动 Chrome 超时（%s）", launchTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("启动 Chrome 失败: %w", err)
		}
		return fmt.Errorf("启动 Chrome 失败: %s", msg)
	}
	return nil
}
