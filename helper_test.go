// [INPUT]: 依赖测试二进制自身充当假 Chrome 进程，以及 t.Setenv 注入的模式、输出路径与释放文件。
// [OUTPUT]: 提供 TestMain 的 helper 进程分支（argv 落盘 / 非零退出 / 长驻等待释放文件）、构造 helper 命令的 newHelperCommand，以及 stopHelper（等 runDetached 交出的完成信号；释放没送达或超时先杀进程再等同一信号）。
// [POS]: 根包测试的进程边界工具；启动路径因此在真实子进程上被断言，不依赖本机装没装 Chrome，也不留后台进程——长驻 helper 的退出由释放文件触发、由唯一后台 Wait 的完成信号确认，测试结束前子进程必已被回收。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// helper 进程用这三个变量控制行为：模式、argv 落盘路径、长驻进程的释放文件。
const (
	helperModeEnv    = "CPR_TEST_HELPER"
	helperOutEnv     = "CPR_TEST_HELPER_OUT"
	helperReleaseEnv = "CPR_TEST_HELPER_RELEASE"
)

// helperHoldLimit 是长驻 helper 的自毁上限：正常路径由 t.Cleanup 释放并以
// 完成信号确认回收，不依赖这个上限；只有释放信号没能送达（测试异常退出、释放
// 文件写不进去）时才靠它收尾，让子进程最迟在 20s 内自己结束。
const helperHoldLimit = 20 * time.Second

// helperExitTimeout 是等 helper 被回收的上限。它等的是真实进程状态（唯一后台
// Wait 的完成信号），不是用来猜退出时机的固定时长。
const helperExitTimeout = 5 * time.Second

// TestMain 让测试二进制在 helper 模式下扮演 Chrome：这是唯一能同时覆盖
// "参数怎么到达子进程"与"进程怎么退出/长驻"的办法。
func TestMain(m *testing.M) {
	switch os.Getenv(helperModeEnv) {
	case "argv":
		if out := os.Getenv(helperOutEnv); out != "" {
			data, err := json.Marshal(os.Args[1:])
			if err == nil {
				_ = os.WriteFile(out, data, 0o644)
			}
		}
		os.Exit(0)
	case "ok":
		os.Exit(0)
	case "fail":
		os.Exit(3)
	case "hold":
		release := os.Getenv(helperReleaseEnv)
		deadline := time.Now().Add(helperHoldLimit)
		for time.Now().Before(deadline) {
			if release != "" {
				if _, err := os.Stat(release); err == nil {
					os.Exit(0)
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// newHelperCommand 把测试二进制再启动成指定行为的假 Chrome。
func newHelperCommand(t *testing.T, mode string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), helperModeEnv+"="+mode)
	return cmd
}

// helperBinary 返回当前测试二进制的绝对路径，供假 chrome 使用。
func helperBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// stopHelper 让长驻 helper 结束并等它被唯一的后台 Wait 回收：读到完成信号即
// 子进程已经不在，测试可以结束（Windows 上残留的子进程会锁住测试二进制，go
// 随后删不掉它）。released=false（释放信号没送出去）或释放超时都先杀掉它，再
// 等同一个完成信号——绝不另开第二个 Wait 所有者。
func stopHelper(t *testing.T, cmd *exec.Cmd, done <-chan error, released bool) {
	t.Helper()
	if released {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("the fake Chrome did not exit cleanly: %v", err)
			}
			return
		case <-time.After(helperExitTimeout):
			t.Errorf("the fake Chrome ignored its release for %v", helperExitTimeout)
		}
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("killing the fake Chrome: %v", err)
	}
	select {
	case err := <-done:
		// 被杀的进程必然以非零码退出，这是兜底路径，不是缺陷。
		t.Logf("the fake Chrome had to be killed (%v)", err)
	case <-time.After(helperExitTimeout):
		t.Fatalf("the fake Chrome was still running %v after it was killed", helperExitTimeout)
	}
}

// waitFor 等一个真实异步结果出现：条件是应用状态本身，不是固定时长。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
