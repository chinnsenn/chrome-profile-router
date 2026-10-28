// [INPUT]: 依赖 launchChrome 的 linux 分支、PATH 里的假 google-chrome（测试二进制副本）与 helper 的 argv 落盘。
// [OUTPUT]: 保护 Linux 交付契约：从 PATH 找到 Chrome、profile 目录与目标 URL 原样到达子进程（含中文、空格、& 与 #）、缺 Chrome 时不启动。
// [POS]: Linux 启动路径的端到端回归，子进程是真实进程但可断言 argv。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeChromeOnPath 把一个名为 google-chrome 的测试二进制放进临时目录并指向 PATH。
func fakeChromeOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(helperBinary(t), filepath.Join(dir, "google-chrome")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestLaunchChromeDeliversProfileAndTarget(t *testing.T) {
	fakeChromeOnPath(t)
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
	fakeChromeOnPath(t)
	out := filepath.Join(t.TempDir(), "argv.json")
	t.Setenv(helperModeEnv, "argv")
	t.Setenv(helperOutEnv, out)

	target, err := fileTarget("/tmp/报 告 #1.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := launchChrome("Default", target); err != nil {
		t.Fatal(err)
	}

	argv := readHelperArgv(t, out)
	if len(argv) != 2 || argv[1] != target {
		t.Fatalf("file target reached Chrome as %#v, want %q", argv, target)
	}
}

func TestLaunchChromeReportsMissingChrome(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := launchChrome("Default", "https://example.test/"); err == nil {
		t.Fatal("launching without Chrome installed must fail with a readable error")
	}
}

// 以下两条走生产的 openWith 入口：假 chrome 是真进程，断言的是队列的
// 交付状态，不手动推进任何一步。

func TestOpenWithKeepsRequestWhenChromeFails(t *testing.T) {
	fakeChromeOnPath(t)
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
	fakeChromeOnPath(t)
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
