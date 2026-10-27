// [INPUT]: 依赖 ui.Color 与 ui.Animation 类型，由 view.go 和 profile_view.go 消费。
// [OUTPUT]: 提供浅色/深色 × 原生磨砂/不透明（高对比或降低透明度）× 的调色板（目标卡、身份卡片、悬停/选中薄底、头像描边、数字徽标与卡片/浮层投影），以及全局动效常量。
// [POS]: 根包的视觉层样式表，不含任何状态或布局逻辑。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// 内容入场 180ms、出场 120ms；控件反馈与按压保持短促。
// 键盘导航本身不做过渡——它必须立即生效。
const (
	motionEntry    = 180 * time.Millisecond // 内容到位
	motionExit     = 120 * time.Millisecond // 内容离场
	motionFeedback = 140 * time.Millisecond // 背景/边框颜色
	motionPress    = 160 * time.Millisecond // 按压缩放
)

// palette 是一帧内使用的全部颜色。半透明值只在窗口真的有材质
// （c.Vibrancy()）且系统未要求降低透明度、也未要求高对比时使用；否则回落
// 到不透明表面，文字永远保持不透明。
type palette struct {
	// opaque 表示本帧使用实色表面：卡片不再叠加阴影，避免不透明背景上
	// 出现浑浊的灰边。
	opaque bool

	window      ui.Color // 根背景以主题色轻覆原生材质，保持文字对比
	destination ui.Color // 顶部目标地址卡
	popover     ui.Color // 空格详情浮层：比地址卡更实，保证压在卡片上仍可读
	cardShadow  ui.Color // 身份卡片的极浅投影，只在选中/悬停时使用
	popShadow   ui.Color // 空格详情浮层的投影，比卡片更深一档

	rowNormal ui.Color // 身份卡片常态：几乎不可见的薄底
	rowHover  ui.Color // 指针悬停的一层薄覆盖
	rowChosen ui.Color // 键盘选中：主题色薄底
	rowBorder ui.Color // 身份卡片常态描边：正常外观完全透明，高对比时显形
	border    ui.Color

	accent      ui.Color
	textPrimary ui.Color
	textSecond  ui.Color
	textMuted   ui.Color

	avatarRing ui.Color // 头像细描边，把头像从卡片底上分离

	shortcutText  ui.Color // 右侧数字键徽标：低调到不与地址争注意力
	shortcutBg    ui.Color // 徽标常态薄底
	shortcutOnBg  ui.Color // 选中卡片的徽标：主题色实心
	shortcutOnTxt ui.Color // 选中卡片的徽标文字

	danger    ui.Color
	segBg     ui.Color
	segActive ui.Color
	glyph     ui.Color // 关闭按钮的笔画
}

