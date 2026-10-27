// [INPUT]: 依赖 card_motion.go 的纯函数与 ui.NewTester 的离屏帧；不接触 Chrome 或真实窗口。
// [OUTPUT]: 约束卡片倾斜的方向、钳制、ReduceMotion 归零，以及"外壳几何与键盘选择不受倾斜影响"。
// [POS]: 根包的动效回归入口，与 ui_test.go 的输入回归并列。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"image"
	"image/color"
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
			rx, ry, sx, sy := cardTiltTarget(tc.px, tc.py, tc.w, tc.h, tc.over, tc.reduceMotion)
			if rx != 0 || ry != 0 || sx != 0 || sy != 0 {
				t.Fatalf("inactive card must have no tilt: rotX=%v rotY=%v shiftX=%v shiftY=%v", rx, ry, sx, sy)
			}
		})
	}
}

// TestCardTiltTargetFollowsPointer 约束方向与来源一致：指针在左上时左边缘
// 靠近、上边缘靠近（rotateY>0、rotateX<0），右下时符号全部相反；归一化
// 钳制在 [-1,1]，盒外的指针不会放大角度。
func TestCardTiltTargetFollowsPointer(t *testing.T) {
	const w, h = 200, 56

	rxTL, ryTL, sxTL, syTL := cardTiltTarget(0, 0, w, h, true, false)
	rxBR, ryBR, sxBR, syBR := cardTiltTarget(w, h, w, h, true, false)
	if !(ryTL > 0 && rxTL < 0) {
		t.Fatalf("top-left must tilt its left and top edges toward the viewer: rotX=%v rotY=%v", rxTL, ryTL)
	}
	if !(ryBR < 0 && rxBR > 0) {
		t.Fatalf("bottom-right must be the opposite of top-left: rotX=%v rotY=%v", rxBR, ryBR)
	}
	if rxTL != -rxBR || ryTL != -ryBR {
		t.Fatalf("opposite corners must be symmetric: TL=(%v,%v) BR=(%v,%v)", rxTL, ryTL, rxBR, ryBR)
	}
	if !(sxTL < 0 && syTL < 0 && sxBR > 0 && syBR > 0) {
		t.Fatalf("parallax must follow the pointer side: TL=(%v,%v) BR=(%v,%v)", sxTL, syTL, sxBR, syBR)
	}

	// 中心：没有角度也没有视差（放大由 hover_layout.go 负责，不在这里）。
	if rx, ry, sx, sy := cardTiltTarget(w/2, h/2, w, h, true, false); rx != 0 || ry != 0 || sx != 0 || sy != 0 {
		t.Fatalf("center must be flat: rotX=%v rotY=%v shift=(%v,%v)", rx, ry, sx, sy)
	}

	// 钳制：盒外远处与盒角给出同一个极限值。
	cornerRX, cornerRY, cornerSX, cornerSY := cardTiltTarget(w, h, w, h, true, false)
	farRX, farRY, farSX, farSY := cardTiltTarget(10*w, 10*h, w, h, true, false)
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

// TestCardHoverKeepsKeyboardChoiceAndRestoresGeometry 是交互层改动的验收条件：
// 悬停动画进行中，(1) 悬停卡相对静止尺寸真的变大、邻居让开且互不重叠，
// (2) 静止指针不得改写键盘选择，(3) 指针离开后所有卡片回到静止几何。
//
// 后者同时覆盖"入场面板缩放期间局部坐标漂移"这一回归——坐标取自根元素后
// 必须稳定。
func TestCardHoverKeepsKeyboardChoiceAndRestoresGeometry(t *testing.T) {
	app := tiltApp()
	view := ui.NewTester(app.view, 660, 490)

	// 基准：指针不在窗口里时三张卡的几何。
	var before []ui.Rect
	for _, p := range app.profiles {
		before = append(before, mustFindRect(t, view, p.Name))
	}

	// 悬停到第一张卡左上角：放大与倾斜一起发生，然后跑完弹簧。
	view.Move(before[0].X+2, before[0].Y+2)
	view.Advance(time.Second)
	view.Advance(time.Second)

	during, ok := view.Find("Personal")
	if !ok {
		t.Fatal("hovered profile disappeared")
	}
	if during.W <= before[0].W || during.H <= before[0].H {
		t.Fatalf("the hovered card must grow past its rest size: rest=%+v hovered=%+v", before[0], during)
	}
	neighbor := mustFindRect(t, view, "Work")
	if neighbor.H >= before[1].H {
		t.Fatalf("the neighbour must give way: rest=%+v hovered=%+v", before[1], neighbor)
	}
	if during.Y+during.H > neighbor.Y+0.5 {
		t.Fatalf("the hovered card overlaps its neighbour: hovered=%+v neighbour=%+v", during, neighbor)
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
	for i, p := range app.profiles {
		after, ok := view.Find(p.Name)
		if !ok {
			t.Fatalf("profile %q disappeared after the pointer left", p.Name)
		}
		if !nearlySameRect(before[i], after) {
			t.Fatalf("leaving the card did not restore %q: before=%+v after=%+v", p.Name, before[i], after)
		}
	}
	if app.chosen != 1 {
		t.Fatalf("leaving the card changed the choice: got %d, want 1", app.chosen)
	}
}

// TestCardTiltEntryAnimationKeepsKeyboardChoice 是"入场缩放期间静止指针不得
// 改写键盘选择"的回归：面板进出场会缩放整个列表，卡片的窗口矩形逐帧变化；
// 若把那份变化当成"指针移动"，静止的指针就会把刚用方向键做出的选择改回去。
// 指针坐标取自根元素（窗口坐标），因此入场动画不会伪造移动。
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

	// 指针落下的那一帧会把选择同步到指针下的行（这是真实移动，符合设计）；
	// 之后的按键必须在自己的那一帧前进一格，并在整段入场动画里保持不变。
	before := app.chosen
	view.Key(0, ui.KeyRight)
	want := app.chosen
	if want != before+1 {
		t.Fatalf("the key press must apply in its own frame: got %d, want %d", want, before+1)
	}
	for range 12 {
		view.Advance(6 * time.Millisecond)
		if app.chosen != want {
			t.Fatalf("the entry animation's moving card rectangles overwrote the keyboard choice: got %d, want %d", app.chosen, want)
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
	// 指针不动时，倾斜本身不得让交互层漂移：弹簧静止后连续两帧的盒子一致。
	steady := mustFindRect(t, view, "Personal")
	view.Advance(50 * time.Millisecond)
	if !nearlySameRect(steady, mustFindRect(t, view, "Personal")) {
		t.Fatalf("tilt keeps drifting the interaction layer while the pointer is still: %+v → %+v",
			steady, mustFindRect(t, view, "Personal"))
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

// TestCardTiltGridKeepsOrderAndColumns 约束悬停/倾斜不改变身份顺序、列数与
// 窗口尺寸：九身份网格下扫过卡片四角后，身份顺序、同一行的相对次序与窗口
// 尺寸都不变（卡片会因悬停而重新分配宽度，但顺序与列数不变）。
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

	// 顺序与列数不变：每一行内卡片仍然从左到右排列，行内互不重叠。
	for i, p := range app.profiles {
		box, ok := view.Find(p.Name)
		if !ok {
			t.Fatalf("profile %q disappeared while tilting", p.Name)
		}
		if col := i % 3; col > 0 {
			prev := mustFindRect(t, view, app.profiles[i-1].Name)
			if box.X < prev.X {
				t.Fatalf("hover reordered row %d: %q is now left of %q", i/3, p.Name, app.profiles[i-1].Name)
			}
			if box.X+0.5 < prev.X+prev.W {
				t.Fatalf("cards in row %d overlap: %+v and %+v", i/3, prev, box)
			}
		}
		// 交叉轴放大只吃行间距，卡片不得侵入相邻行。
		if row := i / 3; row > 0 {
			above := mustFindRect(t, view, app.profiles[(row-1)*3].Name)
			if box.Y+0.5 < above.Y+above.H {
				t.Fatalf("hovered card %q overlaps the row above: %+v and %+v", p.Name, above, box)
			}
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
