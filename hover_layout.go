// [INPUT]: 只依赖 mygo ui 的 Rect/Transform 与 layout.go 的列表内缩常量；不读指针、不读应用状态。
// [OUTPUT]: 提供悬停放大与邻居让位的纯几何：把"带状"成员（网格的同一行，或单列的整列）沿带状轴重新分配，悬停卡放大到 hoverGrow（网格 1.06）/hoverGrowTall（单列 1.08），邻居按 1/(1+d) 衰减缩窄并向反方向移位，带首尾外边界与卡片间距保持不变；结果写入调用方提供的 dst（不分配、不共享可变状态），供 profile_view.go 的交互层施加。
// [POS]: 根包的悬停几何分片，纯函数：输出只取决于入参，无包级可变状态，多 tester 与并行测试互不影响。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import "github.com/egoist/mygo/ui"

const (
	// hoverGrow 是网格悬停卡沿带状轴（同一行，x）的放大倍数，同时也是它
	// 交叉轴（y）的目标倍数。这是相对**静止尺寸**的放大：静止状态恒为 1，
	// 不存在"先把所有卡缩到 0.94、悬停时再恢复成 1"的假放大。
	hoverGrow float32 = 1.06
	// hoverGrowTall 是单列悬停卡沿带状轴（整列，y）的放大倍数。单列的宽卡
	// 横向没有余量（只剩列表内缩），放大因此主要落在高度上：1.08 的高度由
	// 上下邻居各让 4% 兑现，横向只吃内缩余量（约 1.008–1.02）。
	hoverGrowTall float32 = 1.08
	// hoverCrossPad 是交叉轴放大必须给倾斜留出的余量（pt）。悬停卡的内层
	// 表面还在做 5.5° 倾斜，透视会把它的外接矩形再撑大 2pt 左右；中间行
	// 紧邻的是 6pt 行间距、首末行紧邻的是 5pt 列表内缩，因此交叉轴放大只能
	// 吃到"可用间距减去这份余量"，否则倾斜后的外接矩形会压到邻行。
	hoverCrossPad float32 = 2.5
)

// hoverShift 是一张卡相对静止布局的变换：绕盒子中心缩放 (SX, SY)，再位移
// (DX, DY)。两者都由 hoverLayout 给出，静止时恒为 (1, 1, 0, 0)。
type hoverShift struct {
	SX, SY float32
	DX, DY float32
}