// newPalette 按外观、窗口材质与无障碍偏好选择一组颜色。
//
// ui.Context 只暴露高对比；"降低透明度"由 view.go 从 mygo.Theme 读取后传入。
// 二者任一成立、或窗口根本没有材质时，全部表面改为实色并去掉投影——投影
// 在不透明底上只会脏成灰边，而实色本身已经给出足够的层级。根背景仅以薄层
// 主题色覆盖，保留原生磨砂；手动主题由 view.go 同步原生外观。
func newPalette(dark, vibrancy, highContrast, reduceTransparency bool) palette {
	opaque := !vibrancy || highContrast || reduceTransparency

	p := palette{opaque: opaque}
	if dark {
		p.textPrimary = ui.Hex("#F2F4F8")
		p.textSecond = ui.Hex("#CDD3DE")
		p.textMuted = ui.Hex("#AEB6C2")
		p.accent = ui.Hex("#4C9AFF")
		p.danger = ui.Hex("#FF6B5E")
		p.glyph = ui.Hex("#C8CDD6")
		if opaque {
			p.window = ui.Hex("#16181D")
			p.destination = ui.Hex("#1F232A")
			p.popover = ui.Hex("#262B34")
			p.rowNormal = ui.Hex("#1A1D24")
			p.rowHover = ui.Hex("#23272F")
			p.rowChosen = ui.Hex("#1E2C41")
			p.segBg = ui.Hex("#22262E")
			p.segActive = ui.Hex("#333945")
			p.shortcutBg = ui.Hex("#262B34")
			p.cardShadow = ui.Transparent
			p.popShadow = ui.RGBA(0, 0, 0, 0.32)
		} else {
			p.window = ui.RGBA(22, 24, 29, 0.18)
			p.destination = ui.RGBA(255, 255, 255, 0.055)
			p.popover = ui.RGBA(30, 34, 41, 0.78)
			p.rowNormal = ui.RGBA(255, 255, 255, 0.032)
			p.rowHover = ui.RGBA(255, 255, 255, 0.075)
			p.rowChosen = ui.RGBA(76, 154, 255, 0.18)
			p.segBg = ui.RGBA(255, 255, 255, 0.07)
			p.segActive = ui.RGBA(255, 255, 255, 0.16)
			p.shortcutBg = ui.RGBA(255, 255, 255, 0.09)
			p.cardShadow = ui.RGBA(0, 0, 0, 0.22)
			p.popShadow = ui.RGBA(0, 0, 0, 0.28)
		}
		if highContrast {
			p.border = ui.RGBA(255, 255, 255, 0.28)
			p.rowBorder = p.border
			p.avatarRing = ui.RGBA(255, 255, 255, 0.34)
			p.textSecond = ui.Hex("#C6CCD6")
			p.textMuted = ui.Hex("#A2A9B4")
		} else {
			p.border = ui.RGBA(255, 255, 255, 0.10)
			p.rowBorder = ui.Transparent
			p.avatarRing = ui.RGBA(255, 255, 255, 0.18)
		}
		p.shortcutText = ui.Hex("#A0A8B4")
		p.shortcutOnBg = p.accent
		p.shortcutOnTxt = ui.Hex("#0B1220")
		return p
	}

	p.textPrimary = ui.Hex("#16181D")
	p.textSecond = ui.Hex("#434D5A")
	p.textMuted = ui.Hex("#5A636F")
	p.accent = ui.Hex("#2563EB")
	p.danger = ui.Hex("#C0392B")
	p.glyph = ui.Hex("#4B5058")
	if opaque {
		p.window = ui.Hex("#F6F6F7")
		p.destination = ui.Hex("#FFFFFF")
		p.popover = ui.Hex("#FFFFFF")
		p.rowNormal = ui.Hex("#F8F8FA")
		p.rowHover = ui.Hex("#EDEFF4")
		p.rowChosen = ui.Hex("#E8F0FE")
		p.segBg = ui.Hex("#E7E9EE")
		p.segActive = ui.Hex("#FFFFFF")
		p.shortcutBg = ui.Hex("#EDEFF3")
		p.cardShadow = ui.Transparent
		p.popShadow = ui.RGBA(0, 0, 0, 0.18)
	} else {
		p.window = ui.RGBA(246, 246, 247, 0.18)
		p.destination = ui.RGBA(255, 255, 255, 0.62)
		p.popover = ui.RGBA(255, 255, 255, 0.92)
		p.rowNormal = ui.RGBA(255, 255, 255, 0.10)
		p.rowHover = ui.RGBA(255, 255, 255, 0.34)
		p.rowChosen = ui.RGBA(37, 99, 235, 0.12)
		p.segBg = ui.RGBA(0, 0, 0, 0.055)
		p.segActive = ui.RGBA(255, 255, 255, 0.85)
		p.shortcutBg = ui.RGBA(0, 0, 0, 0.05)
		p.cardShadow = ui.RGBA(0, 0, 0, 0.10)
		p.popShadow = ui.RGBA(0, 0, 0, 0.14)
	}
	if highContrast {
		p.border = ui.RGBA(0, 0, 0, 0.26)
		p.rowBorder = p.border
		p.avatarRing = ui.RGBA(0, 0, 0, 0.32)
		p.textSecond = ui.Hex("#3A3F47")
		p.textMuted = ui.Hex("#4E545C")
	} else {
		p.border = ui.RGBA(0, 0, 0, 0.085)
		p.rowBorder = ui.Transparent
		p.avatarRing = ui.RGBA(0, 0, 0, 0.14)
	}
	p.shortcutText = ui.Hex("#78808A")
	p.shortcutOnBg = p.accent
	p.shortcutOnTxt = ui.Hex("#FFFFFF")
	return p
}

// springPress 是按压/抬起使用的快弹簧：无回弹，160ms 内到位，
// 且能被新的目标打断——指针按住不放时不会出现"必须等动画结束"。
func springPress() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: motionPress, Bounce: 0}}
}

// springEntry 用原生无回弹弹簧令内容到位，不改变布局或阻塞输入。
func springEntry() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: motionEntry, Bounce: 0}}
}

// springExit 与入场共用原生弹簧语义，真正完成后才退出，不按时钟猜测完成。
func springExit() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: motionExit, Bounce: 0}}
}

// fadeColors 让背景与边框颜色平滑过渡，位置与尺寸不参与，因此不会因为
// 重排而出现跟随滚动条的错位动画。
//
// 只用于控件层（关闭按钮、主题分段开关）：这些控件的变化来自指针，慢一帧
// 无妨。身份卡片的选择与悬停绝不使用它——键盘选择必须在按下的那一帧就
// 可见；卡片的投影也随状态瞬时切换，不参与过渡。
func fadeColors() ui.ElementTransition {
	return ui.ElementTransition{Duration: motionFeedback, Colors: true}
}
