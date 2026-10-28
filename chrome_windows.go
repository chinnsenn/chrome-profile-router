// [INPUT]: 依赖 LOCALAPPDATA、ProgramFiles、ProgramFiles(x86) 下的 Chrome 安装目录。
// [OUTPUT]: 提供 chromeDataDir（%LOCALAPPDATA%\Google\Chrome\User Data）与 chromeExecutable（按 per-user 优先顺序找到的 chrome.exe）。
// [POS]: Windows 的平台路径事实；只认 Chrome Stable 的三个常见安装位置，不读注册表、不查其他通道。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// chromeDataDir 返回 Windows 的 Chrome 用户数据根目录。
//
// LOCALAPPDATA 是唯一来源：取不到就如实报错，不回退 USERPROFILE 猜测。
func chromeDataDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("未找到 LOCALAPPDATA，无法定位 Chrome 用户数据")
	}
	return filepath.Join(local, "Google", "Chrome", "User Data"), nil
}

// chromeExecutable 按 per-user 优先的顺序找 chrome.exe。
//
// 顺序是 LOCALAPPDATA（当前用户的 per-user 安装）→ ProgramFiles →
// ProgramFiles(x86)；第一个真实存在的文件胜出。三个都没有就报错并列出
// 已查找的路径，让界面能如实说明失败原因。
func chromeExecutable() (string, error) {
	roots := []string{
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	tried := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		exe := filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe")
		tried = append(tried, exe)
		if fi, err := os.Stat(exe); err == nil && fi.Mode().IsRegular() {
			return exe, nil
		}
	}
	if len(tried) == 0 {
		return "", fmt.Errorf("未找到 Chrome 可执行文件：LOCALAPPDATA 与 ProgramFiles 都没有给出安装根目录")
	}
	return "", fmt.Errorf("未找到 Chrome 可执行文件（已查找：%s）", joinList(tried))
}
