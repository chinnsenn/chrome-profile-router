// [INPUT]: 依赖 selectorApp 的真实视图、MyGo 离屏 tester 的指针/点击/帧推进，以及 hover_layout.go 的常量；不启动 Chrome。
// [OUTPUT]: 约束悬停放大与邻居让位的消费者行为——悬停卡变大、邻居让位且不重叠、放大边缘可点击、离开复位、静止指针不改写选择、ReduceMotion 无几何变化、网格/单列/孤卡边界。
// [POS]: 根包的悬停交互回归入口，与 hover_layout_test.go 的纯几何回归并列。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// hoverApp 造出 count 个身份的应用；网格需要超过 singleColumnLimit 个。
func hoverApp(count int) *selectorApp {
	app := &selectorApp{themeMode: ThemeLight}
	for i := range count {
		app.profiles = append(app.profiles, Profile{
			Dir:   fmt.Sprintf("Profile %d", i),
			Name:  fmt.Sprintf("Identity %d", i+1),
			Email: fmt.Sprintf("account%d@example.com", i+1),
			Color: ui.Hex("#3686E8"),
		})
	}
	return app
}

// rects 读出所有身份的可见盒子。
func rects(t *testing.T, view *ui.Tester, app *selectorApp) []ui.Rect {
	t.Helper()
	out := make([]ui.Rect, len(app.profiles))
	for i, p := range app.profiles {
		out[i] = mustFindRect(t, view, p.Name)
	}
	return out
}

// assertDisjoint 断言任意两个盒子都不相交（容差 0.5pt，吸收像素取整）。
func assertDisjoint(t *testing.T, boxes []ui.Rect, names []string) {
	t.Helper()
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			a, b := boxes[i], boxes[j]
			if a.X < b.X+b.W-0.5 && b.X < a.X+a.W-0.5 &&
				a.Y < b.Y+b.H-0.5 && b.Y < a.Y+a.H-0.5 {
				t.Fatalf("%q and %q overlap: %+v vs %+v", names[i], names[j], a, b)
			}
		}
	}
}

