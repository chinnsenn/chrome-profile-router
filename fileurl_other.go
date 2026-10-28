// [INPUT]: 依赖 fileTarget 给出的 POSIX 绝对路径。
// [OUTPUT]: 提供 fileURLParts：POSIX 路径没有 host，path 原样交给 url.URL 转义。
// [POS]: 非 Windows 的 file URL 组成事实；Windows 的盘符与 UNC 规则在 fileurl_windows.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
//go:build !windows

package main

// fileURLParts 把绝对路径拆成 file URL 的 host 与 path。
//
// POSIX 路径没有 host 分量，直接交给 url.URL 做百分号编码即可。
func fileURLParts(abs string) (host, path string) {
	return "", abs
}
