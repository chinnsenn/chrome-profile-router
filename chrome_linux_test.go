// [INPUT]: 依赖 linux 分支的 chromeDataDir / chromeExecutable 与 PATH、XDG_CONFIG_HOME、HOME。
// [OUTPUT]: 保护 Linux 的信任边界：配置目录按 XDG 优先级解析，Chrome 只从 PATH 的 Stable 名字解析，缺失时给出可读错误。
// [POS]: Linux 平台路径决策的行为边界回归，只改环境变量，不加产品专用开关。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChromeDataDirLinuxFollowsXDGConfigHome(t *testing.T) {
	t.Setenv("HOME", "/home/tester")

	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := chromeDataDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/xdg", "google-chrome"); got != want {
		t.Fatalf("XDG_CONFIG_HOME set: got %q, want %q", got, want)
	}

	// 空值与相对路径都按未设置处理（XDG 规范：相对路径必须被忽略）。
	for _, unset := range []string{"", "relative/config"} {
		t.Setenv("XDG_CONFIG_HOME", unset)
		got, err := chromeDataDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join("/home/tester", ".config", "google-chrome"); got != want {
			t.Fatalf("XDG_CONFIG_HOME=%q: got %q, want %q", unset, got, want)
		}
	}
}

func TestChromeExecutableLinuxUsesPath(t *testing.T) {
	dir := t.TempDir()
	stable := filepath.Join(dir, "google-chrome-stable")
	if err := os.WriteFile(stable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := chromeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != stable {
		t.Fatalf("got %q, want %q", got, stable)
	}

	// google-chrome 是 Stable 的主命令名，两个都在时它优先。
	primary := filepath.Join(dir, "google-chrome")
	if err := os.WriteFile(primary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = chromeExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != primary {
		t.Fatalf("got %q, want the primary name %q", got, primary)
	}
}

func TestChromeExecutableLinuxReportsMissingChrome(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := chromeExecutable()
	if err == nil {
		t.Fatal("a missing Chrome must be reported, not launched")
	}
	if !strings.Contains(err.Error(), "Chrome") {
		t.Fatalf("error %q does not name Chrome", err)
	}
}
