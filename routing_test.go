// [INPUT]: 依赖 selectorApp 请求队列与启动收尾。
// [OUTPUT]: 保护启动期间新请求与失败重试的用户可见契约；文件 URL 编码在 fileurl_<平台>_test.go。
// [POS]: 根包的路由边界回归，不执行 Chrome 或原生退出。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"errors"
	"testing"
)

func TestHomeLaunchKeepsRequestsArrivingDuringLaunch(t *testing.T) {
	app := &selectorApp{launching: true}
	app.enqueue(request{Kind: requestURL, Value: "https://example.com/arrived"})
	app.finishLaunch(nil)
	app.advanceQueue(0)
	head, ok := app.current()
	if !ok || head.Value != "https://example.com/arrived" {
		t.Fatalf("home launch consumed a later external request: %#v, present=%v", head, ok)
	}
}

func TestLaunchFailurePreservesRequestForRetry(t *testing.T) {
	app := &selectorApp{}
	app.enqueue(request{Kind: requestURL, Value: "https://example.com/first"})
	first, _ := app.current()
	app.launching = true
	app.enqueue(request{Kind: requestURL, Value: "https://example.com/second"})
	app.finishLaunch(errors.New("Chrome launch failed"))
	head, ok := app.current()
	if !ok || head != first || app.launching || app.launchError == "" {
		t.Fatalf("failed launch cannot retry original request: head=%#v launching=%v error=%q", head, app.launching, app.launchError)
	}
	app.finishLaunch(nil)
	app.advanceQueue(first.ID)
	next, ok := app.current()
	if !ok || next.Value != "https://example.com/second" {
		t.Fatalf("successful retry lost next request: %#v, present=%v", next, ok)
	}
}

