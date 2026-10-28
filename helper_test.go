// [INPUT]: 依赖测试二进制自身充当假 Chrome 进程，以及 t.Setenv 注入的模式、输出路径与释放文件。
// [OUTPUT]: 提供 TestMain 的 helper 进程分支（argv 落盘 / 非零退出 / 长驻等待）与构造 helper 命令的 newHelperCommand。
// [POS]: 根包测试的进程边界工具；启动路径因此在真实子进程上被断言，不依赖本机装没装 Chrome，也不留后台进程。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
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

// helperHoldLimit 是长驻 helper 的自毁上限：测试忘了写释放文件时也不会把
// 进程留到整轮测试之后。
const helperHoldLimit = 20 * time.Second

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
