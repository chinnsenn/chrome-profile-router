// [INPUT]: 依赖 ui 的 Color / ConicGradient / GradientStop / Keyframe / Local 类型与 view_style.go 的调色板。
// [OUTPUT]: 提供选中身份卡的流彩描边（5 stop 锥形渐变、6 秒线性循环角度）与外辉光（两级单色高斯阴影）的参数、按卡片归属的 stops 缓存与静态关键帧，供 profile_view.go 在 isChosen && !pal.opaque 时使用。
// [POS]: 根包的装饰参数表，只产出颜色、渐变与角度，不含布局或事件；缓存由 ui.Local 挂在调用方的元素上，因此本文件不持有任何进程级可变状态。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// 流彩描边的来源与适配（详见 README 的「选中卡流彩与柔光的来源」一节）：
//
//   - stop 位置 [0, .12, .55, .75, 1] 取自 Stripe AnimatedBorderView 的
//     CAGradientLayer.locations，原值未改；它表达「一段亮弧 + 一段渐隐」。
//   - 周期 6s、线宽 1pt 取自 Magic UI BorderBeam 的 duration / borderWidth 默认值。
//   - 暖色 #FFAA40、紫 #9C40FF 取自 BorderBeam 默认的 colorFrom / colorTo。
//
// 与来源的差异（均为本项目适配，不冒充原值）：
//
//   - 来源把渐变当「图层」用，靠 mask 只留 1px 环；这里直接用
//     BorderConicGradient 画环，不叠第二层，也不产生逐帧切片或贴图。
//   - 来源旋转整张渐变层，需要覆盖对角线的正方形；这里只改渐变的起始角度，
//     矩形本身不旋转，因此不需要那条技巧。
//   - 来源的透明 stop 会让人看不见环；这里把每个 stop 与基础选中边框
//     （accent α.55）做标准 source-over 预合成，透明处露出基础边，选中态
//     在任何相位都不会丢掉边框。
//   - 首尾 stop 取同一色，转一圈回到原点时颜色连续，接缝不跳变。
const (
	chosenBorderWidth     = 1.0             // DIP，BorderBeam borderWidth 原值
	chosenSpinPeriod      = 6 * time.Second // BorderBeam duration 原值
	chosenBaseBorderAlpha = 0.55            // 现有选中边框 accent α，作预合成底

	// Pow GlowEffect：amount 照搬原值；base radius 是适配值（Pow 默认 16）。
	// 离屏观感显示 R=4 时只有最外一圈约 46 像素有可见差异、几乎读不出辉光，
	// 因此取 16 的一半即 8，让内两级落在 1.4 / 2.8pt。仍只取半径公式的内两级。
	glowAmount     = 0.7
	glowBaseRadius = 8.0  // 适配值，非 Pow 原默认 16
	glowGain       = 0.45 // 全量 alpha 过亮，整体下调；1.2:4 的比例不变
)

// 内两级半径：amount*r/4 与 amount*r/2。
const (
	chosenGlowInnerRadius = glowAmount * glowBaseRadius / 4 // 1.40
	chosenGlowOuterRadius = glowAmount * glowBaseRadius / 2 // 2.80
)

// chosenStopPositions 是 Stripe 原值，顺序与长度不可随意改。
var chosenStopPositions = [5]float32{0, 0.12, 0.55, 0.75, 1}

// chosenSpinFrames 是共享的静态关键帧：0° → 360°，线性，无限循环。
// 360° 与 0° 是同一相位，所以循环接缝不可见；ReduceMotion 时框架直接把
// 播放完成在末帧，等价于静止在 0°。
var chosenSpinFrames = []ui.Keyframe{
	{At: 0, Value: 0},
	{At: chosenSpinPeriod, Value: 360},
}

// chosenGlowAlpha 是 Pow 的 alpha 公式：sqrt(amount)/div，再乘整体 gain。
// div 取 1.2 与 4（内两级），比例 3.3333:1 保持不变。
func chosenGlowAlpha(div float32) float32 {
	return float32(math.Sqrt(glowAmount)) / div * glowGain
}

