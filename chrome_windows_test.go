// [INPUT]: 依赖 windows 分支的 chromeDataDir / chromeExecutable 与 LOCALAPPDATA、ProgramFiles、ProgramFiles(x86)。
// [OUTPUT]: 保护 Windows 的信任边界：用户数据只在 LOCALAPPDATA 下解析，chrome.exe 按 per-user 优先的固定顺序查找，缺失时列出已查找路径。
// [POS]: Windows 平台路径决策的行为边界回归，只改环境变量，不读注册表。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChromeDataDirWindowsUsesLocalAppData(t *testing.T) {
	local := `C:\Users\tester\AppData\Local`
	t.Setenv("LOCALAPPDATA", local)
	got, err := chromeDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(local, "Google", "Chrome", "User Data"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	t.Setenv("LOCALAPPDATA", "")
	if _, err := chromeDataDir(); err == nil || !strings.Contains(err.Error(), "LOCALAPPDATA") {
		t.Fatalf("missing LOCALAPPDATA must be reported, got %v", err)
	}
}

// installChromeAt 在 root 下造一份 chrome.exe，返回它的路径。
func installChromeAt(t *testing.T, root string) string {
	t.Helper()
	exe := filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestChromeExecutableWindowsPrefersPerUserInstall(t *testing.T) {
	tmp := t.TempDir()
	local, pf, pfx := filepath.Join(tmp, "local"), filepath.Join(tmp, "pf"), filepath.Join(tmp, "pfx")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", pf)
	t.Setenv("ProgramFiles(x86)", pfx)

	// 只有 Program Files (x86) 装了 Chrome：最后一级回落必须能找到它。
	wantPFX := installChromeAt(t, pfx)
	got, err := chromeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPFX {
		t.Fatalf("got %q, want the Program Files (x86) install %q", got, wantPFX)
	}

	// 再加 Program Files：它排在 (x86) 前面。
	wantPF := installChromeAt(t, pf)
	got, err = chromeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != wantPF {
		t.Fatalf("got %q, want the Program Files install %q", got, wantPF)
	}

	// 最后加 per-user 安装：当前用户的安装优先于两个系统位置。
	wantLocal := installChromeAt(t, local)
	got, err = chromeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != wantLocal {
		t.Fatalf("got %q, want the per-user install %q", got, wantLocal)
	}
}

func TestChromeExecutableWindowsReportsMissingChrome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(tmp, "local"))
	t.Setenv("ProgramFiles", filepath.Join(tmp, "pf"))
	t.Setenv("ProgramFiles(x86)", filepath.Join(tmp, "pfx"))
	_, err := chromeExecutable()
	if err == nil {
		t.Fatal("a missing Chrome must be reported, not launched")
	}
	for _, root := range []string{"local", "pf", "pfx"} {
		if !strings.Contains(err.Error(), root) {
			t.Fatalf("error %q does not list the searched location %q", err, root)
		}
	}
}
