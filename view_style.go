// [INPUT]: 依赖 ui.Color 与 ui.Animation 类型，仅被 view.go 读取。
// [OUTPUT]: 提供浅色/深色 × 半透明/不透明 × 高对比的调色板，以及全局动效常量。
// [POS]: 根包的视觉层样式表，不含任何状态或布局逻辑。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// 动效时长统一落在 125–180ms：入场与颜色过渡短到不挡操作，又能被看见。
// 键盘导航本身不做过渡——它必须立即生效。
const (
	motionEntry    = 150 * time.Millisecond // 面板作为材质到位
	motionFeedback = 140 * time.Millisecond // 背景/边框颜色
	motionPress    = 160 * time.Millisecond // 按压缩放
)

// palette 是一帧内使用的全部颜色。半透明值只在窗口真的有材质
// （c.Vibrancy()）时使用；否则回落到不透明表面，文字永远保持不透明。
type palette struct {
	window      ui.Color // 根背景以主题色轻覆原生材质，保持文字对比
	destination ui.Color // 目标地址卡片
	rowNormal   ui.Color
	rowHover    ui.Color
	rowChosen   ui.Color
	border      ui.Color
	accent      ui.Color
	textPrimary ui.Color
	textSecond  ui.Color
	textMuted   ui.Color
	keycapBg    ui.Color
	keycapText  ui.Color
	danger      ui.Color
	segBg       ui.Color
	segActive   ui.Color
	glyph       ui.Color // 关闭按钮的笔画
}

// newPalette 按外观、窗口材质与高对比偏好选择一组颜色。
//
// v0.6.4 不暴露降低透明度设置；c.Vibrancy 仅表示窗口配置了原生材质。
// 原生材质的系统辅助功能行为交给 macOS，应用在高对比时使用不透明背景。
// 根背景保留主题色覆盖，避免手动主题与系统材质外观不一致时失去可读性。
func newPalette(dark, vibrancy, highContrast bool) palette {
	opaque := !vibrancy || highContrast

	var p palette
	if dark {
		p.textPrimary = ui.Hex("#F2F4F8")
		p.textSecond = ui.Hex("#A7ADBA")
		p.textMuted = ui.Hex("#7A818E")
		p.accent = ui.Hex("#4C9AFF")
		p.danger = ui.Hex("#FF6B5E")
		p.glyph = ui.Hex("#C8CDD6")
		if opaque {
			p.window = ui.Hex("#16181D")
			p.destination = ui.Hex("#20242B")
			p.rowNormal = ui.Hex("#1C2027")
			p.rowHover = ui.Hex("#262B34")
			p.rowChosen = ui.Hex("#1D2A3D")
			p.keycapBg = ui.Hex("#2A2F38")
			p.segBg = ui.Hex("#22262E")
			p.segActive = ui.Hex("#333945")
		} else {
			p.window = ui.RGBA(22, 24, 29, 0.78)
			p.destination = ui.RGBA(24, 27, 33, 0.52)
			p.rowNormal = ui.RGBA(255, 255, 255, 0.045)
			p.rowHover = ui.RGBA(255, 255, 255, 0.085)
			p.rowChosen = ui.RGBA(76, 154, 255, 0.20)
			p.keycapBg = ui.RGBA(255, 255, 255, 0.10)
			p.segBg = ui.RGBA(255, 255, 255, 0.07)
			p.segActive = ui.RGBA(255, 255, 255, 0.16)
		}
		if highContrast {
			p.border = ui.RGBA(255, 255, 255, 0.28)
			p.textSecond = ui.Hex("#C6CCD6")
			p.textMuted = ui.Hex("#A2A9B4")
		} else {
			p.border = ui.RGBA(255, 255, 255, 0.10)
		}
		p.keycapText = ui.Hex("#D6DBE4")
		return p
	}

	p.textPrimary = ui.Hex("#16181D")
	p.textSecond = ui.Hex("#5B6068")
	p.textMuted = ui.Hex("#8A8F98")
	p.accent = ui.Hex("#2563EB")
	p.danger = ui.Hex("#C0392B")
	p.glyph = ui.Hex("#4B5058")
	if opaque {
		p.window = ui.Hex("#F6F6F7")
		p.destination = ui.Hex("#FFFFFF")
		p.rowNormal = ui.Hex("#FBFBFC")
		p.rowHover = ui.Hex("#EFF1F5")
		p.rowChosen = ui.Hex("#E8F0FE")
		p.keycapBg = ui.Hex("#EDEEF2")
		p.segBg = ui.Hex("#E7E9EE")
		p.segActive = ui.Hex("#FFFFFF")
	} else {
		p.window = ui.RGBA(246, 246, 247, 0.78)
		p.destination = ui.RGBA(255, 255, 255, 0.60)
		p.rowNormal = ui.RGBA(255, 255, 255, 0.32)
		p.rowHover = ui.RGBA(255, 255, 255, 0.55)
		p.rowChosen = ui.RGBA(37, 99, 235, 0.13)
		p.keycapBg = ui.RGBA(255, 255, 255, 0.50)
		p.segBg = ui.RGBA(0, 0, 0, 0.055)
		p.segActive = ui.RGBA(255, 255, 255, 0.85)
	}
	if highContrast {
		p.border = ui.RGBA(0, 0, 0, 0.26)
		p.textSecond = ui.Hex("#3A3F47")
		p.textMuted = ui.Hex("#4E545C")
	} else {
		p.border = ui.RGBA(0, 0, 0, 0.085)
	}
	p.keycapText = ui.Hex("#4B5058")
	return p
}

// springPress 是按压/抬起使用的快弹簧：无回弹，160ms 内到位，
// 且能被新的目标打断——指针按住不放时不会出现"必须等动画结束"。
func springPress() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: motionPress, Bounce: 0}}
}

// springEntry 以 1% 缩放和淡入提供短促的到位反馈，不改变布局或阻塞输入。
func springEntry() ui.Animation {
	return ui.Animation{Spring: ui.SpringOptions{Duration: motionEntry, Bounce: 0}}
}

// fadeColors 让背景与边框颜色平滑过渡，位置与尺寸不参与，
// 因此不会因为重排而出现跟随滚动条的错位动画。
func fadeColors() ui.ElementTransition {
	return ui.ElementTransition{Duration: motionFeedback, Colors: true}
}
