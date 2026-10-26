// [INPUT]: 依赖 MyGo 离屏视图输入与 selectorApp 的真实选择状态。
// [OUTPUT]: 保护键盘与鼠标混合操作的行为回归。
// [POS]: 根包的交互回归入口，不启动 Chrome 或改写用户配置。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestKeyboardSelectionSurvivesStationaryPointer 约束混合输入：静止的指针不能撤销键盘选择。
func TestKeyboardSelectionSurvivesStationaryPointer(t *testing.T) {
	app := &selectorApp{profiles: []Profile{
		{Dir: "Default", Name: "Personal", Color: ui.Hex("#3686E8")},
		{Dir: "Profile 1", Name: "Work", Color: ui.Hex("#42A389")},
		{Dir: "Profile 2", Name: "Testing", Color: ui.Hex("#9271D8")},
	}, themeMode: ThemeLight}
	view := ui.NewTester(app.view, 660, 490)
	box, ok := view.Find("Personal")
	if !ok {
		t.Fatal("first profile is not visible")
	}
	view.Move(box.X+box.W/2, box.Y+box.H/2)
	view.Advance(time.Second)
	view.Key(0, ui.KeyRight)
	view.Advance(time.Second)
	if app.chosen != 1 {
		t.Fatalf("stationary pointer overwrote keyboard choice: got %d, want 1", app.chosen)
	}
}
