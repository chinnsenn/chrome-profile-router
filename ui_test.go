// [INPUT]: 依赖 MyGo 离屏视图输入、selectorApp 的真实选择状态、settings.go 的默认设置与 layout.go 的设置面板预算。
// [OUTPUT]: 保护混合输入、九身份同屏、网格边界与窄屏导航，以及设置面板的入口、Esc 分层、面板期间的键盘屏蔽、滑块/开关/恢复默认的真实写入、固定高度与动效开关的收敛行为；启动反馈用像素与精确文本断言约束（地址卡与选中卡副行都是强调色、指示器是画出来的强调色像素而不是只有无障碍 Label、ReduceMotion 下静态表达且推进时钟后像素不变、点击非选中卡时选择与被交付卡一致），并在 CPR_DUMP_LOADING_FRAME=1 时把四档 launching 帧写成 PNG。
// [POS]: 根包的交互回归入口，不启动 Chrome 或改写用户配置（设置写入一律走临时目录或内存）。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
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

// ============================================================================
// 启动中的可见反馈
// ============================================================================

// accentPixelThreshold 判定"这个像素被强调色染过"：强调色（深色 #4C9AFF、
// 浅色 #2563EB）的蓝比红高 179–198 级，而两档文字色（≤17 级）、选中卡薄底
// （≤32 级）与地址卡底（≤11 级）都远低于它，阈值取在中间。
const accentPixelThreshold = 60

// accentPixels 数出矩形里被强调色染过的像素。它只对**真的画出来的**像素
// 成立：无障碍 Label 不进图像，所以"指示器只剩一个 Label"的假绿在这里
// 必然失败。
func accentPixels(img *image.RGBA, r ui.Rect) int {
	b := img.Bounds()
	count := 0
	for y := max(b.Min.Y, int(r.Y)); y < min(b.Max.Y, int(r.Y+r.H)); y++ {
		for x := max(b.Min.X, int(r.X)); x < min(b.Max.X, int(r.X+r.W)); x++ {
			px := img.RGBAAt(x, y)
			if int(px.B)-int(px.R) >= accentPixelThreshold {
				count++
			}
		}
	}
	return count
}

// sameRow 判定两个矩形在同一条次行上：y 与行高一致（宽度随文字内容不同，
// 因此不比）。
func sameRow(a, b ui.Rect) bool {
	return absf(a.Y-b.Y) <= 1 && absf(a.H-b.H) <= 1
}

// dumpLoadingFrame 在 CPR_DUMP_LOADING_FRAME=1 时把真实渲染帧写成 PNG 供
// 人工核验；未设置时不写盘、不产生任何文件。图片来自 viewWith 的真实渲染
// （Tester.Image()），不是手工绘制。
func dumpLoadingFrame(t *testing.T, view *ui.Tester, name string) {
	t.Helper()
	if os.Getenv("CPR_DUMP_LOADING_FRAME") != "1" {
		return
	}
	const dir = "/tmp/cpr-loading-frames"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建 %s 失败: %v", dir, err)
	}
	path := filepath.Join(dir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建 %s 失败: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, view.Image()); err != nil {
		t.Fatalf("写 %s 失败: %v", path, err)
	}
	t.Logf("loading 帧已写出: %s", path)
}

