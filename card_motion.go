// [INPUT]: 依赖 mygo ui 的 PointerPosition / LayoutBounds / AnimateTo / Transform / Preferences；不读 selectorApp 状态。
// [OUTPUT]: 提供身份卡片的悬停 3D 倾斜与内层视差位移（指针归一化、钳制、弹簧跟随、ReduceMotion 归零），供 profile_view.go 消费。
// [POS]: 根包的动效层分片，纯装饰：不写选择、不改布局、不改命中区域与滚动位置。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import "github.com/egoist/mygo/ui"

// 倾斜参数移植自 Vanilla Tilt 1.8.1（max 15°、perspective 1000px、scale 1）
// 与 Atropos（rotateXMax/rotateYMax 15、activeOffset 50、children 反向视差），
// 但按本工具的卡片尺寸做了适配——这里写的是适配值，不是来源默认值：
//
//   - tiltMax 5.5°：来源面向展示型大卡片；本卡片只有 56–60pt 高、每天高频
//     使用，15° 会让文字在小卡片上明显斜切，5.5° 只留下可读的透视感。
//   - perspective 700：来源的 1000 面向大卡片；小卡片用更短的观察距离才能
//     得到看得出的深度，否则几乎读不出是 3D。
//   - hoverScale 1.01：Atropos 用 translateZ 把卡片抬起来，MyGo 没有公开的
//     TranslateZ，用 1% 均匀缩放近似同样的"离屏幕更近"的感觉。
//   - 内层视差 1.5pt / 徽标 2pt：Atropos 的 data-atropos-offset 是百分比，
//     换算到 56–60pt 的卡片上只剩 1–2pt 的可辨范围；徽标略强一档，因为它
//     是最靠近边缘的元素，需要比文字多一点深度差。
const (
	// cardTiltMax 是卡片的最大倾斜角（度）。
	cardTiltMax float32 = 5.5
	// cardTiltPerspective 是透视距离的兜底值（pt），只在拿不到卡片宽度时
	// 使用；正常路径见 cardPerspective。
	cardTiltPerspective float32 = 700
	// cardPerspectiveRatio 是透视距离与卡片宽度之比。见 cardPerspective 的
	// 推导：只有这个比值决定相对形变，因此固定像素值的透视（Vanilla Tilt
	// 的 1000px）只对某一种卡片宽度成立。
	cardPerspectiveRatio float32 = 2.5
	// cardHoverScale 是悬停时内层视觉容器的均匀缩放。
	cardHoverScale float32 = 1.01
	// cardParallaxContent 是头像与文字列的最大反向位移（pt）。
	cardParallaxContent float32 = 1.5
	// cardParallaxBadge 是数字键徽标的最大反向位移（pt）。
	cardParallaxBadge float32 = 2
	// cardParallaxBadgeRatio 把内容位移换算成徽标位移。两个操作数都是非零
	// 常量，因此这是编译期常量除法，不存在运行时除零。
	cardParallaxBadgeRatio float32 = cardParallaxBadge / cardParallaxContent
)

// cardPerspective 返回宽度为 width 的卡片应使用的透视距离。
//
// 推导：把卡片宽度 W 归一化，u = x/W ∈ [-1/2, 1/2]。绕 y 轴转 θ 后
// z = -x·sinθ = -u·W·sinθ，透视投影的分母 w = 1 - z/d = 1 + u·sinθ/k，
// 其中 k = d/W。于是
//
//	u' = u·cosθ / (1 + u·sinθ/k)
//
// 只含 θ 与 k：相对形变与卡片像素宽度无关，只取决于"透视距离是宽度的
// 几倍"。取 u = 1/2 即得峰值处的比例 cosθ·k/(k + sinθ/2)。
//
// 所以固定 700pt（或 Vanilla Tilt 的 1000px）对 604pt 宽的单列卡只有
// 1.16×宽——峰值角点位移 15.9pt，明显过强；对 192pt 宽的网格卡是
// 3.65×宽——只有 2.2pt，几乎看不出。改成 d = k·W 后，两种宽度得到同一
// 强度。这是对 Vanilla Tilt 固定像素透视的有意偏离。
//
// 宽度未知（<=0，例如首帧还没布局）时回退到 cardTiltPerspective。
func cardPerspective(width float32) float32 {
	if width <= 0 {
		return cardTiltPerspective
	}
	return cardPerspectiveRatio * width
}

