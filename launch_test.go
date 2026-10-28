// [INPUT]: 依赖 runDetached 与 helper 测试进程（argv 落盘 / 非零退出 / 长驻），以及请求队列的交付状态。
// [OUTPUT]: 保护启动语义：立即失败可见、干净早退算成功、浏览器长驻时启动动作不等待其生命周期、失败不出队。
// [POS]: 启动握手的行为边界回归，不执行本机 Chrome。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunDetachedReportsImmediateFailure(t *testing.T) {
	err := runDetached(newHelperCommand(t, "fail"), 2*time.Second)
	if err == nil {
		t.Fatal("a Chrome that exits with an error right after start must be reported")
	}
}

func TestRunDetachedAcceptsCleanEarlyExit(t *testing.T) {
	if err := runDetached(newHelperCommand(t, "ok"), 2*time.Second); err != nil {
		t.Fatalf("a clean exit means the running Chrome took the request: %v", err)
	}
}

func TestRunDetachedReturnsWhileChromeKeepsRunning(t *testing.T) {
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv(helperReleaseEnv, release)
	// 无论断言是否失败都释放 helper，绝不留后台进程。
	t.Cleanup(func() {
		if err := os.WriteFile(release, nil, 0o644); err != nil {
			t.Errorf("releasing the fake Chrome: %v", err)
		}
	})

	start := time.Now()
	err := runDetached(newHelperCommand(t, "hold"), 200*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("a Chrome that keeps running is a successful launch: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("launch waited %v for a browser that keeps running", elapsed)
	}
}

// readHelperArgv 读取假 Chrome 落盘的 argv。
func readHelperArgv(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fake Chrome recorded no argv: %v", err)
	}
	var argv []string
	if err := json.Unmarshal(data, &argv); err != nil {
		t.Fatalf("fake Chrome recorded %q: %v", data, err)
	}
	return argv
}