// TestLaunchingFeedbackIsVisibleAndHonest 约束"选中身份到 Chrome 打开之间"
// 的可见反馈：启动状态与选中同帧写入，地址卡次行与选中身份卡副行都立刻
// 显示加载文案**并改用强调色**（像素断言，不是只数文本条目）；启动中不
// 重复交付，失败回到既有错误并允许原地重试；成功交付后按原队列语义出队，
// 出场帧继续显示真正交付的地址与「已交给 Chrome」，绝不回落默认主页；
// 减少动态效果下 loading 只留静态表达，不请求动画帧。
func TestLaunchingFeedbackIsVisibleAndHonest(t *testing.T) {
	const target = "https://example.test/loading"
	quits := 0
	app := settingsApp(func() { quits++ })
	app.enqueue(request{Kind: requestURL, Value: target})
	var launches int
	var delivered uint64
	view := ui.NewTester(func(c *ui.Context) {
		app.viewWith(c, func(p Profile) {
			launches++
			_, delivered, _ = app.beginLaunch()
		})
	}, 660, 420)
	view.SetVibrancy(true)
	view.Advance(time.Second)
	// 启动前记下选中卡副行的位置：启动中它换成了加载文案，而布局不变，
	// 这个盒子仍是同一行，用作强调色的取样区域。
	restSub, ok := view.Find("account1@example.com")
	if !ok {
		t.Fatal("启动前没有画出选中卡的副行")
	}

	if err := view.Click("Identity 1"); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatalf("点击选中卡没有发起启动: %d", launches)
	}
	if !app.snapshot().launching {
		t.Fatal("选中身份后没有在同一帧进入启动状态")
	}
	// 两处文案必须是两个**真的画出来的文本节点**：地址卡次行与选中身份卡
	// 副行。这里按精确匹配计数——指示器的无障碍 Label 用另一个名字
	// （loadingIndicatorLabel），因此不像旧的"含 launchingNote 的条目 ≥2"
	// 那样被 Label 污染。
	notes := 0
	for _, s := range view.Texts() {
		if s == launchingNote {
			notes++
		}
	}
	if notes != 2 {
		t.Fatalf("启动中应当只有地址卡与选中身份卡两处加载文案，实际 %d 处: %v", notes, view.Texts())
	}
	noteBox, ok := view.Find(launchingNote)
	if !ok {
		t.Fatalf("地址卡的加载文案没有画出来: %v", view.Texts())
	}
	cardBox, ok := view.Find("Identity 1")
	if !ok {
		t.Fatal("选中身份卡没有画出来")
	}
	if noteBox.Y+noteBox.H > cardBox.Y {
		t.Fatalf("第一处加载文案不在地址卡里: note=%+v card=%+v", noteBox, cardBox)
	}
	// 地址卡次行与选中卡副行都必须是强调色：与常态同色同字号的小字不算
	// "启动中"这个状态。选中卡的副行取样区域用启动前同一张卡的邮箱行
	// （布局不变，位置仍有效）。
	img := view.Image()
	if n := accentPixels(img, noteBox); n < 20 {
		t.Fatalf("地址卡的加载文案没有用强调色画出来（强调像素 %d）", n)
	}
	if n := accentPixels(img, restSub); n < 20 {
		t.Fatalf("选中卡的加载副行没有强调色（强调像素 %d，取样 %+v）", n, restSub)
	}
	// 启动中重复点击不得再次交付。
	if err := view.Click("Identity 2"); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatalf("启动中重复点击又发起了一次启动: %d", launches)
	}

	// 失败：收起 loading、显示既有错误，并允许原地重试。
	app.completeLaunch(target, delivered, errors.New("boom"))
	view.Frame()
	if app.snapshot().launching {
		t.Fatal("失败后仍停在加载状态")
	}
	if !view.HasText("启动失败：") {
		t.Fatalf("失败后没有显示既有错误: %v", view.Texts())
	}
	if err := view.Click("Identity 1"); err != nil {
		t.Fatal(err)
	}
	if launches != 2 {
		t.Fatalf("失败后不能原地重试: %d", launches)
	}

	// 成功：按原队列语义出队，出场帧继续显示真正交付的地址与诚实文案。
	app.completeLaunch(target, delivered, nil)
	view.Frame()
	if app.pendingCount() != 0 {
		t.Fatalf("成功交付后请求没有出队: %d", app.pendingCount())
	}
	if view.HasText("google.com") {
		t.Fatalf("出场帧回落成默认主页的地址: %v", view.Texts())
	}
	if !view.HasText("example.test") {
		t.Fatalf("出场帧没有继续显示真正交付的地址: %v", view.Texts())
	}
	if !view.HasText("已交给 Chrome") {
		t.Fatalf("出场帧没有给出诚实的交付文案: %v", view.Texts())
	}
	view.Advance(time.Second)
	if quits != 1 {
		t.Fatalf("成功交付后没有按原出场语义退出: quits=%d", quits)
	}

	// 减少动态效果：loading 只留静态文案，不请求动画帧。
	reduced := settingsApp(nil)
	reduced.settings.Motion.ReduceMotion = motionReduceOn
	reduced.enqueue(request{Kind: requestURL, Value: target})
	reducedView := ui.NewTester(func(c *ui.Context) {
		reduced.viewWith(c, func(p Profile) { reduced.beginLaunch() })
	}, 660, 420)
	reducedView.SetVibrancy(true)
	reducedView.Advance(time.Second)
	if err := reducedView.Click("Identity 1"); err != nil {
		t.Fatal(err)
	}
	reducedView.Advance(time.Second)
	if !reducedView.HasText(launchingNote) {
		t.Fatalf("减少动态效果下没有加载文案: %v", reducedView.Texts())
	}
	if reducedView.FramePending() {
		t.Fatal("减少动态效果下 loading 仍在请求动画帧")
	}
}

