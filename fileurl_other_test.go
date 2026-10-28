// [INPUT]: 依赖 fileTarget 对 POSIX 绝对路径的输出。
// [OUTPUT]: 保护 POSIX 的 file URL 组成：host 为空、路径逐段保真（空格/#/?/% 都只是路径字符），查询串与片段不被伪造。
// [POS]: 非 Windows 上 fileTarget 的行为边界；Windows 的盘符与 UNC 在 fileurl_windows_test.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
//go:build !windows

package main

import (
	"net/url"
	"testing"
)

func TestFileTargetPreservesSpecialCharacters(t *testing.T) {
	path := "/tmp/日本語 #report?100%.html"
	target, err := fileTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" || parsed.Path != path || parsed.RawQuery != "" || parsed.Fragment != "" {
		t.Fatalf("file path changed meaning: %q -> %#v", path, parsed)
	}
}
