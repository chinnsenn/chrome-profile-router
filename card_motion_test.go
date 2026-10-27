// [INPUT]: 依赖 card_motion.go 的纯函数与 ui.NewTester 的离屏帧；不接触 Chrome 或真实窗口。
// [OUTPUT]: 约束卡片倾斜的方向、钳制、ReduceMotion 归零，以及"外壳几何与键盘选择不受倾斜影响"。
// [POS]: 根包的动效回归入口，与 ui_test.go 的输入回归并列。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestCardTiltTargetZeroWhenInactive 约束"未悬停或 ReduceMotion 时目标为零"：
// 不产生任何倾斜、缩放或视差位移，而不是跳变到某个静止角度。
func TestCardTiltTargetZeroWhenInactive(t *testing.T) {
	cases := []struct {
		name         string
		px, py       float32
		w, h         float32
		over         bool
		reduceMotion bool
	}{
		{"not hovered", 10, 10, 200, 56, false, false},
		{"reduce motion", 10, 10, 200, 56, true, true},
		{"zero width", 10, 10, 0, 56, true, false},
		{"zero height", 10, 10, 200, 0, true, false},
		{"negative box", 10, 10, -1, -1, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rx, ry, s, sx, sy := cardTiltTarget(tc.px, tc.py, tc.w, tc.h, tc.over, tc.reduceMotion)
			if rx != 0 || ry != 0 || s != 1 || sx != 0 || sy != 0 {
				t.Fatalf("inactive card must have no tilt: rotX=%v rotY=%v scale=%v shiftX=%v shiftY=%v", rx, ry, s, sx, sy)
			}
		})
	}
}

// TestCardTiltTargetFollowsPointer 约束方向与来源一致：指针在左上时左边缘
// 靠近、上边缘靠近（rotateY>0、rotateX<0），右下时符号全部相反；归一化
// 钳制在 [-1,1]，盒外的指针不会放大角度。
func TestCardTiltTargetFollowsPointer(t *testing.T) {
	const w, h = 200, 56

	rxTL, ryTL, sTL, sxTL, syTL := cardTiltTarget(0, 0, w, h, true, false)
	rxBR, ryBR, sBR, sxBR, syBR := cardTiltTarget(w, h, w, h, true, false)
	if !(ryTL > 0 && rxTL < 0) {
		t.Fatalf("top-left must tilt its left and top edges toward the viewer: rotX=%v rotY=%v", rxTL, ryTL)
	}
	if !(ryBR < 0 && rxBR > 0) {
		t.Fatalf("bottom-right must be the opposite of top-left: rotX=%v rotY=%v", rxBR, ryBR)
	}
	if rxTL != -rxBR || ryTL != -ryBR {
		t.Fatalf("opposite corners must be symmetric: TL=(%v,%v) BR=(%v,%v)", rxTL, ryTL, rxBR, ryBR)
	}
	if sTL != cardHoverScale || sBR != cardHoverScale {
		t.Fatalf("hover must scale by %v: got %v and %v", cardHoverScale, sTL, sBR)
	}
	if !(sxTL < 0 && syTL < 0 && sxBR > 0 && syBR > 0) {
		t.Fatalf("parallax must follow the pointer side: TL=(%v,%v) BR=(%v,%v)", sxTL, syTL, sxBR, syBR)
	}

	// 中心：没有角度，但悬停缩放仍然存在。
	if rx, ry, s, sx, sy := cardTiltTarget(w/2, h/2, w, h, true, false); rx != 0 || ry != 0 || s != cardHoverScale || sx != 0 || sy != 0 {
		t.Fatalf("center must be flat: rotX=%v rotY=%v scale=%v shift=(%v,%v)", rx, ry, s, sx, sy)
	}

	// 钳制：盒外远处与盒角给出同一个极限值。
	cornerRX, cornerRY, _, cornerSX, cornerSY := cardTiltTarget(w, h, w, h, true, false)
	farRX, farRY, _, farSX, farSY := cardTiltTarget(10*w, 10*h, w, h, true, false)
	if cornerRX != farRX || cornerRY != farRY || cornerSX != farSX || cornerSY != farSY {
		t.Fatalf("out-of-box pointer must clamp: corner=(%v,%v,%v,%v) far=(%v,%v,%v,%v)",
			cornerRX, cornerRY, cornerSX, cornerSY, farRX, farRY, farSX, farSY)
	}
	if max(cornerRX, -cornerRX) > cardTiltMax+1e-4 || max(cornerRY, -cornerRY) > cardTiltMax+1e-4 {
		t.Fatalf("angles exceed the maximum %v: rotX=%v rotY=%v", cardTiltMax, cornerRX, cornerRY)
	}
}