// TestLaunchingIndicatorIsPainted 断言"启动中"的指示器是**画出来的像素**，
// 而不是只有一个无障碍 Label：深色、浅色、深色 + 减少动态效果、深色 +
// 不透明表面四档都要求指示器盒内存在强调色像素，且指示器与次行都不改变
// 布局（次行仍 15pt、卡片盒子不动）。
//
// 减少动态效果那一档额外要求 FramePending()==false 且推进时钟后像素不变：
// 静态描边既不自调度重绘，也不留下逐帧变化的动画。不透明那一档反过来要求
// 推进时钟后画面确实变了（那里没有流彩，唯一的动画源就是 Spinner），
// 证明动画档用的是真正自调度的 Spinner，而不是静态占位。
func TestLaunchingIndicatorIsPainted(t *testing.T) {
	cases := []struct {
		name     string
		dark     bool
		vibrancy bool
		reduce   bool
	}{
		{name: "dark", dark: true, vibrancy: true},
		{name: "light", dark: false, vibrancy: true},
		{name: "dark-reduce-motion", dark: true, vibrancy: true, reduce: true},
		{name: "dark-opaque", dark: true, vibrancy: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := settingsApp(nil)
			app.enqueue(request{Kind: requestURL, Value: "https://example.test/loading"})
			if tc.dark {
				app.themeMode = ThemeDark
			}
			if tc.reduce {
				app.settings.Motion.ReduceMotion = motionReduceOn
			}
			view := ui.NewTester(func(c *ui.Context) {
				app.viewWith(c, func(p Profile) { app.beginLaunch() })
			}, 660, 420)
			if tc.vibrancy {
				view.SetVibrancy(true)
			}
			view.Advance(time.Second)

			restCard := mustFindRect(t, view, "Identity 1")
			restSub := mustFindRect(t, view, "account1@example.com")
			restNote := mustFindRect(t, view, "/loading")

			if err := view.Click("Identity 1"); err != nil {
				t.Fatal(err)
			}
			// 指针离开列表：几何比较不能被悬停放大污染（选择不受影响）。
			view.Move(0, 0)
			view.Advance(time.Second)
			if !app.snapshot().launching {
				t.Fatal("点击后没有进入启动状态")
			}
			dumpLoadingFrame(t, view, tc.name)

			indicator, ok := view.Find(loadingIndicatorLabel)
			if !ok {
				t.Fatalf("启动中没有画指示器（只有文案不算）: %v", view.Texts())
			}
			// 12–15pt：次行高 15pt，指示器必须明显可见且不得撑高这一行。
			if indicator.W < 12 || indicator.W > 15 || indicator.H < 12 || indicator.H > 15 {
				t.Fatalf("指示器尺寸 %+v 不在次行能容纳且明显可见的档位（12–15pt）", indicator)
			}
			if n := accentPixels(view.Image(), indicator); n < 12 {
				t.Fatalf("指示器只有无障碍 Label，没有真的画出强调色像素（%d）: %+v", n, indicator)
			}

			// 布局不变：地址卡次行仍是 15pt，选中卡盒子与启动前一致。
			note := mustFindRect(t, view, launchingNote)
			if absf(note.H-15) > 0.5 {
				t.Fatalf("地址卡次行高度被改变了: %+v", note)
			}
			if !sameRow(note, restNote) {
				t.Fatalf("加载文案换了行: note=%+v rest=%+v", note, restNote)
			}
			if card := mustFindRect(t, view, "Identity 1"); !nearlySameRect(card, restCard) {
				t.Fatalf("启动状态改变了卡片几何: card=%+v rest=%+v", card, restCard)
			}
			// 选中卡的加载副行在四档外观下都是强调色：opaque 表面（高对比/
			// 降低透明度/无材质）不能退回与常态同色的小字。
			if n := accentPixels(view.Image(), restSub); n < 20 {
				t.Fatalf("选中卡的加载副行没有强调色（强调像素 %d，取样 %+v）", n, restSub)
			}
			before := append([]byte(nil), view.Image().Pix...)
			if tc.reduce {
				if view.FramePending() {
					t.Fatal("减少动态效果下 loading 仍在请求动画帧（静态描边不得自调度）")
				}
				view.Advance(300 * time.Millisecond)
				if !bytes.Equal(before, view.Image().Pix) {
					t.Fatal("减少动态效果下 loading 仍在逐帧变化，静态表达变成了空转动画")
				}
			} else if !tc.vibrancy {
				// 不透明表面没有流彩，唯一的动画源就是 Spinner 自己。
				view.Advance(100 * time.Millisecond)
				if bytes.Equal(before, view.Image().Pix) {
					t.Fatal("动画档的 Spinner 没有自调度重绘，说明画的是静态占位")
				}
			}
		})
	}
}

