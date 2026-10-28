// [INPUT]: 依赖 darwin 分支的 chromeDataDir 与 HOME。
// [OUTPUT]: 保护 macOS 既有路径：Chrome 用户数据仍在 ~/Library/Application Support/Google/Chrome。
// [POS]: macOS 平台路径的回归，确保跨平台改动不动已发布行为。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"path/filepath"
	"testing"
)

func TestChromeDataDirDarwinKeepsApplicationSupport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := chromeDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "Library", "Application Support", "Google", "Chrome"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
