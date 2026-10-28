// [INPUT]: 依赖 fileTarget 对 Windows 绝对路径的输出。
// [OUTPUT]: 保护 Windows 的 file URL 组成：盘符路径是 file:///C:/…（无反斜线、无 %5C），UNC 是 file://server/share/…，空格与 # 正确转义。
// [POS]: Windows 上 fileTarget 的行为边界；断言用 url.Parse 独立解码，不复述实现。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
//go:build windows

package main

import (
	"net/url"
	"strings"
	"testing"
)

func parseFileTarget(t *testing.T, path string) (*url.URL, string) {
	t.Helper()
	target, err := fileTarget(path)
	if err != nil {
		t.Fatalf("fileTarget(%q): %v", path, err)
	}
	if strings.Contains(target, `\`) || strings.Contains(target, "%5C") {
		t.Fatalf("file URL keeps a backslash: %q", target)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("file URL %q does not parse: %v", target, err)
	}
	return parsed, target
}

func TestFileTargetWindowsDrivePath(t *testing.T) {
	parsed, target := parseFileTarget(t, `C:\Users\报 告\Desktop\a #1.html`)

	if parsed.Scheme != "file" {
		t.Fatalf("scheme = %q in %q", parsed.Scheme, target)
	}
	if parsed.Host != "" {
		t.Fatalf("host = %q, want none for a drive path (%q)", parsed.Host, target)
	}
	if want := "/C:/Users/报 告/Desktop/a #1.html"; parsed.Path != want {
		t.Fatalf("path = %q, want %q (%q)", parsed.Path, want, target)
	}
}

func TestFileTargetWindowsUNCPath(t *testing.T) {
	parsed, target := parseFileTarget(t, `\\server\share\a #1.html`)

	if parsed.Scheme != "file" || parsed.Host != "server" {
		t.Fatalf("got scheme %q host %q in %q, want file://server/…", parsed.Scheme, parsed.Host, target)
	}
	if want := "/share/a #1.html"; parsed.Path != want {
		t.Fatalf("path = %q, want %q (%q)", parsed.Path, want, target)
	}
}
