// [INPUT]: 依赖 MyGo 离屏视图输入、selectorApp 的真实选择状态、settings.go 的默认设置与 layout.go 的设置面板预算。
// [OUTPUT]: 保护混合输入、九身份同屏、网格边界与窄屏导航，以及设置面板的入口、Esc 分层、面板期间的键盘屏蔽、滑块/开关/恢复默认的真实写入、固定高度与动效开关的收敛行为。
// [POS]: 根包的交互回归入口，不启动 Chrome 或改写用户配置（设置写入一律走临时目录或内存）。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo"
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

// ============================================================================
// 设置面板
// ============================================================================

// settingsProfiles 造出 count 个身份，供设置面板的交互回归使用。
func settingsProfiles(count int) []Profile {
	out := make([]Profile, 0, count)
	for i := range count {
		out = append(out, Profile{
			Dir:   fmt.Sprintf("Profile %d", i),
			Name:  fmt.Sprintf("Identity %d", i+1),
			Email: fmt.Sprintf("account%d@example.com", i+1),
			Color: ui.Hex("#3686E8"),
		})
	}
	return out
}

// settingsApp 造一个已经读过默认设置的应用：三个身份、浅色主题。settingsWritable
// 为 false，因此任何写盘尝试都会被跳过——回归测试绝不碰用户真实的
// settings.json。
func settingsApp(quit func()) *selectorApp {
	return &selectorApp{
		themeMode: ThemeLight,
		settings:  defaultSettings(),
		profiles:  settingsProfiles(3),
		quit:      quit,
	}
}

// settingsTester 用 viewWith 渲染真实视图，并把启动动作换成记录回调：设置
// 面板期间的"误启动"能被断言，而不会真的拉起 Chrome。
//
// 窗口带材质（SetVibrancy(true)），因为只有那样外观三项才是可调的——真实
// 窗口就是这种状态；无材质/降低透明度下的置灰另有专门回归。
func settingsTester(app *selectorApp, launched *[]string, w, h int) *ui.Tester {
	view := ui.NewTester(func(c *ui.Context) {
		app.viewWith(c, func(p Profile) { *launched = append(*launched, p.Name) })
	}, w, h)
	view.SetVibrancy(true)
	return view
}

// TestSettingsEscapeClosesPanelBeforeQuitting 约束 Esc 分层：面板打开时 Esc
// 只关闭面板并回到列表，面板已关时第二次 Esc 才退出应用。
func TestSettingsEscapeClosesPanelBeforeQuitting(t *testing.T) {
	quits := 0
	app := settingsApp(func() { quits++ })
	var launched []string
	view := settingsTester(app, &launched, 460, 490)

	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !app.settingsOpenState() {
		t.Fatal("齿轮没有打开设置面板")
	}
	if !view.HasText("背景浓度") {
		t.Fatal("设置面板没有渲染出来")
	}

	view.Key(0, ui.KeyEscape)
	if quits != 0 {
		t.Fatalf("第一次 Esc 退出了应用，应该只关闭面板: quits=%d", quits)
	}
	if app.settingsOpenState() {
		t.Fatal("第一次 Esc 没有关闭设置面板")
	}
	if !view.HasText("Identity 1") {
		t.Fatal("关闭面板后没有回到身份列表")
	}

	view.Key(0, ui.KeyEscape)
	if quits != 1 {
		t.Fatalf("面板已关时 Esc 必须退出应用: quits=%d", quits)
	}
}

