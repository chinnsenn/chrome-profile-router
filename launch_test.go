// [INPUT]: 依赖 runDetached 的启动结果与完成信号、helper 测试进程（argv 落盘 / 非零退出 / 长驻），以及请求队列的交付状态。
// [OUTPUT]: 保护启动语义：立即失败可见、干净早退算成功、浏览器长驻时启动动作不等待其生命周期（返回时子进程仍活、清理时等完成信号确认已被回收）、失败不出队。
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
	if _, err := runDetached(newHelperCommand(t, "fail"), 2*time.Second); err == nil {
		t.Fatal("a Chrome that exits with an error right after start must be reported")
	}
}

func TestRunDetachedAcceptsCleanEarlyExit(t *testing.T) {
	if _, err := runDetached(newHelperCommand(t, "ok"), 2*time.Second); err != nil {
		t.Fatalf("a clean exit means the running Chrome took the request: %v", err)
	}
}

func TestRunDetachedReturnsWhileChromeKeepsRunning(t *testing.T) {
	release := filepath.Join(t.TempDir(), "release")
	t.Setenv(helperReleaseEnv, release)

	const grace = 200 * time.Millisecond
	cmd := newHelperCommand(t, "hold")
	start := time.Now()
	done, err := runDetached(cmd, grace)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("a Chrome that keeps running is a successful launch: %v", err)
	}
	// 无论断言是否失败都释放 helper，并等唯一后台 Wait 的完成信号：Windows 上
	// 残留的子进程会锁住测试二进制，go 随后删不掉它。这条清理注册在 t.TempDir
	// 的删除之前，所以释放文件在整个等待期间都还在。释放文件写不进去时不能就此
	// 返回——记下错误、改成杀进程，仍然等同一个完成信号。
	t.Cleanup(func() {
		released := true
		if err := os.WriteFile(release, nil, 0o644); err != nil {
			t.Errorf("releasing the fake Chrome: %v", err)
			released = false
		}
		stopHelper(t, cmd, done, released)
	})

	if elapsed < grace {
		t.Fatalf("launch returned after %v, before the %v observation window: the fake Chrome had already exited", elapsed, grace)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("launch waited %v for a browser that keeps running", elapsed)
	}
	// 观察窗结束时子进程必须还活着：完成信号没到，说明后台 Wait 还在等它。
	select {
	case werr := <-done:
		t.Fatalf("the fake Chrome had already exited when launch returned (%v)", werr)
	default:
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