// tiltApp 是倾斜回归用的最小应用：三个身份，单列卡片。
func tiltApp() *selectorApp {
	return &selectorApp{themeMode: ThemeLight, profiles: []Profile{
		{Dir: "Default", Name: "Personal", Email: "personal@example.com", Color: ui.Hex("#3686E8")},
		{Dir: "Profile 1", Name: "Work", Email: "work@example.com", Color: ui.Hex("#42A389")},
		{Dir: "Profile 2", Name: "Testing", Email: "testing@example.com", Color: ui.Hex("#9271D8")},
	}}
}

// TestCardTiltKeepsShellGeometryAndKeyboardChoice 是本次改动的验收条件：
// 倾斜动画进行中，(1) 外壳几何（命中区域）不变，(2) 静止指针不得改写键盘
// 选择。后者同时覆盖"入场面板缩放期间局部坐标漂移"这一回归——坐标取自
// 根元素后必须稳定。
func TestCardTiltKeepsShellGeometryAndKeyboardChoice(t *testing.T) {
	app := tiltApp()
	view := ui.NewTester(app.view, 660, 490)

	// 基准：指针不在窗口里时第一张卡的几何。
	before, ok := view.Find("Personal")
	if !ok {
		t.Fatal("first profile is not visible")
	}

	// 悬停到卡片左上角，让倾斜真实发生，然后跑完动画。
	view.Move(before.X+2, before.Y+2)
	view.Advance(time.Second)
	if !view.FramePending() {
		// 没有待渲染帧说明弹簧已静止；这是允许的，但倾斜通道必须已经建立。
		t.Log("tilt spring already settled")
	}
	view.Advance(time.Second)

	during, ok := view.Find("Personal")
	if !ok {
		t.Fatal("hovered profile disappeared")
	}
	if !nearlySameRect(before, during) {
		t.Fatalf("tilt moved the card's shell: before=%+v during=%+v", before, during)
	}

	// 倾斜进行中按方向键：静止指针不得把选择改回来。
	view.Key(0, ui.KeyRight)
	view.Advance(20 * time.Millisecond)
	view.Advance(20 * time.Millisecond)
	if app.chosen != 1 {
		t.Fatalf("stationary pointer overwrote the keyboard choice during tilt: got %d, want 1", app.chosen)
	}

	// 入场/倾斜都未结束时再确认一次：不推进时间，只补帧。
	view.Frame()
	if app.chosen != 1 {
		t.Fatalf("extra frame overwrote the keyboard choice during tilt: got %d, want 1", app.chosen)
	}

	// 指针移出卡片：倾斜回中，几何仍然不变。
	view.Move(2, 2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	after, ok := view.Find("Personal")
	if !ok {
		t.Fatal("profile disappeared after the pointer left")
	}
	if !nearlySameRect(before, after) {
		t.Fatalf("returning from tilt moved the card's shell: before=%+v after=%+v", before, after)
	}
	if app.chosen != 1 {
		t.Fatalf("leaving the card changed the choice: got %d, want 1", app.chosen)
	}
}

// TestCardTiltEntryAnimationKeepsKeyboardChoice 是"入场缩放期间静止指针不得
// 改写键盘选择"的回归：列表容器随面板缩放，它的局部坐标逐帧漂移；若把那份
// 坐标当成"指针移动"的依据，静止的指针会把刚用方向键做出的选择改回去。
// 这里选一个在入场动画中会跨越行边界的窗口坐标（(350,200) 在旧实现里会把
// 选择从 1 改成 4）。
func TestCardTiltEntryAnimationKeepsKeyboardChoice(t *testing.T) {
	app := &selectorApp{themeMode: ThemeLight}
	for i := range 9 {
		app.profiles = append(app.profiles, Profile{
			Dir: fmt.Sprintf("Profile %d", i), Name: fmt.Sprintf("Identity %d", i+1),
			Email: fmt.Sprintf("account%d@example.com", i+1), Color: ui.Hex("#3686E8"),
		})
	}
	app.resetEntry() // 让入场动画真实进行，而不是直接停在就绪态
	view := ui.NewTester(app.view, 700, 470)
	view.Move(350, 200)
	view.Advance(6 * time.Millisecond)
	view.Key(0, ui.KeyRight)
	if app.chosen != 1 {
		t.Fatalf("the key press must apply in its own frame: got %d, want 1", app.chosen)
	}
	for range 12 {
		view.Advance(6 * time.Millisecond)
		if app.chosen != 1 {
			t.Fatalf("the entry animation's drifting list coordinates overwrote the keyboard choice: got %d, want 1", app.chosen)
		}
	}
}

// TestCardTiltRendersReal3D 约束倾斜不是 2D 假动作：内层内容随指针一侧
// 发生透视位移（bbox 尺寸改变），而外壳盒子不动；ReduceMotion 下内层完全
// 不位移。
func TestCardTiltRendersReal3D(t *testing.T) {
	app := tiltApp()
	view := ui.NewTester(app.view, 660, 490)

	// Find 先返回外壳（父先于子被登记），因此这里用副文本测量内层。
	shell, ok := view.Find("Personal")
	if !ok {
		t.Fatal("first profile is not visible")
	}
	rest, ok := view.Find("personal@example.com")
	if !ok {
		t.Fatal("subtitle is not visible")
	}

	view.Move(shell.X+2, shell.Y+2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	tl, _ := view.Find("personal@example.com")
	view.Move(shell.X+shell.W-2, shell.Y+shell.H-2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	br, _ := view.Find("personal@example.com")

	// 纯 2D 平移/均匀缩放做不出这种不对称：左上时左边缘外扩、右边缘内收，
	// 右下时正好相反。
	if tl.X >= rest.X || br.X <= rest.X {
		t.Fatalf("inner content must move with the pointer side: rest=%v topLeft=%v bottomRight=%v", rest, tl, br)
	}
	// 透视下倾斜会改变 bbox 高度（纯旋转不缩放不改变它）。
	if tl.H <= rest.H || br.H <= rest.H {
		t.Fatalf("perspective must change the tilted content's height: rest=%v topLeft=%v bottomRight=%v", rest, tl, br)
	}
	if !nearlySameRect(shell, mustFindRect(t, view, "Personal")) {
		t.Fatalf("the shell must not move while the inner content tilts")
	}

	// ReduceMotion：内层内容一像素都不动。
	still := ui.NewTester(app.view, 660, 490)
	still.SetPreferences(ui.Preferences{ReduceMotion: true})
	stillShell, ok := still.Find("Personal")
	if !ok {
		t.Fatal("first profile is not visible with ReduceMotion")
	}
	stillRest, ok := still.Find("personal@example.com")
	if !ok {
		t.Fatal("subtitle is not visible with ReduceMotion")
	}
	still.Move(stillShell.X+2, stillShell.Y+2)
	still.Advance(time.Second)
	still.Move(stillShell.X+stillShell.W-2, stillShell.Y+stillShell.H-2)
	still.Advance(time.Second)
	if got, _ := still.Find("personal@example.com"); got != stillRest {
		t.Fatalf("ReduceMotion must not move the inner content: rest=%+v got=%+v", stillRest, got)
	}
}

// TestCardTiltGridKeepsOrderAndColumns 约束倾斜不改变身份顺序、列数与窗口尺寸：
// 九身份网格下扫过卡片四角后，顺序、文本与窗口尺寸都不变。
func TestCardTiltGridKeepsOrderAndColumns(t *testing.T) {
	app := &selectorApp{themeMode: ThemeLight}
	for i := range 9 {
		app.profiles = append(app.profiles, Profile{
			Dir: fmt.Sprintf("Profile %d", i), Name: fmt.Sprintf("Identity %d", i+1),
			Email: fmt.Sprintf("account%d@example.com", i+1), Color: ui.Hex("#3686E8"),
		})
	}
	view := ui.NewTester(app.view, 700, 470)

	var baseline []ui.Rect
	for _, p := range app.profiles {
		box, ok := view.Find(p.Name)
		if !ok {
			t.Fatalf("profile %q is not visible", p.Name)
		}
		baseline = append(baseline, box)
	}

	// 依次扫过第一张卡片的四个角，每次跑完弹簧。
	first := baseline[0]
	for _, pt := range [][2]float32{
		{first.X + 1, first.Y + 1},
		{first.X + first.W - 1, first.Y + 1},
		{first.X + first.W - 1, first.Y + first.H - 1},
		{first.X + 1, first.Y + first.H - 1},
	} {
		view.Move(pt[0], pt[1])
		view.Advance(400 * time.Millisecond)
	}

	for i, p := range app.profiles {
		box, ok := view.Find(p.Name)
		if !ok {
			t.Fatalf("profile %q disappeared while tilting", p.Name)
		}
		if !nearlySameRect(baseline[i], box) {
			t.Fatalf("tilt moved profile %q: before=%+v after=%+v", p.Name, baseline[i], box)
		}
	}
	if w, h := view.Image().Rect.Dx(), view.Image().Rect.Dy(); w != 700 || h != 470 {
		t.Fatalf("window resized by tilt: got %dx%d", w, h)
	}
}

// mustFindRect 返回文本或标签的盒子，找不到就失败。
func mustFindRect(t *testing.T, view *ui.Tester, s string) ui.Rect {
	t.Helper()
	r, ok := view.Find(s)
	if !ok {
		t.Fatalf("%q is not visible", s)
	}
	return r
}

// nearlySameRect 判定两个矩形在 1pt 内一致：弹簧完全归中前允许极小的残余。
func nearlySameRect(a, b ui.Rect) bool {
	const tol = 1
	return absf(a.X-b.X) <= tol && absf(a.Y-b.Y) <= tol && absf(a.W-b.W) <= tol && absf(a.H-b.H) <= tol
}

// cardProbe 是一张卡片在某一帧的实测几何。
type cardProbe struct {
	shell ui.Rect // 外壳（命中盒）矩形
	surf  ui.Rect // 卡片表面（背景+描边）的渲染外接矩形
	text  ui.Rect // 内层副文本矩形
	badge ui.Rect // 数字键徽标矩形
}

// probeCards 在给定指针位置上渲染一帧，并实测第一张卡片的表面与内容关系。
//
// 逐行逐列比较"表面底色"与"结构性内容"的边界，而不是只比外接矩形：倾斜
// 会把内容推出表面几 pt，这个量在外接矩形里会被抵消，只有在同一行上比较
// 才看得见。扫描窗口按半个卡片间隙裁剪，相邻卡片不会被算进来。
func probeCards(t *testing.T, app *selectorApp, w, h int, px, py float32, names []string, count int) cardProbe {
	t.Helper()
	view := ui.NewTester(app.view, w, h)
	view.Move(px, py)
	view.Advance(time.Second)
	view.Advance(time.Second)

	shell := mustFindRect(t, view, names[0])
	img := view.Image()
	fill := dominantColor(img, shell, w, h) // 卡片自己的底色

	// 半个间隙：单列 rowGap/2 = 2，网格 profileGridGap/2 = 3。
	margin := float32(2)
	if count > singleColumnLimit {
		margin = 3
	}
	x0 := max(int(shell.X-margin), 0)
	x1 := min(int(shell.X+shell.W+margin)-1, w-1)
	y0 := max(int(shell.Y-margin), 0)
	y1 := min(int(shell.Y+shell.H+margin)-1, h-1)

	isFill := func(x, y int) bool { return sameColor(img.RGBAAt(x, y), fill) }

	// 表面外接矩形。
	minX, minY, maxX, maxY := 1<<30, 1<<30, -1, -1
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if isFill(x, y) {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		t.Fatalf("the card surface painted nothing")
	}
	surf := ui.Rect{X: float32(minX), Y: float32(minY), W: float32(maxX - minX + 1), H: float32(maxY - minY + 1)}

	return cardProbe{
		shell: shell,
		surf:  surf,
		text:  mustFindRect(t, view, names[1]),
		badge: mustFindRect(t, view, names[2]),
	}
}

// grow 把 r 四周各放大 d。
func grow(r ui.Rect, d float32) ui.Rect {
	return ui.Rect{X: r.X - d, Y: r.Y - d, W: r.W + 2*d, H: r.H + 2*d}
}

// inside 判定 inner 完全落在 outer 内（0.5pt 容差，吸收像素取整）。
func inside(inner, outer ui.Rect) bool {
	const tol = 0.5
	return inner.X >= outer.X-tol && inner.Y >= outer.Y-tol &&
		inner.X+inner.W <= outer.X+outer.W+tol && inner.Y+inner.H <= outer.Y+outer.H+tol
}

// TestCardContentStaysInsideItsSurface 是缺陷 1 的回归：可见表面（背景+描边）
// 必须与内容一起倾斜，于是内容永远在卡片自己的边框之内。
//
// 两条断言，缺一不可：
//  1. 内容外接矩形不超出表面外接矩形 1.5pt 以上（1pt 描边 + 抗锯齿的余量）；
//  2. 倾斜时表面本身的外接矩形会变（高度增量 > 2pt），证明表面确实在倾斜层
//     里。旧实现把表面留在平直的外壳上、只倾斜内容，表面尺寸恒定不变，第 2
//     条断言因此失败——这正是"内容越过卡片边框"的根因。
func TestCardContentStaysInsideItsSurface(t *testing.T) {
	app := &selectorApp{themeMode: ThemeLight, profiles: []Profile{
		{Dir: "Default", Name: "Personal", Email: "personal@example.com", Color: ui.Hex("#3686E8")},
		{Dir: "Profile 1", Name: "Work", Email: "work@example.com", Color: ui.Hex("#42A389")},
		{Dir: "Profile 2", Name: "Testing", Email: "testing@example.com", Color: ui.Hex("#9271D8")},
	}}
	names := []string{"Personal", "personal@example.com", "1"}
	shell := mustFindRect(t, ui.NewTester(app.view, 660, 420), "Personal")

	rest := probeCards(t, app, 660, 420, 1, 1, names, len(app.profiles))
	corner := probeCards(t, app, 660, 420, shell.X+3, shell.Y+3, names, len(app.profiles))
	if grow := absf(corner.surf.H - rest.surf.H); grow <= 2 {
		t.Fatalf("the card surface does not tilt with its content: rest=%+v tilted=%+v (height change %.1fpt)",
			rest.surf, corner.surf, grow)
	}

	for _, pt := range [][2]float32{
		{shell.X + 3, shell.Y + 3},                     // 左上角内侧
		{shell.X + shell.W - 3, shell.Y + 3},           // 右上角内侧
		{shell.X + 3, shell.Y + shell.H - 3},           // 左下角内侧
		{shell.X + shell.W - 3, shell.Y + shell.H - 3}, // 右下角内侧
		{shell.X + shell.W/2, shell.Y + shell.H/2},     // 中心
	} {
		p := probeCards(t, app, 660, 420, pt[0], pt[1], names, len(app.profiles))
		for label, r := range map[string]ui.Rect{"subtitle": p.text, "badge": p.badge} {
			if !inside(r, grow(p.surf, 1.5)) {
				t.Fatalf("at %v: %s spills outside the card surface: content=%+v surface=%+v", pt, label, r, p.surf)
			}
		}
	}
}

// TestCardTiltStrengthMatchesAcrossWidths 是缺陷 2 的回归：透视距离随卡片
// 宽度成比例（cardPerspective），所以窄卡与宽卡得到同一相对形变。
//
// 形变量用 card_motion.go 里 cardPerspective 推导出的同一公式算：把卡片宽度
// 归一化为 u ∈ [-1/2, 1/2]，绕 y 轴转 θ 后的横向投影为
//
//	u' = u·cosθ / (1 + u·sinθ/k),  k = 透视距离 / 卡片宽度
//
// 这个量只取决于角度与 k。测试用同一指针相对位置（左上角内侧 3pt）得到同一
// 角度，于是只剩 k 在起作用；两种真实卡片宽度（单列 622pt、网格 216.7pt）
// 的相对形变差值必须 <15%。渲染侧另有 TestCardTiltRendersReal3D 与
// TestCardContentStaysInsideItsSurface 证明该公式与绘制一致（拟合残差 <0.4px）。
func TestCardTiltStrengthMatchesAcrossWidths(t *testing.T) {
	if got := cardPerspective(0); got != cardTiltPerspective {
		t.Fatalf("unknown width must fall back to the constant: got %v", got)
	}
	if a, b := cardPerspective(600), cardPerspective(200); absf(a/b-3) > 0.01 {
		t.Fatalf("perspective must scale with width: 600→%v 200→%v", a, b)
	}

	// 相对形变：卡片左右边缘在投影后相对宽度的变化量。
	deform := func(width float32) float64 {
		// 左上角内侧 3pt 的同一相对位置，得到同一角度（角度不依赖宽度）。
		px, py := float32(3), float32(3)
		_, rotY, _, _, _ := cardTiltTarget(px, py, width, 56, true, false)
		theta := float64(rotY) * math.Pi / 180
		k := float64(cardPerspective(width) / width)
		u := 0.5
		outer := u * math.Cos(theta) / (1 + u*math.Sin(theta)/k)
		return 2 * (u - outer) // 两侧边缘合计收窄比例
	}

	narrow := deform(622) // 单列卡实测外壳宽度
	wide := deform(216.7) // 网格卡实测外壳宽度
	ratio := absf(float32(narrow-wide)) / max(float32(narrow), float32(wide))
	t.Logf("relative tilt deformation: 622pt card %.5f, 216.7pt card %.5f, difference %.1f%%", narrow, wide, ratio*100)
	if ratio >= 0.15 {
		t.Fatalf("tilt strength must not depend on card width: 622pt→%.5f vs 216.7pt→%.5f (%.1f%%)", narrow, wide, ratio*100)
	}

	// 旧行为（固定 700pt 透视）在同一公式下必然超出阈值，说明这条测试
	// 确实约束了 cardPerspective，而不是恒真。
	fixed := func(width float32) float64 {
		_, rotY, _, _, _ := cardTiltTarget(3, 3, width, 56, true, false)
		theta := float64(rotY) * math.Pi / 180
		k := float64(cardTiltPerspective / width)
		u := 0.5
		outer := u * math.Cos(theta) / (1 + u*math.Sin(theta)/k)
		return 2 * (u - outer)
	}
	oldRatio := absf(float32(fixed(622)-fixed(216.7))) / max(float32(fixed(622)), float32(fixed(216.7)))
	t.Logf("same formula with a fixed 700pt perspective would differ by %.1f%%", oldRatio*100)
	if oldRatio < 0.15 {
		t.Fatalf("the fixed-perspective comparison must fail the threshold, else this test proves nothing: %.1f%%", oldRatio*100)
	}
}

// TestCardTiltStrengthMeasuredAcrossWidths 用真实绘制佐证同一件事：在 4× 缩放
// 下量卡片表面渲染高度的相对增量（1× 下这个量只有 1–5 像素，会被像素取整
// 吞掉），单列卡与网格卡的相对形变差值必须 <15%。
//
// 注意分工：真正的回归守卫是上面的解析测试——像素取整会让这个实测指标在
// 固定 700pt 透视下也只差 13.9%（勉强过线），而解析指标差 58.7%。实测测试
// 只证明"绘制结果与推导一致、两种宽度落在同一量级"，不承担区分新旧实现的
// 职责。
func TestCardTiltStrengthMeasuredAcrossWidths(t *testing.T) {
	relative := func(count, w, h int) (rel float64, cardW float32) {
		app := &selectorApp{themeMode: ThemeLight}
		for i := range count {
			app.profiles = append(app.profiles, Profile{
				Dir: fmt.Sprintf("Profile %d", i), Name: fmt.Sprintf("Identity %d", i+1),
				Email: fmt.Sprintf("account%d@example.com", i+1), Color: ui.Hex("#3686E8"),
			})
		}
		view := ui.NewTester(app.view, w, h)
		view.SetScale(4)

		// surfaceHeight 把指针放到 (px, py) 后，量卡片表面底色的渲染高度。
		surfaceHeight := func(px, py float32) (float32, float32) {
			view.Move(px, py)
			view.Advance(time.Second)
			view.Advance(time.Second)
			shell := mustFindRect(t, view, "Identity 1")
			img := view.Image()
			fill := img.RGBAAt(int(shell.X+shell.W/2)*4, int(shell.Y)*4+12)
			minY, maxY := 1<<30, -1
			for y := max(int(shell.Y)*4-40, 0); y < min(int((shell.Y+shell.H)*4)+40, h*4); y++ {
				for x := max(int(shell.X)*4-40, 0); x < min(int((shell.X+shell.W)*4)+40, w*4); x++ {
					if sameColor(img.RGBAAt(x, y), fill) {
						minY, maxY = min(minY, y), max(maxY, y)
					}
				}
			}
			if maxY < 0 {
				t.Fatalf("no card surface painted")
			}
			return shell.W, float32(maxY-minY+1) / 4
		}

		cardW, restH := surfaceHeight(1, 1)
		shell := mustFindRect(t, view, "Identity 1")
		_, tiltH := surfaceHeight(shell.X+3, shell.Y+3)
		return float64(absf(tiltH-restH)) / float64(cardW), cardW
	}

	narrow, narrowW := relative(3, 660, 420) // 单列卡
	wide, wideW := relative(9, 700, 470)     // 网格卡
	ratio := absf(float32(narrow-wide)) / max(float32(narrow), float32(wide))
	t.Logf("measured relative deformation: %.1fpt card %.5f, %.1fpt card %.5f, difference %.1f%%",
		narrowW, narrow, wideW, wide, ratio*100)
	if ratio >= 0.15 {
		t.Fatalf("measured tilt strength depends on card width: %.1fpt→%.5f vs %.1fpt→%.5f (%.1f%%)",
			narrowW, narrow, wideW, wide, ratio*100)
	}
}

// dominantColor 返回 r 内出现最多的像素颜色。
func dominantColor(img *image.RGBA, r ui.Rect, w, h int) color.RGBA {
	counts := map[color.RGBA]int{}
	for y := max(int(r.Y), 0); y <= min(int(r.Y+r.H)-1, h-1); y++ {
		for x := max(int(r.X), 0); x <= min(int(r.X+r.W)-1, w-1); x++ {
			counts[img.RGBAAt(x, y)]++
		}
	}
	best, bestN := color.RGBA{}, -1
	for c, n := range counts {
		if n > bestN || (n == bestN && c.A > best.A) {
			best, bestN = c, n
		}
	}
	return best
}

// sameColor 判定两个像素属于同一块表面：每通道容差 2，吸收抗锯齿与舍入。
func sameColor(a, b color.RGBA) bool {
	return absf(float32(a.R)-float32(b.R)) <= 2 &&
		absf(float32(a.G)-float32(b.G)) <= 2 &&
		absf(float32(a.B)-float32(b.B)) <= 2 &&
		absf(float32(a.A)-float32(b.A)) <= 2
}