// TestClickingUnchosenCardLaunchesThatCard 约束"交付哪张卡"与"选中哪张卡"
// 必须一致：指针静止在第二张卡上、键盘把选择移回第一张，再点第二张时，
// 交付的是第二张，选择也必须跟着它；加载强调只能画在真正被交付的那张卡
// 上，旧的选中卡不得同时显示加载状态。
func TestClickingUnchosenCardLaunchesThatCard(t *testing.T) {
	app := settingsApp(nil)
	app.enqueue(request{Kind: requestURL, Value: "https://example.test/loading"})
	var launched []string
	view := ui.NewTester(func(c *ui.Context) {
		app.viewWith(c, func(p Profile) {
			launched = append(launched, p.Name)
			app.beginLaunch()
		})
	}, 660, 420)
	// 无材质（不透明表面）：本用例断言的是"点击哪张卡"的选择语义，不需要
	// 磨砂上的柔光/流彩；磨砂表面下的强调色由 TestLaunchingIndicatorIsPainted
	// 覆盖。
	view.Advance(time.Second)
	// 指针在列表外时记下两张卡副行的位置（静止槽位几何）：启动只换文字，
	// 行位置不变，因此这两个盒子在加载帧里仍是同一行。
	oldSub := mustFindRect(t, view, "account1@example.com")
	deliveredSub := mustFindRect(t, view, "account2@example.com")

	// 指针先落在第一张卡上（第一帧只登记悬停矩形），再移到第二张：第二次
	// 移动落到"与上一帧不同的一行"，选择因此跟着指针到第二张。
	first := mustFindRect(t, view, "Identity 1")
	second := mustFindRect(t, view, "Identity 2")
	cx, cy := second.X+second.W/2, second.Y+second.H/2
	view.Move(first.X+first.W/2, first.Y+first.H/2)
	view.Move(cx, cy)
	view.Advance(time.Second)
	if app.chosen != 1 {
		t.Fatalf("指针真实移动没有选中第二张卡: chosen=%d", app.chosen)
	}
	grown := mustFindRect(t, view, "Identity 2")
	if cx < grown.X || cx > grown.X+grown.W || cy < grown.Y || cy > grown.Y+grown.H {
		t.Fatalf("指针停下的位置不在放大后的第二张卡里: grown=%+v", grown)
	}

	// 键盘把选择移回第一张；指针保持静止，不能把选择拽回去。
	view.Key(0, ui.KeyLeft)
	view.Advance(time.Second)
	if app.chosen != 0 {
		t.Fatalf("键盘没有把选择移回第一张: chosen=%d", app.chosen)
	}

	// 指针一动不动地点击第二张卡：交付第二张，选择必须跟着它。
	view.ClickAt(cx, cy)
	// 指针离开列表：加载期间不再有悬停放大，几何回到静止槽位，上面的取样
	// 区域仍然有效；选择在启动中受守卫，不会因为指针移开而改变。
	view.Move(0, 0)
	view.Advance(time.Second)

	if len(launched) != 1 || launched[0] != "Identity 2" {
		t.Fatalf("点击第二张卡应当交付第二张，实际: %v", launched)
	}
	if !app.snapshot().launching {
		t.Fatal("点击后没有进入启动状态")
	}
	// 加载强调必须落在被交付的那张卡上：它的副行是强调色的加载文案，旧的
	// 选中卡只剩普通邮箱色。两条都是像素断言——无障碍 Label 进不了图像，
	// 因此"只画了文字节点"的假绿在这里不成立。
	img := view.Image()
	if n := accentPixels(img, deliveredSub); n < 20 {
		t.Fatalf("被交付的卡没有画出加载强调（强调像素 %d，区域 %+v）", n, deliveredSub)
	}
	if n := accentPixels(img, oldSub); n != 0 {
		t.Fatalf("旧的选中卡仍在画加载强调（强调像素 %d，区域 %+v）", n, oldSub)
	}
	if app.chosen != 1 {
		t.Fatalf("交付的是第二张卡，选中项却停在 %d（loading 会画错卡）", app.chosen)
	}
	notes := 0
	for _, s := range view.Texts() {
		if s == launchingNote {
			notes++
		}
	}
	if notes != 2 {
		t.Fatalf("启动中应当只有地址卡与被交付卡两处加载文案，实际 %d 处: %v", notes, view.Texts())
	}
}

// TestEscapeQuitsWhileLaunching 约束装饰性 loading 不拦键盘：启动中 Esc
// 仍然立即退出，与既有优先级一致。
func TestEscapeQuitsWhileLaunching(t *testing.T) {
	quits := 0
	app := settingsApp(func() { quits++ })
	view := ui.NewTester(func(c *ui.Context) {
		app.viewWith(c, func(p Profile) { app.beginLaunch() })
	}, 660, 420)
	view.Advance(time.Second)
	if err := view.Click("Identity 1"); err != nil {
		t.Fatal(err)
	}
	if !app.snapshot().launching {
		t.Fatal("点击后没有进入启动状态")
	}
	view.Key(0, ui.KeyEscape)
	if quits != 1 {
		t.Fatalf("启动中 Esc 没有立即退出: quits=%d", quits)
	}
}