// hoverLayout 计算悬停放大与邻居让位，把结果写进 dst。
//
// slots 是静止布局盒（ui.Element.LayoutBounds：上一帧 border-box，不含任何
// 变换，滚动与悬停都不会让它漂移），按渲染顺序排列；cols 是实际列数（1 表示
// 单列）；gap 是相邻卡片的静止间距；hovered 是悬停卡下标，-1 表示没有悬停。
// dst 由调用方提供（至少 len(slots) 长），本函数不分配内存、不持有状态，返回
// dst[:len(slots)]。
//
// 带状（band）定义：
//   - cols > 1：悬停卡所在的那一行，带状轴是 x；
//   - cols == 1：整列，带状轴是 y。
//
// 带内有邻居时只重新分配带状轴上的空间：带首的起点与带尾的外边界固定、卡片
// 之间的 gap 固定、顺序不变，因此相邻卡片的矩形永远互不相交，放大区不会盖住
// 任何邻居。缺口由同带邻居按 1/(1+d) 的距离权重分担——近邻让得多、远邻让得
// 少，而不是整排等比缩小。悬停卡某一侧没有邻居（首列/末列、单列首末项）时，
// 那一侧的份额全部由另一侧承担，也就是"边缘向内锚定"。
//
// 带内只有它自己（仅一个身份、或末行只剩一张）时没有任何人可以让位，带状轴
// 的外边界就是这张卡自己的两条边，所以带状轴不放大，只保留交叉轴的轻微放大：
// 孤卡因此不会越过任何边界，也不会假装挤压了谁。
//
// 交叉轴只放大悬停卡自己，同样受真实可用空间限制：网格行由行间距减去倾斜
// 余量吸收，单列由列表内缩吸收（所以单列横向只有约 1.008–1.02，绝不越过
// 滚动视口的裁切边）。末行不足时带状只含真实存在的卡片，空槽不参与。
//
// reduceMotion、下标越界、或布局盒尚未就绪（宽高非正，例如首帧）时全部写入
// 静止值：减少动态效果下不产生任何新的几何位移。
func hoverLayout(dst []hoverShift, slots []ui.Rect, cols int, gap float32, hovered int, reduceMotion bool) []hoverShift {
	out := dst[:len(slots)]
	for i := range out {
		out[i] = hoverShift{SX: 1, SY: 1}
	}
	if reduceMotion || cols < 1 || hovered < 0 || hovered >= len(slots) || gap < 0 {
		return out
	}
	for i := range slots {
		if slots[i].W <= 0 || slots[i].H <= 0 {
			return out
		}
	}

	alongX := cols > 1
	lo, hi := 0, len(slots)-1
	if alongX {
		lo = hovered / cols * cols
		hi = min(len(slots), lo+cols) - 1
	}

	base := slots[hovered].H
	if alongX {
		base = slots[hovered].W
	}
	factor := hoverGrowTall
	if alongX {
		factor = hoverGrow
	}
	grow := base * (factor - 1)

	if lo == hi {
		// 孤卡（只有一张身份，或末行只剩一张）：没有任何人能让位，带状轴的
		// 外边界就是它自己的两条边，因此带状轴**不放大**——只做交叉轴的轻微
		// 放大（受内缩余量约束）。这样"外边界固定"的契约在单卡情况下依然成立。
		hoverCrossAxis(out, slots, alongX, hovered, gap)
		return out
	}

	// 两侧分摊：有邻居的一侧承担；两侧都有则对半分。
	before, after := hovered-lo, hi-hovered
	var back, front float32
	switch {
	case before == 0:
		front = grow
	case after == 0:
		back = grow
	default:
		back, front = grow/2, grow/2
	}

	// 距离权重之和：两遍扫描即可分摊，不需要保存每张卡的权重。
	var wBack, wFront float32
	for i := lo; i < hovered; i++ {
		wBack += 1 / float32(1+hovered-i)
	}
	for i := hovered + 1; i <= hi; i++ {
		wFront += 1 / float32(1+i-hovered)
	}

	cur := slots[lo].X
	if !alongX {
		cur = slots[lo].Y
	}
	for i := lo; i <= hi; i++ {
		span := slots[i].W
		center := slots[i].X + slots[i].W/2
		if !alongX {
			span = slots[i].H
			center = slots[i].Y + slots[i].H/2
		}
		n := span + grow
		switch {
		case i < hovered:
			n = span - back*(1/float32(1+hovered-i))/wBack
		case i > hovered:
			n = span - front*(1/float32(1+i-hovered))/wFront
		}
		s := n / span
		d := cur + n/2 - center
		if alongX {
			out[i].SX, out[i].DX = s, d
		} else {
			out[i].SY, out[i].DY = s, d
		}
		cur += n + gap
	}

	hoverCrossAxis(out, slots, alongX, hovered, gap)
	return out
}

// hoverCrossAxis 只放大悬停卡的交叉轴，受真实可用空间约束：网格行用行间距
// （首末行用列表内缩），单列用列表内缩。
func hoverCrossAxis(out []hoverShift, slots []ui.Rect, alongX bool, hovered int, gap float32) {
	if alongX {
		slack := min(gap, 2*listShadowPad)
		if extra := min(slots[hovered].H*(hoverGrow-1), slack-hoverCrossPad); extra > 0 {
			out[hovered].SY = 1 + extra/slots[hovered].H
		}
		return
	}
	if extra := min(slots[hovered].W*(hoverGrow-1), 2*(listInset-hoverCrossPad)); extra > 0 {
		out[hovered].SX = 1 + extra/slots[hovered].W
	}
}

// hoverTransform 把一张卡的缩放与位移组合成变换：先绕盒子中心缩放，再平移
// 中心（平移不受缩放影响，所以顺序即"缩放后挪位"）。静止时返回零值变换，
// 避免为未悬停的卡片付出任何绘制代价。
func hoverTransform(s hoverShift) ui.Transform {
	if s.SX == 1 && s.SY == 1 && s.DX == 0 && s.DY == 0 {
		return ui.Transform{}
	}
	return ui.Scaling(s.SX, s.SY).Then(ui.Translation(s.DX, s.DY))
}

// hoverGallery 是一个列表容器自己的逐帧缓冲，经 ui.Local 挂在列表元素上：
// 每个 Context（每个窗口、每个 tester）各有一份，所以没有跨实例共享的可变
// 状态，并行测试互不影响。缓冲只在身份数量增长时扩容，稳态下每帧零分配。
//
// 每帧末尾把本帧的两组矩形与本帧缓冲交换，于是 slotRects/visRects 恒为
// "上一帧几何"，既不复制也不跨帧 alias。悬停判定只读这两组上一帧数据，所以
// 它是 (指针, 上一帧几何) 的纯函数——当前帧的绘制结果无法改变悬停归属。
type hoverGallery struct {
	slotRects []ui.Rect // 上一帧的静止布局盒
	visRects  []ui.Rect // 上一帧的可见矩形（已让位、互不相交）
	frameBuf  []ui.Rect // 本帧的静止布局盒（帧末与 slotRects 交换）
	frameVis  []ui.Rect // 本帧的可见矩形（帧末与 visRects 交换）
	shiftBuf  []hoverShift
	slots     []ui.Element
	layers    []ui.Element
	pressed   []bool
	filled    int
}