// overColor 把 top 以标准 source-over 合成到 base 之上，两者都可以半透明。
// ui.Color.Over 要求 base 不透明，而基础选中边框是 accent α.55，所以这里
// 自己算：A = aT + aB(1-aT)，RGB 按各自权重归一化。
func overColor(top, base ui.Color) ui.Color {
	at := float32(top.A) / 255
	ab := float32(base.A) / 255
	ao := at + ab*(1-at)
	if ao <= 0 {
		return ui.Transparent
	}
	channel := func(t, b uint8) uint8 {
		v := (float32(t)*at + float32(b)*ab*(1-at)) / ao
		return uint8(math.Min(255, math.Max(0, math.Round(float64(v)))))
	}
	return ui.Color{
		R: channel(top.R, base.R),
		G: channel(top.G, base.G),
		B: channel(top.B, base.B),
		A: uint8(math.Min(255, math.Round(float64(ao*255)))),
	}
}

// chosenGradientCache 保存一张卡片上一次算好的 stops，由 ui.Local 挂在
// 该卡片自己的交互层上，因此不同窗口、不同 Tester 之间互不影响，也不存在
// 跨卡共享的可变状态。stops 是定长数组，重建时不分配切片。
type chosenGradientCache struct {
	valid bool
	key   ui.Color
	stops [5]ui.GradientStop
}

// chosenBorderGradient 组出本帧的锥形渐变。
//
// 缓存挂在 layer 上：同一张卡只在 accent 变化时重建 stops，其余帧复用同一片
// 定长数组，因此逐帧不分配、不复制。调用方只在 `isChosen && !pal.opaque`
// 时进入这里——阻断条件是 pal.opaque（高对比 / 降低透明度 / 窗口无材质）；
// ReduceMotion 不阻断，它只让角度停止推进，静态渐变与柔光照常保留。
func chosenBorderGradient(layer ui.Element, p palette, angle float32) ui.ConicGradient {
	cache := ui.Local(layer, "chosenGradient", func() chosenGradientCache {
		return chosenGradientCache{}
	})
	if !cache.valid || cache.key != p.accent {
		fillChosenStops(&cache.stops, p)
		cache.valid = true
		cache.key = p.accent
	}
	return ui.ConicGradient{Stops: cache.stops[:], Angle: angle}
}

// fillChosenStops 把 5 个 stop 与基础选中边框预合成后写入 dst。
func fillChosenStops(dst *[5]ui.GradientStop, p palette) {
	// 色相沿用 Stripe 的 alpha 结构（colors = [c1, c2@0, c2@0, c1, c1]：
	// 前两个 stop 把第二色的 alpha 压到 0，于是整圈只有一段亮弧），
	// 只把单色换成暖色与紫色两个色相：
	//   .00 暖色实心 → .12 暖色淡出（弧头收尾）
	//   .55 紫色淡出（仍不可见，与基础边预合成后露出基础边）
	//   .75 紫色实心 → 1.00 暖色实心 → 回绕到 .00 暖色
	// 首尾同为暖色，所以一圈转回起点时颜色连续。主题 accent 由基础
	// 选中边框与两级柔光体现，不塞进 stop。
	head := ui.Hex("#FFAA40") // BorderBeam colorFrom
	tail := ui.Hex("#9C40FF") // BorderBeam colorTo
	raw := [5]ui.Color{head, head.Alpha(0), tail.Alpha(0), tail, head}

	base := p.accent.Alpha(chosenBaseBorderAlpha)
	for i, c := range raw {
		dst[i] = ui.GradientStop{Color: overColor(c, base), At: chosenStopPositions[i]}
	}
}

// chosenSpin 用现有 Keyframes 调度推进角度；不需要新状态，也不自建定时器。
// 角度冻结由框架负责：ReduceMotion 时 Keyframes 直接完成到末帧，等价于
// 静止在 0°。窗口失焦时的暂停没有实现——本项目没有可用的窗口激活状态，
// 不为它扩应用状态或公共接口。
func chosenSpin(layer ui.Element) float32 {
	return layer.Keyframes("chosenSpin", chosenSpinFrames, ui.KeyframeOptions{Repeat: ui.RepeatForever})
}
