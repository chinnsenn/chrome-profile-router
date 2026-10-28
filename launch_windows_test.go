// [INPUT]: 依赖 launchChrome 的 windows 分支、LOCALAPPDATA 下的假 chrome.exe（测试二进制副本）与 helper 的 argv 落盘。
// [OUTPUT]: 保护 Windows 交付契约：按安装位置找到 chrome.exe、profile 目录与目标 URL 原样到达子进程（含中文、空格、& 与 #）、缺 Chrome 时不启动。
// [POS]: Windows 启动路径的端到端回归，子进程是真实进程但可断言 argv。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeChromeInLocalAppData 把测试二进制复制成 per-user 安装位置的 chrome.exe。
func fakeChromeInLocalAppData(t *testing.T) {
	t.Helper()
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", filepath.Join(local, "none-pf"))
	t.Setenv("ProgramFiles(x86)", filepath.Join(local, "none-pfx"))

	data, err := os.ReadFile(helperBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(local, "Google", "Chrome", "Application", "chrome.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchChromeDeliversProfileAndTarget(t *testing.T) {
	fakeChromeInLocalAppData(t)
	out := filepath.Join(t.TempDir(), "argv.json")
	t.Setenv(helperModeEnv, "argv")
	t.Setenv(helperOutEnv, out)

	const target = "https://例子.test/a?b=1&c=2#frag"
	if err := launchChrome("工作 配置", target); err != nil {
		t.Fatal(err)
	}

	argv := readHelperArgv(t, out)
	if len(argv) != 2 {
		t.Fatalf("Chrome got %d arguments: %#v", len(argv), argv)
	}
	if argv[0] != "--profile-directory=工作 配置" {
		t.Fatalf("profile argument = %q", argv[0])
	}
	if argv[1] != target {
		t.Fatalf("target = %q, want %q", argv[1], target)
	}
}

func TestLaunchChromeKeepsLocalFileTargetIntact(t *testing.T) {
	fakeChromeInLocalAppData(t)
	out := filepath.Join(t.TempDir(), "argv.json")
	t.Setenv(helperModeEnv, "argv")
	t.Setenv(helperOutEnv, out)

	const localFile = `C:\Users\报 告\Desktop\a #1.html`
	target, err := fileTarget(localFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := launchChrome("Default", target); err != nil {
		t.Fatal(err)
	}

	argv := readHelperArgv(t, out)
	if len(argv) != 2 {
		t.Fatalf("Chrome got %d arguments: %#v", len(argv), argv)
	}
	// 用 url.Parse 独立解码交给 Chrome 的那个参数：盘符路径必须是
	// file:///C:/…，反斜线绝不能出现在 URL 里。
	parsed, err := url.Parse(argv[1])
	if err != nil {
		t.Fatalf("file target %q does not parse: %v", argv[1], err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" {
		t.Fatalf("file target = %q, want file:///C:/…", argv[1])
	}
	if want := "/C:/Users/报 告/Desktop/a #1.html"; parsed.Path != want {
		t.Fatalf("path = %q, want %q (%q)", parsed.Path, want, argv[1])
	}
	if strings.Contains(argv[1], `\`) || strings.Contains(argv[1], "%5C") {
		t.Fatalf("file target keeps a backslash: %q", argv[1])
	}
}

func TestLaunchChromeReportsMissingChrome(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", filepath.Join(local, "none-pf"))
	t.Setenv("ProgramFiles(x86)", filepath.Join(local, "none-pfx"))
	if err := launchChrome("Default", "https://example.test/"); err == nil {
		t.Fatal("launching without Chrome installed must fail with a readable error")
	}
}

// 以下两条走生产的 openWith 入口：假 chrome.exe 是真进程，断言的是队列的
// 交付状态，不手动推进任何一步。

func TestOpenWithKeepsRequestWhenChromeFails(t *testing.T) {
	fakeChromeInLocalAppData(t)
	t.Setenv(helperModeEnv, "fail")

	app := &selectorApp{}
	app.enqueue(request{Kind: requestURL, Value: "https://example.test/keep"})
	app.openWith(Profile{Dir: "Default"})

	waitFor(t, "the failed launch to be reported", func() bool {
		app.mu.Lock()
		defer app.mu.Unlock()
		return app.launchError != ""
	})

	head, ok := app.current()
	if !ok || head.Value != "https://example.test/keep" {
		t.Fatalf("a failed launch must keep the request for retry, queue = %#v", head)
	}
	if app.launching {
		t.Fatal("a failed launch left the panel in its loading state")
	}
}

func TestOpenWithDeliversRequestToChrome(t *testing.T) {
	fakeChromeInLocalAppData(t)
	t.Setenv(helperModeEnv, "ok")

	app := &selectorApp{}
	app.enqueue(request{Kind: requestURL, Value: "https://example.test/deliver"})
	app.openWith(Profile{Dir: "Default"})

	waitFor(t, "the request to be dequeued", func() bool { return app.pendingCount() == 0 })

	if app.launchError != "" {
		t.Fatalf("a delivered request must not leave an error: %q", app.launchError)
	}
	if app.launching {
		t.Fatal("a delivered request left the panel in its loading state")
	}
}