// cardTiltSpring 是指针跟随与回中用的弹簧：可被新的目标打断，指针快速
// 扫过卡片时不会出现"排队等动画"的滞后感。与按压缩放（view_style.go 的
// springPress）分开，两者互不覆盖。
func cardTiltSpring() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Stiffness: 260, Damping: 30}}
}

// cardTiltTarget 把指针位置换算成这一帧的目标角度、缩放与视差位移。
//
// px、py 是元素盒内的局部坐标（PointerPosition），w、h 是布局盒尺寸。
// nx、ny 把指针归一化到 [-1, 1] 并钳制，越界（指针在盒外）不会放大角度。
//
// 方向与 Vanilla Tilt 1.8.1 的 getValues() 逐项等价：来源里
// tiltX = max - x*max*2 用于 rotateY，tiltY = y*max*2 - max 用于 rotateX。
// 换成以中心为原点的 nx = 2x-1、ny = 2y-1 后，rotateY = -nx*max、
// rotateX = ny*max。于是指针在左上角时 rotateY 为正（右边缘远离、左边缘
// 靠近）、rotateX 为负（上边缘靠近），卡片朝指针所在一侧倾过来。
//
// 未悬停、ReduceMotion 或尺寸未知时，目标全部归零：ReduceMotion 下既不
// 倾斜也不产生任何 3D 位移，而不是"跳变到目标"。
//
// 返回的 shiftX、shiftY 是"指针一侧"的位移量，调用方取反后施加给内层
// 子元素，得到 Atropos children 那样与倾斜相反方向的视差。
func cardTiltTarget(px, py, w, h float32, over, reduceMotion bool) (rotX, rotY, scale, shiftX, shiftY float32) {
	if !over || reduceMotion || w <= 0 || h <= 0 {
		return 0, 0, 1, 0, 0
	}
	nx := clampUnit(2*px/w - 1)
	ny := clampUnit(2*py/h - 1)
	return ny * cardTiltMax,
		-nx * cardTiltMax,
		cardHoverScale,
		nx * cardParallaxContent,
		ny * cardParallaxContent
}

// clampUnit 把 v 钳制到 [-1, 1]。
func clampUnit(v float32) float32 {
	return min(1, max(-1, v))
}

// cardTilt 读取外壳元素的指针位置，平滑跟随，并返回这一帧的倾斜变换与
// 内层视差位移（1.5pt 档，徽标按 cardParallaxBadge/cardParallaxContent
// 放大）。
//
// 变换链按 CSS 的等价顺序组合：CSS 的
// `perspective(P) rotateX(rx) rotateY(ry) scale3d(s,s,s)` 从左到右相乘为
// P*Rx*Ry*S，而 MyGo 的 Then 是"先应用前者"，所以这里写成
// RotationY().Then(RotationX()).Then(Scaling()).Then(Perspective())。
// 均匀缩放与旋转可交换，因此与来源的书写顺序结果一致。
//
// 所有动画通道都挂在**外壳**元素上：外壳有稳定 Key，跨帧是同一个元素，
// 弹簧不会被重建；变换本身只施加给内层视觉容器，外壳的盒子、命中区域、
// 详情浮层锚点与滚动位置都不变。
func cardTilt(outer ui.Element, c *ui.Context) (tilt ui.Transform, shiftX, shiftY float32) {
	px, py, over := outer.PointerPosition()
	box := outer.LayoutBounds() // 上一帧的布局盒，未受任何变换影响
	rotX, rotY, scale, shiftX, shiftY := cardTiltTarget(px, py, box.W, box.H, over, c.Preferences().ReduceMotion)

	a := cardTiltSpring()
	rotX = outer.AnimateTo("tiltX", rotX, a)
	rotY = outer.AnimateTo("tiltY", rotY, a)
	scale = outer.AnimateTo("tiltScale", scale, a)
	shiftX = outer.AnimateTo("tiltShiftX", shiftX, a)
	shiftY = outer.AnimateTo("tiltShiftY", shiftY, a)

	if rotX == 0 && rotY == 0 && scale == 1 && shiftX == 0 && shiftY == 0 {
		// 完全归中：不设变换，避免为静止状态付出任何绘制代价。
		return ui.Transform{}, 0, 0
	}
	tilt = ui.RotationY(rotY).
		Then(ui.RotationX(rotX)).
		Then(ui.Scaling(scale, scale)).
		Then(ui.Perspective(cardPerspective(box.W)))
	return tilt, shiftX, shiftY
}
