// [INPUT]: 依赖 selectorApp 的设置状态与 settings.go 的夹取/写盘、view_style.go 的调色板、layout.go 的面板高度预算，以及 ui 的滑块/分段/开关控件。
// [OUTPUT]: 渲染设置面板并把它与设置文件绑定：外观浓度、磨砂材质、三个动效开关、减少动态效果与恢复默认，改动即时生效并在离散项/关闭面板时落盘。
// [POS]: 面板里替换身份列表区的那一块；从 view.go 拆出，模式位与键盘路由仍在 view.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// settingsUIState 是设置面板控件的绑定值。滑块与分段控件拿的是指针，必须
// 跨帧稳定，因此不能指向 snapshot 的副本；这些字段只在主线程（渲染帧）读写。
//
// loaded/dark 记录绑定值对应哪一套外观：面板打开、恢复默认，或自动主题下
// 外观翻转时，由 syncSettingsUI 把绑定值拉回设置文件里的当前值。
type settingsUIState struct {
	loaded     bool
	dark       bool
	background float64
	card       float64
	material   int
	reduce     int
}

// syncSettingsUI 把面板控件的绑定值同步成设置里的当前值。
func (a *selectorApp) syncSettingsUI(dark bool) {
	ui := &a.settingsUI
	if ui.loaded && ui.dark == dark {
		return
	}
	a.mu.Lock()
	ap := a.settings.Appearance.normalized()
	motion := a.settings.Motion.normalized()
	a.mu.Unlock()
	if dark {
		ui.background, ui.card = ap.BackgroundTint.Dark, ap.CardTint.Dark
	} else {
		ui.background, ui.card = ap.BackgroundTint.Light, ap.CardTint.Light
	}
	ui.material = materialIndex(ap.Material)
	ui.reduce = 0
	if motion.ReduceMotion == motionReduceOn {
		ui.reduce = 1
	}
	ui.dark, ui.loaded = dark, true
}

// invalidateSettingsUI 让下一帧重新同步绑定值（打开面板、恢复默认时用）。
func (a *selectorApp) invalidateSettingsUI() { a.settingsUI.loaded = false }

// settingsOpenState 报告设置面板是否打开。
func (a *selectorApp) settingsOpenState() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settingsOpen
}

// toggleSettings 在身份列表与设置面板之间切换，并重排窗口高度。
func (a *selectorApp) toggleSettings() {
	a.setSettingsOpen(!a.settingsOpenState())
}

// closeSettings 关闭设置面板，并报告它此前是否打开——Esc 分层靠这个返回值
// 区分"关面板"与"退出应用"。
func (a *selectorApp) closeSettings() bool {
	if !a.settingsOpenState() {
		return false
	}
	a.setSettingsOpen(false)
	return true
}

// setSettingsOpen 切换模式位、重排窗口高度，并在关闭时把设置冲刷到磁盘。
//
// 关闭时不重置选中项、不滚回顶部、不改列数：下一帧列表从同一状态重新画，
// 选中卡照旧可见。面板高度是固定预算（panelSettingsHeight），所以打开与
// 关闭都只改窗口高度。
func (a *selectorApp) setSettingsOpen(open bool) {
	a.mu.Lock()
	if a.settingsOpen == open {
		a.mu.Unlock()
		return
	}
	a.settingsOpen = open
	a.mu.Unlock()
	if open {
		a.invalidateSettingsUI()
	} else {
		a.flushSettings()
	}
	a.requestLayout(a.profileCount())
}