// TestSettingsPanelBlocksLaunchKeys 约束面板期间的键盘屏蔽：数字键、回车与
// ⌘C 都不启动身份、不复制，也不改写列表选择；新链接照常入队并在地址卡显示
// 剩余条数。
func TestSettingsPanelBlocksLaunchKeys(t *testing.T) {
	app := settingsApp(nil)
	var launched []string
	view := settingsTester(app, &launched, 460, 490)

	// 先把选中项挪到第二项：数字键 1 若未被吞掉，会同时改选择并启动第一项。
	app.setChosen(1)
	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}

	view.Key(0, ui.Key1)
	view.Key(0, ui.KeyEnter)
	view.Key(0, ui.KeyRight)
	view.Key(ui.Cmd, ui.KeyC)

	if len(launched) != 0 {
		t.Fatalf("设置面板打开时启动了身份: %v", launched)
	}
	if app.chosen != 1 {
		t.Fatalf("设置面板打开时键盘改写了列表选择: got %d, want 1", app.chosen)
	}
	if view.HasText("已复制") || view.HasText("复制失败") {
		t.Fatal("设置面板打开时 ⌘C 仍然复制了链接")
	}

	// 新链接照常入队，并在地址卡显示剩余条数。
	app.enqueue(request{Kind: requestURL, Value: "https://example.invalid/one"})
	app.enqueue(request{Kind: requestURL, Value: "https://example.invalid/two"})
	view.Frame()
	if !view.HasText("example.invalid") {
		t.Fatal("设置面板打开时地址卡没有显示新链接")
	}
	if !view.HasText("剩余 2 条") {
		t.Fatal("设置面板打开时地址卡没有显示剩余条数")
	}
	if app.pendingCount() != 2 {
		t.Fatalf("面板期间的入队丢失了请求: pending=%d", app.pendingCount())
	}
}

// TestSettingsGearAndCommandCommaTogglePanel 约束入口：齿轮点击与视图内 ⌘,
// 都能打开/关闭面板，且不新增窗口。
func TestSettingsGearAndCommandCommaTogglePanel(t *testing.T) {
	app := settingsApp(nil)
	var launched []string
	view := settingsTester(app, &launched, 460, 490)

	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !app.settingsOpenState() {
		t.Fatal("点击齿轮没有打开面板")
	}
	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if app.settingsOpenState() {
		t.Fatal("再次点击齿轮没有关闭面板")
	}

	view.Key(ui.Cmd, ui.KeyComma)
	if !app.settingsOpenState() {
		t.Fatal("⌘, 没有打开面板")
	}
	view.Key(ui.Cmd, ui.KeyComma)
	if app.settingsOpenState() {
		t.Fatal("⌘, 没有关闭面板")
	}
}

// TestSettingsPanelKeyboardAdjustsSlider 约束面板可键盘操作：Tab 进入控件、
// 方向键按步长调值，且滑块写的是当前外观那一侧。
func TestSettingsPanelKeyboardAdjustsSlider(t *testing.T) {
	app := settingsApp(nil)
	var launched []string
	view := settingsTester(app, &launched, 460, 490)
	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}

	// 头部只有可点击控件、没有可获得焦点的控件，因此第一个 Tab 落在面板的
	// 第一个控件（背景浓度滑块）上。
	view.Key(0, ui.KeyTab)
	view.Key(0, ui.KeyRight)

	// 浅色主题写浅色那一侧：0.34 + 0.02。
	want := quantizeTint(0.34+tintStep, backgroundTintMin, backgroundTintMax)
	if got := app.settings.Appearance.BackgroundTint.Light; got != want {
		t.Fatalf("方向键没有按步长调值: got %.4f, want %.4f", got, want)
	}
	if got := app.settings.Appearance.BackgroundTint.Dark; got != defaultAppearance().BackgroundTint.Dark {
		t.Fatalf("滑块改到了另一侧外观: dark=%.4f", got)
	}
	if !view.HasText(formatTint(want)) {
		t.Fatalf("面板没有显示新的读数 %.2f", want)
	}
}

// TestSettingsResetRestoresDefaultsLive 约束恢复默认：把非默认值写回默认外观
// （含材质与四个 tint）与默认动效，并立即生效。
func TestSettingsResetRestoresDefaultsLive(t *testing.T) {
	app := settingsApp(nil)
	app.settings.Appearance = appearanceSettings{
		Theme:          "auto",
		Material:       "hud",
		BackgroundTint: tintPair{Dark: 0.10, Light: 0.12},
		CardTint:       tintPair{Dark: 0.90, Light: 0.92},
	}
	app.settings.Motion = motionSettings{Spin: false, Hover: false, Tilt: false, ReduceMotion: motionReduceOn}
	var launched []string
	view := settingsTester(app, &launched, 460, 490)

	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("0.12") {
		t.Fatal("面板没有显示非默认的浓度")
	}
	if err := view.Click("恢复默认"); err != nil {
		t.Fatal(err)
	}

	if got := app.settings.Appearance; got != defaultAppearance() {
		t.Fatalf("恢复默认没有把外观写回默认值: %+v", got)
	}
	if got := app.settings.Motion; got != defaultMotion() {
		t.Fatalf("恢复默认没有把动效写回默认值: %+v", got)
	}
	if !view.HasText("0.34") || !view.HasText("0.66") {
		t.Fatal("恢复默认后读数没有回到默认值")
	}
}