// rects 返回长度 n 的本帧静止布局盒缓冲（内容由调用方填入，帧末 record）。
func (g *hoverGallery) rects(n int) []ui.Rect {
	if cap(g.frameBuf) < n {
		g.frameBuf = make([]ui.Rect, n)
	}
	return g.frameBuf[:n]
}

// visible 返回长度 n 的本帧可见矩形缓冲（内容由调用方填入，帧末 record）。
func (g *hoverGallery) visible(n int) []ui.Rect {
	if cap(g.frameVis) < n {
		g.frameVis = make([]ui.Rect, n)
	}
	return g.frameVis[:n]
}

// shifts 返回长度 n 的变换缓冲。
func (g *hoverGallery) shifts(n int) []hoverShift {
	if cap(g.shiftBuf) < n {
		g.shiftBuf = make([]hoverShift, n)
	}
	return g.shiftBuf[:n]
}

// elements 返回长度 n 的元素缓冲：本帧的 slot、交互层与按下状态。这些句柄
// 只在本帧有效，绝不跨帧读取（元素可能被重建）。
func (g *hoverGallery) elements(n int) (slots, layers []ui.Element, pressed []bool) {
	if cap(g.slots) < n {
		g.slots = make([]ui.Element, n)
	}
	if cap(g.layers) < n {
		g.layers = make([]ui.Element, n)
	}
	if cap(g.pressed) < n {
		g.pressed = make([]bool, n)
	}
	return g.slots[:n], g.layers[:n], g.pressed[:n]
}

// hoverAt 返回窗口坐标 (px, py) 应当悬停的卡片下标，-1 表示没有。
//
// 判定顺序是"上一帧的可见矩形 → 上一帧的静止布局盒"：
//
//   - 可见矩形由 hoverLayout 给出，同带内已经重新分配过空间、彼此不相交、
//     间距仍是静止间距，因此**放大出来的那一圈区域只属于放大它的那张卡**。
//     若只用静止布局盒判定，放大后的边缘会落进邻居的静止盒（放大 6% 的
//     一半正好超过 6pt 间距），指针一停在边缘悬停就跳到邻居身上，表现为
//     放大卡突然缩回——这正是原生 smoke 里卡宽从 229 掉到 210 的原因。
//   - 静止盒只作为兜底（首帧、或指针落在可见矩形之外但在卡内），保证悬停
//     从第一帧起就能建立。
//
// 两个来源都只依赖"上一帧的布局/动画值"，与当前帧的绘制无关，所以同一指针
// 位置在连续帧里给出同一个答案，动画不会自己把悬停切走。
func (g *hoverGallery) hoverAt(px, py float32) int {
	if g.filled == 0 {
		return -1 // 首帧还没有布局盒
	}
	if i := rectAt(g.visRects, px, py); i >= 0 {
		return i
	}
	return rectAt(g.slotRects, px, py)
}

// rectAt 返回包含该点的第一个有效矩形下标，-1 表示没有。
func rectAt(rects []ui.Rect, px, py float32) int {
	for i, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		if px >= r.X && px < r.X+r.W && py >= r.Y && py < r.Y+r.H {
			return i
		}
	}
	return -1
}

// record 在帧末把本帧的静止布局盒与可见矩形提升为"上一帧几何"。
//
// 直接交换缓冲而不是复制：本帧写的是 frameBuf/frameVis，上一帧的
// slotRects/visRects 因此成为下一帧的写入目标。悬停判定（hoverAt）只在下一次
// record 之前读取，所以不存在跨帧 alias：被换下去的那两片在本帧已无人读取，
// 下一帧由 rects/visible 重新切片后整段覆盖写入。
func (g *hoverGallery) record(slots, visible []ui.Rect) {
	// 换下去的缓冲长度清零：身份数量变少时，旧的多余条目绝不能被
	// hoverAt 当成"上一帧还有这些卡"读到。容量保留，下一帧直接复用。
	g.slotRects, g.frameBuf = slots, g.slotRects[:0]
	g.visRects, g.frameVis = visible, g.visRects[:0]
	g.filled = len(slots)
}

// visibleRect 返回一张卡在给定变换下的可见矩形（绕中心缩放后平移）。
func visibleRect(r ui.Rect, s hoverShift) ui.Rect {
	w, h := r.W*s.SX, r.H*s.SY
	cx, cy := r.X+r.W/2+s.DX, r.Y+r.H/2+s.DY
	return ui.Rect{X: cx - w/2, Y: cy - h/2, W: w, H: h}
}
