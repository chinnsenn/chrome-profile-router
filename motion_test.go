// [INPUT]: 依赖真实视图、MyGo 离屏时钟与可观察的退出动作。
// [OUTPUT]: 保护短出场的异步完成与输入中断，不启动 Chrome 或修改用户配置。
// [POS]: 根包的内容动效行为回归。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCloseWaitsForContentAnimation(t *testing.T) {
	quits := 0
	app := &selectorApp{themeMode: ThemeLight, quit: func() { quits++ }}
	view := ui.NewTester(app.view, 460, 288)
	before := append([]byte(nil), view.Image().Pix...)
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if quits != 0 {
		t.Fatal("close exited before showing its content animation")
	}
	view.Advance(60 * time.Millisecond)
	if quits != 0 {
		t.Fatal("close blocked or finished before its intermediate frame")
	}
	if bytes.Equal(before, view.Image().Pix) {
		t.Fatal("close waited without drawing an exit animation")
	}
	view.Advance(time.Second)
	if quits != 1 {
		t.Fatalf("settled content must exit once: got %d", quits)
	}
}

func TestCloseDuringNaturalExitKeepsExplicitIntent(t *testing.T) {
	quits := 0
	app := &selectorApp{themeMode: ThemeLight, quit: func() { quits++ }}
	view := ui.NewTester(app.view, 460, 288)
	app.advanceQueue(0)
	view.Frame()
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if quits != 0 {
		t.Fatal("first explicit close must continue the short exit")
	}
	app.enqueue(request{Kind: requestURL, Value: "https://example.invalid/new"})
	view.Frame()
	view.Advance(time.Second)
	if quits != 1 {
		t.Fatalf("new request swallowed explicit close: got %d exits", quits)
	}
}

func TestEntryStartsAgainAfterPreviouslyRenderedContent(t *testing.T) {
	app := &selectorApp{themeMode: ThemeLight}
	app.resetEntry()
	view := ui.NewTester(app.view, 460, 288)
	start := append([]byte(nil), view.Image().Pix...)
	view.Advance(60 * time.Millisecond)
	if bytes.Equal(start, view.Image().Pix) {
		t.Fatal("entry never moved beyond its held initial frame")
	}
	view.Advance(time.Second)
	settled := append([]byte(nil), view.Image().Pix...)
	app.resetEntry()
	view.Frame()
	if !bytes.Equal(start, view.Image().Pix) {
		t.Fatal("show reset did not restore the initial content frame")
	}
	view.Advance(time.Second)
	if !bytes.Equal(settled, view.Image().Pix) {
		t.Fatal("restarted entry did not arrive at the original content")
	}
}

func TestImmediateExitInterruptsContentAnimation(t *testing.T) {
	for _, action := range []string{"Escape", "CmdQ", "CloseAgain"} {
		t.Run(action, func(t *testing.T) {
			quits := 0
			app := &selectorApp{themeMode: ThemeLight, quit: func() { quits++ }}
			view := ui.NewTester(app.view, 460, 288)
			if err := view.Click("关闭"); err != nil {
				t.Fatal(err)
			}
			view.Advance(20 * time.Millisecond)
			switch action {
			case "Escape":
				view.Key(0, ui.KeyEscape)
			case "CmdQ":
				view.Key(ui.Cmd, ui.KeyQ)
			case "CloseAgain":
				if err := view.Click("关闭"); err != nil {
					t.Fatal(err)
				}
			}
			if quits != 1 {
				t.Fatalf("immediate exit waited for animation: got %d", quits)
			}
			view.Advance(time.Second)
			if quits != 1 {
				t.Fatalf("old animation completed another exit: got %d", quits)
			}
		})
	}
}

func TestNewRequestCancelsNaturalExitAndItsOldCompletion(t *testing.T) {
	quits := 0
	app := &selectorApp{themeMode: ThemeLight, quit: func() { quits++ }}
	view := ui.NewTester(app.view, 460, 288)
	app.enqueue(request{Kind: requestURL, Value: "https://example.invalid/first"})
	head, _ := app.current()
	app.advanceQueue(head.ID)
	view.Frame()
	view.Advance(40 * time.Millisecond)
	oldRun := app.motion.run
	app.enqueue(request{Kind: requestURL, Value: "https://example.invalid/new"})
	app.completeExit(oldRun)
	view.Frame()
	view.Advance(time.Second)
	if quits != 0 || app.pendingCount() != 1 {
		t.Fatalf("natural exit consumed a new request: quits=%d pending=%d", quits, app.pendingCount())
	}
	if !view.HasText("example.invalid") {
		t.Fatal("cancelled exit did not render the new target")
	}
	head, _ = app.current()
	app.advanceQueue(head.ID)
	view.Frame()
	view.Advance(time.Second)
	if quits != 1 {
		t.Fatalf("last successful delivery did not exit: got %d", quits)
	}
}

func TestReduceMotionCompletesEntryAndExitImmediately(t *testing.T) {
	quits := 0
	app := &selectorApp{themeMode: ThemeLight, quit: func() { quits++ }}
	view := ui.NewTester(app.view, 460, 288)
	stable := append([]byte(nil), view.Image().Pix...)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	app.resetEntry()
	view.Frame()
	if !bytes.Equal(stable, view.Image().Pix) {
		t.Fatal("reduced motion retained entry geometry or transparency")
	}
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	if quits != 1 {
		t.Fatalf("reduced motion exit waited for animation: got %d", quits)
	}
}

// TestMotionSnapshots 可选保存离屏真实视图，不接触 Chrome 或原生窗口。
func TestMotionSnapshots(t *testing.T) {
	dir := os.Getenv("CHROME_ROUTER_MOTION_ARTIFACTS")
	if dir == "" {
		t.Skip("set CHROME_ROUTER_MOTION_ARTIFACTS to save motion frames")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	quits := 0
	app := &selectorApp{
		themeMode: ThemeLight, quit: func() { quits++ },
		profiles: []Profile{
			{Dir: "Default", Name: "Motion Personal", Email: "personal@example.invalid", Color: ui.Hex("#3686E8")},
			{Dir: "Profile 1", Name: "Motion Work", Email: "work@example.invalid", Color: ui.Hex("#42A389")},
		},
	}
	app.resetEntry()
	view := ui.NewTester(app.view, 460, 264)
	save := func(name string) {
		t.Helper()
		path := filepath.Join(dir, name+".png")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, view.Image())
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("save %s: encode=%v close=%v", path, err, closeErr)
		}
		t.Logf("saved %s at %s", path, view.Now().Format(time.RFC3339Nano))
	}
	save("entry-start")
	view.Advance(60 * time.Millisecond)
	save("entry-middle")
	view.Advance(time.Second)
	save("ready")
	if err := view.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	view.Advance(60 * time.Millisecond)
	save("exit-middle")
	view.Advance(time.Second)
	save("exit-complete")
	if quits != 1 {
		t.Fatalf("smoke exit count = %d, want 1", quits)
	}
}