// TestSettingsPanelFitsFixedHeight 约束面板高度是固定预算：真实窗口尺寸下
// 所有控件都在窗口内，最后一项不需要滚动就能看到。
func TestSettingsPanelFitsFixedHeight(t *testing.T) {
	app := settingsApp(nil)
	var launched []string
	area := mygo.Rectangle{Width: 1440, Height: 900}
	w, h := settingsPanelSize(len(app.profiles), area)
	view := settingsTester(app, &launched, w, h)

	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	box, ok := view.Find("恢复默认")
	if !ok || box.H <= 0 {
		t.Fatalf("恢复默认控件没有画出来: ok=%v box=%+v", ok, box)
	}
	if box.Y+box.H > float32(h) {
		t.Fatalf("设置面板内容超出固定高度: bottom=%.1f window=%d", box.Y+box.H, h)
	}
	// 关闭后窗口高度回到列表预算：固定面板不会把窗口永久撑高。
	_, listH := profilePanelSize(len(app.profiles), area)
	if listH >= h {
		t.Fatalf("列表预算应当比设置面板矮，才能观察到高度回收: list=%d settings=%d", listH, h)
	}
}

// TestSpinOffStopsAnimationFrames 约束「选中卡流彩」关闭后的代价与观感：不再
// 请求动画帧、两帧同像素，但静态基础选中边框仍在。
func TestSpinOffStopsAnimationFrames(t *testing.T) {
	app := settingsApp(nil)
	app.settings.Motion.Spin = false
	view := ui.NewTester(app.view, 660, 300)
	// 只有窗口真的有材质时流彩才存在（pal.opaque 会挡住它）。
	view.SetVibrancy(true)
	view.Advance(time.Second)
	if view.FramePending() {
		t.Fatal("关掉流彩后视图仍在请求动画帧")
	}
	first := append([]byte(nil), view.Image().Pix...)
	view.Advance(200 * time.Millisecond)
	if !bytes.Equal(first, view.Image().Pix) {
		t.Fatal("关掉流彩后画面仍在变化")
	}

	// 静态边框仍在：卡片表面左边缘的像素与卡内底色不同。
	shell := mustFindRect(t, view, "Identity 1")
	img := view.Image()
	x := int(panelPadding + listInset)
	y := int(shell.Y + shell.H/2)
	border := img.RGBAAt(x, y)
	fill := img.RGBAAt(x+3, y)
	if sameColor(border, fill) {
		t.Fatalf("关掉流彩后静态选中边框消失了: border=%v fill=%v", border, fill)
	}

	// 对照：流彩开着时视图会持续请求动画帧（否则上一条断言证明不了什么）。
	spinning := settingsApp(nil)
	spinView := ui.NewTester(spinning.view, 660, 300)
	spinView.SetVibrancy(true)
	spinView.Advance(time.Second)
	if !spinView.FramePending() {
		t.Fatal("流彩开着时反而不请求动画帧，说明测试没有覆盖到流彩")
	}
}

