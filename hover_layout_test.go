// [INPUT]: 依赖 hover_layout.go 的纯几何与 layout.go 的卡片尺寸常量；不接触窗口、Chrome 或指针。
// [OUTPUT]: 约束悬停分配的守恒性质——放大相对静止尺寸、外边界固定、间距固定、同带矩形互斥、边缘向内、孤卡不越界、ReduceMotion 无几何变化。
// [POS]: 根包的悬停几何回归入口，与 hover_ui_test.go 的消费者回归并列。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// hoverSlotsFor 造出 count 张卡的静止布局盒，与真实网格/单列一致。
func hoverSlotsFor(count, cols int, width, height, gap float32) []ui.Rect {
	out := make([]ui.Rect, count)
	inner := width - 2*listInset
	cardW := inner
	if cols > 1 {
		cardW = (inner - float32(cols-1)*gap) / float32(cols)
	}
	for i := range out {
		col, row := i%cols, i/cols
		out[i] = ui.Rect{
			X: listInset + float32(col)*(cardW+gap),
			Y: listShadowPad + float32(row)*(height+gap),
			W: cardW,
			H: height,
		}
	}
	return out
}

// applyShift 把一张卡的变换作用到它的静止盒上，得到可见矩形。
func applyShift(r ui.Rect, s hoverShift) ui.Rect {
	w, h := r.W*s.SX, r.H*s.SY
	cx, cy := r.X+r.W/2+s.DX, r.Y+r.H/2+s.DY
	return ui.Rect{X: cx - w/2, Y: cy - h/2, W: w, H: h}
}

// hoveredShift 是便捷读取：把 hoverLayout 的结果写进临时缓冲并返回下标项。
func hoveredShift(t *testing.T, slots []ui.Rect, cols int, gap float32, hovered int) ([]hoverShift, hoverShift) {
	t.Helper()
	out := make([]hoverShift, len(slots))
	hoverLayout(out, slots, cols, gap, hovered, false)
	return out, out[hovered]
}

// TestHoverLayoutGrowsHoveredCardRelativeToRest 是"不得退化成先缩小再恢复"的
// 守卫：静止状态必须是 identity，悬停卡必须真的比静止尺寸大。
func TestHoverLayoutGrowsHoveredCardRelativeToRest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		count      int
		cols       int
		width, gap float32
		height     float32
		axis       string
	}{
		{"grid middle", 9, 3, 700, profileGridGap, profileCardHeight, "x"},
		{"single column", 3, 1, 460, rowGap, rowHeight, "y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slots := hoverSlotsFor(tc.count, tc.cols, tc.width, tc.height, tc.gap)

			rest := make([]hoverShift, len(slots))
			hoverLayout(rest, slots, tc.cols, tc.gap, -1, false)
			for i, s := range rest {
				if s != (hoverShift{SX: 1, SY: 1}) {
					t.Fatalf("no hover must be identity, card %d got %+v", i, s)
				}
			}

			target := min(4, tc.count-1)
			_, got := hoveredShift(t, slots, tc.cols, tc.gap, target)
			if tc.axis == "x" {
				if got.SX <= 1.05 {
					t.Fatalf("hovered card must grow along the band axis: SX=%v", got.SX)
				}
				if got.SY <= 1 {
					t.Fatalf("hovered card must grow on the cross axis too: SY=%v", got.SY)
				}
			} else {
				if got.SY <= 1.05 {
					t.Fatalf("hovered card must grow taller: SY=%v", got.SY)
				}
				if got.SX <= 1 {
					t.Fatalf("hovered card must still widen within the list inset: SX=%v", got.SX)
				}
				if got.SX > 1.03 {
					t.Fatalf("single-column widening must stay inside the inset slack: SX=%v", got.SX)
				}
			}
		})
	}
}

