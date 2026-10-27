// [INPUT]: 依赖 MyGo 离屏视图输入与 selectorApp 的真实选择状态。
// [OUTPUT]: 保护混合输入、九身份同屏、网格边界与窄屏导航的行为回归。
// [POS]: 根包的交互回归入口，不启动 Chrome 或改写用户配置。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
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

// TestNineProfilesFitAndNavigateAsGrid 约束九身份同屏和方向键的空间对应关系。
func TestNineProfilesFitAndNavigateAsGrid(t *testing.T) {
	app := &selectorApp{themeMode: ThemeLight}
	for i := 0; i < 9; i++ {
		app.profiles = append(app.profiles, Profile{
			Dir: fmt.Sprintf("Profile %d", i), Name: fmt.Sprintf("Identity %d", i+1),
			Email: fmt.Sprintf("account%d@example.com", i+1), Color: ui.Hex("#3686E8"),
		})
	}
	view := ui.NewTester(app.view, 660, 470)
	view.Move(0, 0)
	view.Advance(time.Second)
	view.Key(0, ui.KeyDown)
	if app.chosen != 3 {
		t.Fatalf("down must select the next grid row: got %d, want 3", app.chosen)
	}
	for _, p := range app.profiles {
		box, ok := view.Find(p.Name)
		if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > 660 || box.Y+box.H > 470 {
			t.Fatalf("profile %q is outside the panel: found=%v bounds=%+v", p.Name, ok, box)
		}
	}
	for _, step := range []struct {
		key  ui.Key
		want int
	}{
		{ui.KeyRight, 4}, {ui.KeyRight, 5}, {ui.KeyRight, 5},
		{ui.KeyDown, 8}, {ui.KeyDown, 8}, {ui.KeyLeft, 7}, {ui.KeyUp, 4},
	} {
		view.Key(0, step.key)
		if app.chosen != step.want {
			t.Fatalf("grid boundary navigation: got %d, want %d", app.chosen, step.want)
		}
	}
	// 末行缺一个格子时，向下选择该行最后一个真实身份。
	app.profiles = app.profiles[:8]
	view.Frame()
	view.Key(0, ui.KeyRight)
	view.Key(0, ui.KeyDown)
	if app.chosen != 7 {
		t.Fatalf("incomplete last row selected an empty cell: got %d", app.chosen)
	}
	// 窄屏改为两列后，键盘必须使用实际列数。
	view.SetSize(480, 560)
	view.Key(0, ui.KeyUp)
	if app.chosen != 5 {
		t.Fatalf("narrow grid used stale column count: got %d, want 5", app.chosen)
	}
	first, ok := view.Find(app.profiles[0].Name)
	if !ok {
		t.Fatal("first profile missing after resize")
	}
	view.Move(first.X+first.W/2, first.Y+first.H/2)
	view.Advance(time.Second)
	view.Key(0, ui.KeyDown)
	view.Advance(time.Second)
	if app.chosen != 2 {
		t.Fatalf("stationary pointer overwrote grid navigation: got %d, want 2", app.chosen)
	}
}