// settingsPanel 渲染设置面板，替换身份列表区。地址卡保留（当前链接与剩余
// 条数仍在，它同时是「阅读面浓度」的实时预览对象），面板高度固定为
// panelSettingsHeight，因此打开/关闭只改窗口高度，不改列数、滚动位置或身份
// 顺序。所有控件都是框架控件：Tab 进入、方向键调值、自带无障碍标签与读数。
func (a *selectorApp) settingsPanel(c *ui.Context, pal palette, dark bool) {
	a.syncSettingsUI(dark)
	// 系统「降低透明度」或「提高对比」时外观三项置灰只读：pal.opaque 已经把
	// 界面切成实色表面，此时再调浓度与材质没有意义。不提供"忽略系统设置"
	// 的开关——那是抢系统权威，也会让文字对比失控。
	opaque := pal.opaque

	ui.Scroll(c).Key("settings").Grow(1).Children(func() {
		ui.Column(c).Gap(6).Padding(listShadowPad, listInset).Children(func() {
			settingsGroupTitle(c, pal, "外观")
			if opaque {
				// 实色表面有两个来源：本窗口根本没有原生材质（Linux、Windows 10），
				// 或者系统要求降低透明度/提高对比。前者用 c.Vibrancy() 就能判断，
				// 不需要按 OS 打表；说给用户的原因必须是真的。
				notice := "系统已开启「降低透明度」或「提高对比」，界面使用不透明表面，外观三项暂不可调。"
				if !c.Vibrancy() {
					notice = "当前窗口不提供原生磨砂材质，界面使用不透明表面，外观三项暂不可调。"
				}
				ui.Text(c, notice).
					FontSize(10).
					TextColor(pal.textMuted).
					Wrap()
			}
			a.tintSlider(c, pal, "背景浓度", &a.settingsUI.background, backgroundTintMin, backgroundTintMax, true, opaque)
			a.tintSlider(c, pal, "阅读面浓度", &a.settingsUI.card, cardTintMin, cardTintMax, false, opaque)
			a.materialPicker(c, pal, opaque)

			settingsGroupTitle(c, pal, "动效")
			a.motionSwitch(c, pal, "选中卡流彩", &a.settings.Motion.Spin)
			a.motionSwitch(c, pal, "悬停放大与让位", &a.settings.Motion.Hover)
			a.motionSwitch(c, pal, "3D 倾斜", &a.settings.Motion.Tilt)
			a.reducePicker(c, pal)

			settingsGroupTitle(c, pal, "操作")
			a.resetControl(c, pal)
		})
	})
}

// settingsGroupTitle 是设置面板的分组标题：小字、低调，只做分段，不与控件
// 争层级。
func settingsGroupTitle(c *ui.Context, pal palette, title string) {
	ui.Text(c, title).FontSize(10).FontWeight(600).TextColor(pal.textMuted).Shrink(0)
}

// formatTint 把覆盖不透明度显示成两位小数，与写进设置文件的精度一致。
func formatTint(v float64) string { return fmt.Sprintf("%.2f", v) }

// tintSlider 是"标签 + 当前读数 + 滑块"的一组：读数给出两位小数的真值，
// 强度不只靠颜色深浅表达（数值本身可朗读）。
//
// 滑块写的是**当前外观那一侧**，并且先夹取量化再写设置；量化后的值回写给
// 绑定值，保证读数、滑块与文件里写的是同一个数字。拖动期间只改内存并重绘，
// 落盘留给面板关闭/退出前的 flushSettings。
func (a *selectorApp) tintSlider(c *ui.Context, pal palette, label string, value *float64, lo, hi float64, background, disabled bool) {
	ui.Column(c).Gap(1).Shrink(0).Children(func() {
		ui.Row(c).Height(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(pal.textSecond).Shrink(0)
			ui.Spacer(c)
			ui.Text(c, formatTint(quantizeTint(*value, lo, hi))).
				FontSize(11.5).
				TextColor(pal.textMuted).
				Shrink(0)
		})
		slider := ui.Slider(c, value, lo, hi).
			Step(tintStep).
			Label(label).
			Disabled(disabled)
		if slider.Changed() && !disabled {
			*value = quantizeTint(*value, lo, hi)
			if a.setTint(background, a.settingsUI.dark, *value) {
				c.Invalidate()
			}
		}
	})
}