// TestHoverLayoutKeepsBandEdgesAndGaps 约束带首尾外边界与卡片间距恒定：这是
// "不遮挡、不裁切"的几何来源。
func TestHoverLayoutKeepsBandEdgesAndGaps(t *testing.T) {
	const cols, gap = 3, profileGridGap
	slots := hoverSlotsFor(9, cols, 700, profileCardHeight, gap)

	for hovered := 0; hovered < 9; hovered++ {
		shifts, _ := hoveredShift(t, slots, cols, gap, hovered)
		row := hovered / cols * cols
		end := min(len(slots), row+cols)

		first := applyShift(slots[row], shifts[row])
		last := applyShift(slots[end-1], shifts[end-1])
		if absf(first.X-slots[row].X) > 0.01 {
			t.Fatalf("hovered %d moved the row's left edge: %v → %v", hovered, slots[row].X, first.X)
		}
		right := slots[end-1].X + slots[end-1].W
		if absf(last.X+last.W-right) > 0.01 {
			t.Fatalf("hovered %d moved the row's right edge: %v → %v", hovered, right, last.X+last.W)
		}

		for i := range end - 1 - row {
			a, b := applyShift(slots[row+i], shifts[row+i]), applyShift(slots[row+i+1], shifts[row+i+1])
			if got := b.X - (a.X + a.W); absf(got-gap) > 0.01 {
				t.Fatalf("hovered %d changed the gap after card %d: %v", hovered, row+i, got)
			}
		}
	}
}

// TestHoverLayoutNeighboursShrinkByDistance 约束"按邻近关系让位"而不是整排
// 等比缩小：近邻让得比远邻多，且每一张都比静止小。
func TestHoverLayoutNeighboursShrinkByDistance(t *testing.T) {
	const cols, gap = 3, profileGridGap
	slots := hoverSlotsFor(9, cols, 700, profileCardHeight, gap)
	shifts, _ := hoveredShift(t, slots, cols, gap, 1) // 中间列，两侧各一张

	left, right := shifts[0], shifts[2]
	if left.SX >= 1 || right.SX >= 1 {
		t.Fatalf("both neighbours must shrink: left=%v right=%v", left.SX, right.SX)
	}
	if left.SX != right.SX {
		t.Fatalf("symmetric neighbours must shrink equally: %v vs %v", left.SX, right.SX)
	}

	// 单列：整列都是邻居，越远让得越少。
	single := hoverSlotsFor(5, 1, 460, rowHeight, rowGap)
	col, _ := hoveredShift(t, single, 1, rowGap, 2)
	near, far := col[1], col[0]
	if near.SY >= 1 || far.SY >= 1 {
		t.Fatalf("single-column neighbours must shrink: near=%v far=%v", near.SY, far.SY)
	}
	if near.SY > far.SY {
		t.Fatalf("the nearer neighbour must give way more: near=%v far=%v", near.SY, far.SY)
	}
}

// TestHoverLayoutRectanglesNeverOverlap 是"放大不盖住邻居"的核心断言：任何
// 悬停位置、任何列数下，同带（以及相邻行）的可见矩形都互不相交。
func TestHoverLayoutRectanglesNeverOverlap(t *testing.T) {
	for _, tc := range []struct {
		name        string
		count, cols int
		width       float32
		height, gap float32
	}{
		{"grid 9", 9, 3, 700, profileCardHeight, profileGridGap},
		{"grid 8 partial last row", 8, 3, 700, profileCardHeight, profileGridGap},
		{"grid 2 cols", 7, 2, 480, profileCardHeight, profileGridGap},
		{"single 4", 4, 1, 460, rowHeight, rowGap},
		{"singleton", 1, 1, 460, rowHeight, rowGap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slots := hoverSlotsFor(tc.count, tc.cols, tc.width, tc.height, tc.gap)
			for hovered := 0; hovered < tc.count; hovered++ {
				shifts, _ := hoveredShift(t, slots, tc.cols, tc.gap, hovered)
				boxes := make([]ui.Rect, len(slots))
				for i := range slots {
					boxes[i] = applyShift(slots[i], shifts[i])
				}
				for i := range boxes {
					for j := range len(boxes) - i - 1 {
						j += i + 1
						a, b := boxes[i], boxes[j]
						if a.X < b.X+b.W-0.01 && b.X < a.X+a.W-0.01 &&
							a.Y < b.Y+b.H-0.01 && b.Y < a.Y+a.H-0.01 {
							t.Fatalf("hovered %d: cards %d and %d overlap: %+v vs %+v", hovered, i, j, a, b)
						}
					}
				}
			}
		})
	}
}

