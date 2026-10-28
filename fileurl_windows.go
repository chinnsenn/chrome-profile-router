// [INPUT]: 依赖 fileTarget 给出的 Windows 绝对路径（盘符路径或 UNC 路径）。
// [OUTPUT]: 提供 fileURLParts：盘符路径得到空 host 与 "/C:/…"，UNC 得到 server host 与 "/share/…"。
// [POS]: Windows 的 file URL 组成事实；反斜线绝不能进 URL，否则会变成 %5C 且 drive 被当成 scheme 的一部分。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
//go:build windows

package main

import (
	"path/filepath"
	"strings"
)

// fileURLParts 把 Windows 绝对路径拆成 file URL 的 host 与 path。
//
//	`C:\Users\报 告\Desktop\a #1.html` → host "", path "/C:/Users/报 告/Desktop/a #1.html"
//	`\\server\share\a #1.html`         → host "server", path "/share/a #1.html"
//
// 盘符路径必须以 "/" 起头，否则 `C:` 会被当成 URL 的 scheme；UNC 的 server 属于
// host，share 与其余部分是 path。反斜线在这里一次性换成 "/"，URL 里不再出现。
func fileURLParts(abs string) (host, path string) {
	p := filepath.ToSlash(abs)

	if rest, ok := strings.CutPrefix(p, "//"); ok {
		server, tail, _ := strings.Cut(rest, "/")
		return server, "/" + tail
	}

	if len(p) >= 2 && p[1] == ':' {
		return "", "/" + p
	}

	return "", p
}