// materialPicker 是磨砂材质的分段控件：四个用途材质，离散量不用滑块。标签
// 独占一行，四个档位名与 settings.json 里的取值完全一致，手改文件时不会对
// 不上号。切换走 Window.SetVibrancy + Window.Update，写 appearance.material。
func (a *selectorApp) materialPicker(c *ui.Context, pal palette, disabled bool) {
	ui.Column(c).Gap(2).Shrink(0).Children(func() {
		ui.Text(c, "磨砂材质").FontSize(11.5).TextColor(pal.textSecond).Shrink(0)
		seg := ui.Segmented(c, &a.settingsUI.material, materialOrder...).
			Label("磨砂材质").
			Disabled(disabled)
		if seg.Changed() && !disabled {
			a.pickMaterial(c)
		}
	})
}

// pickMaterial 把分段控件的档位落到设置与原生窗口：SetVibrancy 经
// Window.Update 投递，避免在渲染帧中途重绘原生表面。
func (a *selectorApp) pickMaterial(c *ui.Context) {
	idx := a.settingsUI.material
	if idx < 0 || idx >= len(materialOrder) {
		idx = 0
		a.settingsUI.material = 0
	}
	name := materialOrder[idx]
	if !a.setMaterialSetting(name) {
		return
	}
	if w := a.windowRef(); w != nil {
		v := vibrancyForMaterial(name)
		w.Update(func() { w.SetVibrancy(v) })
	}
	c.Invalidate()
}

// motionSwitch 是"标签 + 右侧开关"的一行。开关直接绑在设置字段上（地址跨帧
// 稳定），Changed 时立刻写盘：这是离散改动，不存在拖动期间的高频写入。
func (a *selectorApp) motionSwitch(c *ui.Context, pal palette, label string, on *bool) {
	ui.Row(c).Height(22).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(pal.textSecond).Shrink(1).MinWidth(0)
		ui.Spacer(c)
		sw := ui.Switch(c, on).Label(label)
		if sw.Changed() {
			// 开关直接改的是内存设置字段，Changed 之后立刻写盘。
			a.persistSettingsNow()
			c.Invalidate()
		}
	})
}

// reducePicker 是减少动态效果的两档：跟随系统 / 强制开启。**没有**"强制
// 关闭"——那会覆盖系统的 ReduceMotion。强制开启时所有几何动效归零，与系统
// 开启时走同一条归零通路。
func (a *selectorApp) reducePicker(c *ui.Context, pal palette) {
	ui.Row(c).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, "减少动态效果").FontSize(11.5).TextColor(pal.textSecond).Shrink(0)
		ui.Spacer(c)
		seg := ui.Segmented(c, &a.settingsUI.reduce, "跟随系统", "强制开启").Label("减少动态效果")
		if seg.Changed() {
			mode := motionReduceSystem
			if a.settingsUI.reduce == 1 {
				mode = motionReduceOn
			}
			a.setMotionReduce(mode)
			c.Invalidate()
		}
	})
}

// resetControl 是面板底部的"恢复默认"：浓度与材质耦合，用户很容易调成
// "读不清也调不回来"，所以这个入口必须存在。
func (a *selectorApp) resetControl(c *ui.Context, pal palette) {
	btn := ui.Row(c).
		Key("settingsReset").
		Padding(4, 10).
		Radius(7).
		Background(pal.rowNormal).
		Border(1, pal.border).
		Cursor(ui.CursorPointer).
		Role(ui.RoleButton).
		Label("恢复默认").
		Tooltip("恢复默认外观与动效，并立即生效").
		Shrink(0)
	if btn.Hovered() {
		btn.Background(pal.rowHover)
	}
	btn.Children(func() {
		ui.Text(c, "恢复默认").FontSize(11).FontWeight(500).TextColor(pal.textPrimary)
	})
	if btn.Clicked() {
		a.resetToDefaults(c)
	}
}

// resetToDefaults 把外观与动效恢复默认并立刻生效：设置写盘、原生材质换回
// 默认档位、滑块绑定值下一帧重新同步。它是"调不回来"的兜底，因此不延迟
// 落盘。
func (a *selectorApp) resetToDefaults(c *ui.Context) {
	a.resetSettings()
	a.invalidateSettingsUI()
	if w := a.windowRef(); w != nil {
		v := vibrancyForMaterial(defaultAppearance().Material)
		w.Update(func() { w.SetVibrancy(v) })
	}
	c.Invalidate()
}