// TestHoverLayoutEdgesAnchorInward 约束首列/末列与末行不足时的行为：放大完全
// 发生在有邻居的一侧，卡片不越过带的边界。
func TestHoverLayoutEdgesAnchorInward(t *testing.T) {
	const cols, gap = 3, profileGridGap
	slots := hoverSlotsFor(7, cols, 700, profileCardHeight, gap) // 末行只有 1 张

	// 首列：左侧不动，向右长。
	_, got := hoveredShift(t, slots, cols, gap, 3)
	box := applyShift(slots[3], got)
	if absf(box.X-slots[3].X) > 0.01 {
		t.Fatalf("first column must keep its left edge: %v → %v", slots[3].X, box.X)
	}
	if box.X+box.W <= slots[3].X+slots[3].W {
		t.Fatalf("first column must grow rightwards: %+v", box)
	}

	// 末列：右侧不动，向左长。
	_, got = hoveredShift(t, slots, cols, gap, 5)
	box = applyShift(slots[5], got)
	right := slots[5].X + slots[5].W
	if absf(box.X+box.W-right) > 0.01 {
		t.Fatalf("last column must keep its right edge: %v → %v", right, box.X+box.W)
	}

	// 末行孤卡：无人让位，带状轴不放大，只做交叉轴轻微放大，绝不越过行边界。
	_, got = hoveredShift(t, slots, cols, gap, 6)
	box = applyShift(slots[6], got)
	if got.SX != 1 {
		t.Fatalf("a lone card must not grow along the band axis: SX=%v", got.SX)
	}
	if box.X < slots[0].X-0.01 || box.X+box.W > slots[2].X+slots[2].W+0.01 {
		t.Fatalf("a lone last-row card must stay inside the row: %+v", box)
	}
}

// TestHoverLayoutReduceMotionIsIdentity 约束 ReduceMotion 下不产生任何新的
// 几何位移（静态悬停底色由 profileItem 负责，与本函数无关）。
func TestHoverLayoutReduceMotionIsIdentity(t *testing.T) {
	slots := hoverSlotsFor(9, 3, 700, profileCardHeight, profileGridGap)
	out := make([]hoverShift, len(slots))
	hoverLayout(out, slots, 3, profileGridGap, 4, true)
	for i, s := range out {
		if s != (hoverShift{SX: 1, SY: 1}) {
			t.Fatalf("ReduceMotion must not move card %d: %+v", i, s)
		}
	}
}

// TestHoverLayoutDefersUntilLaidOut 约束首帧：布局盒还没就绪时全部静止，
// 而不是按 0 宽高算出无穷缩放。
func TestHoverLayoutDefersUntilLaidOut(t *testing.T) {
	slots := hoverSlotsFor(3, 1, 460, rowHeight, rowGap)
	slots[1] = ui.Rect{}
	out := make([]hoverShift, len(slots))
	hoverLayout(out, slots, 1, rowGap, 0, false)
	for i, s := range out {
		if s != (hoverShift{SX: 1, SY: 1}) {
			t.Fatalf("unlaid-out cards must stay at rest, card %d got %+v", i, s)
		}
	}
	if got := hoverTransform(hoverShift{SX: 1, SY: 1}); got != (ui.Transform{}) {
		t.Fatalf("rest must be the zero transform")
	}
	if hoverTransform(hoverShift{SX: 1.06, SY: 1}) == (ui.Transform{}) {
		t.Fatalf("a growing card must not get the zero transform")
	}
}