// TestHoverGrowsCardAndPushesNeighboursAside 是核心消费者回归：悬停网格中列
// 后，(1) 悬停卡相对静止尺寸真的变大（两轴），(2) 同一行两侧邻居缩窄并向
// 外移位，(3) 所有卡片互不重叠且不越出窗口。
func TestHoverGrowsCardAndPushesNeighboursAside(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	rest := rects(t, view, app)

	mid := rest[4] // 第 2 行中列
	view.Move(mid.X+mid.W/2, mid.Y+mid.H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	hovered := rects(t, view, app)

	if hovered[4].W <= rest[4].W || hovered[4].H <= rest[4].H {
		t.Fatalf("the hovered card must grow past its rest size: rest=%+v hovered=%+v", rest[4], hovered[4])
	}
	if hovered[4].W/rest[4].W < 1.05 {
		t.Fatalf("hovered card grew too little along the row: %.3f", hovered[4].W/rest[4].W)
	}
	for _, i := range []int{3, 5} {
		if hovered[i].W >= rest[i].W {
			t.Fatalf("neighbour %d must give way: rest=%+v hovered=%+v", i, rest[i], hovered[i])
		}
	}
	// 左邻居是第一列，左边缘被钉住，于是它的右边缘向左退让；右邻居对称。
	if hovered[3].X+hovered[3].W >= rest[3].X+rest[3].W {
		t.Fatalf("left neighbour must retreat rightwards: rest right=%v hovered right=%v",
			rest[3].X+rest[3].W, hovered[3].X+hovered[3].W)
	}
	if hovered[5].X <= rest[5].X {
		t.Fatalf("right neighbour must retreat leftwards: rest left=%v hovered left=%v", rest[5].X, hovered[5].X)
	}

	names := make([]string, len(app.profiles))
	for i, p := range app.profiles {
		names[i] = p.Name
	}
	assertDisjoint(t, hovered, names)
	for i, box := range hovered {
		if box.X < 0 || box.Y < 0 || box.X+box.W > 700 || box.Y+box.H > 470 {
			t.Fatalf("%q escaped the window while hovered: %+v", names[i], box)
		}
	}
}

// TestHoverSingleColumnGrowsTallerAndPushesRows 约束单列：宽卡在固定窗口里
// 无法均匀放大，放大主要落在高度上，上下邻居让位；横向只吃列表内缩。
func TestHoverSingleColumnGrowsTallerAndPushesRows(t *testing.T) {
	app := hoverApp(3)
	view := ui.NewTester(app.view, 660, 490)
	rest := rects(t, view, app)

	view.Move(rest[1].X+rest[1].W/2, rest[1].Y+rest[1].H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	hovered := rects(t, view, app)

	if hovered[1].H/rest[1].H < 1.05 {
		t.Fatalf("the hovered row must grow taller: rest=%+v hovered=%+v", rest[1], hovered[1])
	}
	if hovered[1].W <= rest[1].W {
		t.Fatalf("the hovered row must also widen within the inset: rest=%+v hovered=%+v", rest[1], hovered[1])
	}
	if hovered[1].W/rest[1].W > 1.05 {
		t.Fatalf("single-column widening must stay inside the list inset: %.3f", hovered[1].W/rest[1].W)
	}
	if hovered[0].H >= rest[0].H || hovered[2].H >= rest[2].H {
		t.Fatalf("rows above and below must give way: above=%+v below=%+v", hovered[0], hovered[2])
	}
	if hovered[0].Y+hovered[0].H > hovered[1].Y+0.5 || hovered[2].Y+0.5 < hovered[1].Y+hovered[1].H {
		t.Fatalf("single-column rows overlap: %+v %+v %+v", hovered[0], hovered[1], hovered[2])
	}
}

// TestHoverGrownEdgeIsClickable 是"点到的永远是看见的那张卡"的回归，且必须
// 覆盖最坏几何：**第一列**放大后向右伸进第二列的静止 slot。
//
// 放大 6% 的一半（约 6.8pt）大于 6pt 间距，所以第一列的可见右边缘一定落在
// 邻居的静止布局盒里。命中按"布局盒"登记，若 slot 参与登记，这一圈区域会被
// 邻居的空 slot 吃掉——原生 smoke 里表现为悬停卡从 229.7 掉回 210.2、邻居
// 反而放大。slot 因此 PassThrough，命中只由已变换的交互层决定。
//
// 启动动作经 profileListWith 注入记录回调，本测试不启动任何系统进程；断言
// 的是真实 Clicked 消费者事件（而不是 app.chosen——指针移动本身就会改 chosen）。
func TestHoverGrownEdgeIsClickable(t *testing.T) {
	app := hoverApp(9)
	var launched []string
	view := ui.NewTester(func(c *ui.Context) {
		pal := newPalette(false, false, false, false)
		snap := app.snapshot()
		app.profileListWith(c, pal, snap, nil, func(p Profile) {
			launched = append(launched, p.Name)
		})
	}, 700, 470)
	rest := rects(t, view, app)

	first := rest[0]
	view.Move(first.X+first.W/2, first.Y+first.H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	grown := mustFindRect(t, view, app.profiles[0].Name)

	// 放大后的右边缘内侧 2pt：既在可见表面内，又已在静止盒之外，并且落在
	// 第二列的静止 slot 范围内。
	x := grown.X + grown.W - 2
	if x <= first.X+first.W {
		t.Fatalf("the hovered card did not grow beyond its rest box: rest=%+v grown=%+v", first, grown)
	}
	if x < rest[1].X {
		t.Fatalf("the probe point %.2f is not inside the neighbour's rest slot (%.2f), so this case would not catch the occlusion", x, rest[1].X)
	}

	view.Move(x, grown.Y+grown.H/2)
	view.Advance(60 * time.Millisecond)
	if app.hovered != 0 {
		t.Fatalf("the pointer on the grown edge must still hover the first card: hovered=%d", app.hovered)
	}
	view.ClickAt(x, grown.Y+grown.H/2)
	view.Advance(60 * time.Millisecond)

	if len(launched) != 1 {
		t.Fatalf("clicking the grown edge must launch exactly once, got %v", launched)
	}
	if launched[0] != app.profiles[0].Name {
		t.Fatalf("clicking the grown edge launched %q, want %q", launched[0], app.profiles[0].Name)
	}
}

// TestHoverIndexStableAcrossFrames 约束悬停归属不随动画帧变化：指针停在放大
// 出来的边缘上时，连续多帧都必须是同一张卡在放大，邻居不得反转，也不得
// 因为放大本身把悬停切走（那会表现为大小来回跳）。
func TestHoverIndexStableAcrossFrames(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	rest := rects(t, view, app)

	mid := rest[4]
	view.Move(mid.X+mid.W/2, mid.Y+mid.H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	hovered := mustFindRect(t, view, app.profiles[4].Name)

	// 停在放大边缘：既在可见表面内，又已超出静止盒——最容易让命中盒摆动的
	// 位置。
	edgeX := hovered.X + hovered.W - 1
	view.Move(edgeX, hovered.Y+hovered.H/2)

	// 弹簧在收敛过程中每帧都在变，所以逐帧只断言"悬停没有切走"：放大卡始终
	// 比静止宽、邻居始终比静止窄。切走会立刻表现为两者同时回到静止值。
	var prev, curW float32
	for frame := range 12 {
		view.Advance(16 * time.Millisecond)
		cur := mustFindRect(t, view, app.profiles[4].Name)
		left := mustFindRect(t, view, app.profiles[3].Name)
		if cur.W <= rest[4].W+0.5 {
			t.Fatalf("frame %d: the hovered card fell back to its rest width, so the hover was dropped: %+v", frame, cur)
		}
		if left.W >= rest[3].W-0.5 {
			t.Fatalf("frame %d: the neighbour returned to its rest width, so the hover moved to it: %+v", frame, left)
		}
		prev, curW = curW, cur.W
	}
	// 收敛后不再抖动：最后两帧的宽度差在 1pt 内。
	if prev != 0 && absf(curW-prev) > 1 {
		t.Fatalf("the hovered card's width still oscillates after settling: %v → %v", prev, curW)
	}
}

// TestHoverLeavesRestGeometry 约束离开平滑复位：指针移出列表后所有卡片回到
// 静止几何，窗口尺寸不变。
func TestHoverLeavesRestGeometry(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	rest := rects(t, view, app)

	view.Move(rest[4].X+rest[4].W/2, rest[4].Y+rest[4].H/2)
	view.Advance(time.Second)
	view.Move(2, 2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	after := rects(t, view, app)

	for i := range rest {
		if !nearlySameRect(rest[i], after[i]) {
			t.Fatalf("card %d did not return to its rest box: rest=%+v after=%+v", i, rest[i], after[i])
		}
	}
	if w, h := view.Image().Rect.Dx(), view.Image().Rect.Dy(); w != 700 || h != 470 {
		t.Fatalf("hover resized the window: got %dx%d", w, h)
	}
}

// TestHoverReducedMotionKeepsGeometry 约束 ReduceMotion：不新增任何几何动效，
// 只保留原有静态悬停反馈（不移动、不缩放）。
func TestHoverReducedMotionKeepsGeometry(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	view.SetPreferences(ui.Preferences{ReduceMotion: true})
	rest := rects(t, view, app)

	view.Move(rest[4].X+rest[4].W/2, rest[4].Y+rest[4].H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	after := rects(t, view, app)

	for i := range rest {
		if !nearlySameRect(rest[i], after[i]) {
			t.Fatalf("ReduceMotion must not move card %d: rest=%+v after=%+v", i, rest[i], after[i])
		}
	}
}

// TestHoverSingletonAndPartialLastRow 约束边界：只有一个身份时不放大（没有
// 人可以交换空间），末行只剩一张时同样不越过行边界；两种情况下所有卡片都
// 保持在窗口内且互不重叠。
func TestHoverSingletonAndPartialLastRow(t *testing.T) {
	single := hoverApp(1)
	view := ui.NewTester(single.view, 660, 490)
	rest := mustFindRect(t, view, single.profiles[0].Name)
	view.Move(rest.X+rest.W/2, rest.Y+rest.H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	// 只有一个身份时没有人能让位：主轴（单列的高度）必须一动不动，横向只
	// 允许吃列表内缩的那点余量。
	got := mustFindRect(t, view, single.profiles[0].Name)
	if absf(got.H-rest.H) > 0.5 || absf(got.Y-rest.Y) > 0.5 {
		t.Fatalf("a lone identity must not grow along its band axis: rest=%+v hovered=%+v", rest, got)
	}
	if got.W/rest.W > 1.02 || got.X < 0 || got.X+got.W > 660 {
		t.Fatalf("a lone identity may only grow within the list inset: rest=%+v hovered=%+v", rest, got)
	}

	partial := hoverApp(7)
	view = ui.NewTester(partial.view, 700, 470)
	before := rects(t, view, partial)
	last := before[6] // 末行唯一一张
	view.Move(last.X+last.W/2, last.Y+last.H/2)
	view.Advance(time.Second)
	view.Advance(time.Second)
	after := rects(t, view, partial)

	names := make([]string, len(partial.profiles))
	for i, p := range partial.profiles {
		names[i] = p.Name
	}
	assertDisjoint(t, after, names)
	if after[6].X < before[0].X-0.5 || after[6].X+after[6].W > before[2].X+before[2].W+0.5 {
		t.Fatalf("a lone last-row card must stay inside its row: rest=%+v hovered=%+v", before[6], after[6])
	}
	if !nearlySameRect(before[6], after[6]) && after[6].H > before[6].H+1 {
		t.Logf("lone last-row card only grew on the cross axis: rest=%+v hovered=%+v", before[6], after[6])
	}
}

// TestHoverStationaryPointerKeepsKeyboardChoice 约束静止指针：悬停放大进行中
// 按方向键，弹簧与放大都不得把选择改回去。
func TestHoverStationaryPointerKeepsKeyboardChoice(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	rest := rects(t, view, app)

	view.Move(rest[4].X+rest[4].W/2, rest[4].Y+rest[4].H/2)
	view.Advance(time.Second)

	// 先让悬停把选择同步到指针下的那一行，再按键；断言的是"键按下后选择前进
	// 一格，并且整段放大动画里都不被指针改写"。
	view.Move(rest[4].X+rest[4].W/2+1, rest[4].Y+rest[4].H/2)
	view.Advance(time.Second)
	before := app.chosen
	view.Key(0, ui.KeyRight)
	if app.chosen != before+1 {
		t.Fatalf("the key press must apply in its own frame: got %d, want %d", app.chosen, before+1)
	}
	want := app.chosen
	for range 8 {
		view.Advance(20 * time.Millisecond)
		if app.chosen != want {
			t.Fatalf("hover animation overwrote the keyboard choice: got %d, want %d", app.chosen, want)
		}
	}
}

// TestHoverAcrossCardsNeverOverlaps 约束快速扫过：连续在两个悬停目标之间移动
// 时，每一帧的可见矩形都不相交，且窗口尺寸不变。
func TestHoverAcrossCardsNeverOverlaps(t *testing.T) {
	app := hoverApp(9)
	view := ui.NewTester(app.view, 700, 470)
	names := make([]string, len(app.profiles))
	for i, p := range app.profiles {
		names[i] = p.Name
	}

	for _, idx := range []int{4, 5, 3, 1} {
		box := mustFindRect(t, view, app.profiles[idx].Name)
		view.Move(box.X+box.W/2, box.Y+box.H/2)
		for range 6 {
			view.Advance(16 * time.Millisecond)
			assertDisjoint(t, rects(t, view, app), names)
		}
	}
	if w, h := view.Image().Rect.Dx(), view.Image().Rect.Dy(); w != 700 || h != 470 {
		t.Fatalf("hover resized the window: got %dx%d", w, h)
	}
}