// TestHoverAndTiltOffKeepRestGeometry 约束「悬停放大与让位」「3D 倾斜」关闭
// 后的几何：指针落在卡片角上时表面与内容仍停在静止槽位（不放大、不倾斜、
// 不视差），而两个开关打开时同一位置必须产生可见差异。
func TestHoverAndTiltOffKeepRestGeometry(t *testing.T) {
	names := []string{"Identity 1", "account1@example.com", "1"}
	probe := func(motion motionSettings) (rest, corner cardProbe) {
		app := settingsApp(nil)
		app.settings.Motion = motion
		rest = probeCards(t, app, 660, 420, 1, 1, names, len(app.profiles))
		corner = probeCards(t, app, 660, 420, rest.shell.X+3, rest.shell.Y+3, names, len(app.profiles))
		return rest, corner
	}

	rest, corner := probe(defaultMotion())
	if nearlySameRect(corner.surf, rest.surf) && nearlySameRect(corner.text, rest.text) {
		t.Fatal("开关全开时指针落在卡片角上没有产生任何几何差异，本测试无法区分关与开")
	}

	off := defaultMotion()
	off.Hover, off.Tilt = false, false
	offRest, offCorner := probe(off)
	if !nearlySameRect(offCorner.surf, offRest.surf) {
		t.Fatalf("关掉悬停放大后卡片表面仍离开了静止槽位: rest=%+v hovered=%+v", offRest.surf, offCorner.surf)
	}
	if !nearlySameRect(offCorner.text, offRest.text) {
		t.Fatalf("关掉倾斜后内容仍有视差位移: rest=%+v hovered=%+v", offRest.text, offCorner.text)
	}
}

// TestSettingsAppearanceControlsDisabledWithoutMaterial 约束无障碍降级：系统
// 「降低透明度」/「提高对比」或窗口没有材质时，界面走不透明表面，外观三项
// 置灰只读，并给出原因说明；动效开关仍然可调，也没有"忽略系统设置"的开关。
func TestSettingsAppearanceControlsDisabledWithoutMaterial(t *testing.T) {
	app := settingsApp(nil)
	var launched []string
	// 不带 SetVibrancy：窗口没有材质，pal.opaque 成立。
	view := ui.NewTester(func(c *ui.Context) {
		app.viewWith(c, func(p Profile) { launched = append(launched, p.Name) })
	}, 460, 490)

	if err := view.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !view.HasText("不透明表面") {
		t.Fatal("外观置灰时没有给出原因说明")
	}

	// 外观三项退出焦点顺序：第一个 Tab 落在动效的第一个开关上。
	view.Key(0, ui.KeyTab)
	view.Key(0, ui.KeySpace)
	if app.settings.Motion.Spin {
		t.Fatal("外观置灰后第一个 Tab 没有落到动效开关上")
	}
	// 置灰的材质档位点不动。
	if err := view.Click("hud"); err != nil {
		t.Fatal(err)
	}
	if got := app.settings.Appearance; got != defaultAppearance() {
		t.Fatalf("外观置灰后仍被改动: %+v", got)
	}
	// 没有"忽略系统设置"的开关。
	if view.HasText("忽略") {
		t.Fatal("面板出现了忽略系统设置的开关")
	}
}

// TestForcedReduceMotionMatchesSystemReduce 约束「减少动态效果：强制开启」：
// 它与系统开启 ReduceMotion 走同一条归零通路——入场没有缩放与位移、流彩
// 角度不推进（因此没有空转的动画帧），界面直接落在终态。
func TestForcedReduceMotionMatchesSystemReduce(t *testing.T) {
	app := settingsApp(nil)
	app.settings.Motion.ReduceMotion = motionReduceOn
	view := ui.NewTester(app.view, 460, 300)
	view.SetVibrancy(true)
	view.Advance(time.Second)
	stable := append([]byte(nil), view.Image().Pix...)
	if view.FramePending() {
		t.Fatal("强制减少动态效果后仍在请求动画帧")
	}

	app.resetEntry()
	view.Frame()
	if !bytes.Equal(stable, view.Image().Pix) {
		t.Fatal("强制减少动态效果后入场仍有几何位移或透明度变化")
	}

	// 系统偏好开着时同样不请求帧（对照组，证明这条通路与系统一致）。
	sys := settingsApp(nil)
	sysView := ui.NewTester(sys.view, 460, 300)
	sysView.SetVibrancy(true)
	sysView.SetPreferences(ui.Preferences{ReduceMotion: true})
	sysView.Advance(time.Second)
	if sysView.FramePending() {
		t.Fatal("系统 ReduceMotion 下仍在请求动画帧")
	}
}
