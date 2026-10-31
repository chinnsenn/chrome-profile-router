// [INPUT]: 依赖 selectorApp 队列/启动状态、MyGo ui 包、view_style.go 调色板、settings.go 的外观设置、motion.go 内容生命周期，以及 layout.go 布局常量。
// [OUTPUT]: 渲染 header 与身份画廊，把指针与键盘意图转成选择、启动与退出；header 右侧提供设置入口（自绘滑杆图标 + 视图内快捷键），设置面板替换身份列表区，地址卡由 destination.go 渲染、面板控件在 settings_panel.go；snapshot 另外携带出场相位与最近交付地址（exiting/delivered），供地址卡在出场帧显示诚实文案。
// [POS]: 根包的视图层主入口；身份项渲染委托给 profile_view.go，路由与队列逻辑仍在 main.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// snapshot 是一帧渲染所需的全部状态，只在持锁时取一次，
// 之后渲染不再触碰 selectorApp 的字段。
type snapshot struct {
	profiles     []Profile
	chosen       int
	themeMode    ThemeMode
	launching    bool
	launchError  string
	loadErr      string
	revealChosen bool
	// reduceTransparency 是系统"降低透明度"偏好：与其它帧状态一起在持锁
	// 时复制，渲染期间不再回读 selectorApp。
	reduceTransparency bool
	// appearance 是设置文件里的外观值（主题、材质档位、两层覆盖不透明度），
	// 已夹取并补齐默认值；渲染帧只用它，不再回读文件。
	appearance appearanceSettings
	// motion 是设置文件里的动效分组（三个开关 + 减少动态效果档位），已补齐
	// 默认值；渲染帧用它决定要不要跑流彩、放大让位与倾斜。
	motion motionSettings
	// settingsOpen 表示本帧画的是设置面板而不是身份列表。
	settingsOpen bool
	// exiting 表示本帧处于自然出场（交付成功后的淡出）。它只影响地址卡
	// 的文案：此时队列已空，但地址卡必须继续显示真正交付的地址。
	exiting bool
	// delivered 是最近一次成功交付的地址，仅在与 exiting 同时成立时使用。
	delivered string
}

// reduceMotion 报告本帧是否按"减少动态效果"渲染：系统偏好与设置里的
// 「强制开启」取并集。没有「强制关闭」——那会覆盖系统的 ReduceMotion。
func (s snapshot) reduceMotion(system bool) bool {
	return system || s.motion.ReduceMotion == motionReduceOn
}

// view 绘制整个窗口。它只读 selectorApp 的状态，所有副作用都在本帧内
// 明确发生：键盘选择、悬停选择、主题切换、设置面板与重新加载。
func (a *selectorApp) view(c *ui.Context) {
	a.viewWith(c, a.openWith)
}

// viewWith 是 view 的实现，启动动作由调用方给出。
//
// 生产路径永远传 a.openWith；测试传记录用的回调，于是"设置面板打开时数字键
// 与回车会不会启动身份"可以在不拉起任何 Chrome 进程的前提下被断言（与
// profileListWith 同一手法：没有全局可变的 launcher）。
func (a *selectorApp) viewWith(c *ui.Context, launch func(Profile)) {
	if a.reloadRequested {
		a.reloadRequested = false
		a.reload()
	}

	// Esc 分层：设置面板打开时只关闭面板（回到身份列表），面板已关时才退出
	// 应用。除此之外 Esc 仍然永远有效——启动中、错误、空状态下都能用。
	if c.Shortcut(0, ui.KeyEscape) {
		if a.closeSettings() {
			c.Invalidate()
			return
		}
		a.requestQuit()
		return
	}
	// 面板窗口不激活应用，系统菜单栏收不到退出与设置命令，所以两个快捷键
	// 都在视图内直接接管（ui.Cmd 在 macOS 是 ⌘、其他平台是 Ctrl）。
	// 设置快捷键切换设置面板，退出快捷键立即退出。
	if c.Shortcut(ui.Cmd, ui.KeyQ) {
		a.flushSettings()
		a.requestQuit()
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.toggleSettings()
		c.Invalidate()
		return
	}
	// 键盘先于渲染处理：按键在送达的这一帧就改变选择，没有一帧延迟。
	if a.handleKeysWith(c, launch) {
		return
	}

	snap := a.snapshot()
	if snap.chosen < 0 || snap.chosen >= len(snap.profiles) {
		snap.chosen = 0
	}
	// 本帧生效的减少动态效果：系统偏好与「强制开启」取并集。
	reduce := snap.reduceMotion(c.Preferences().ReduceMotion)

	detailOpen := ui.Local(c.Root(), "profileDetailOpen", func() bool { return false })
	if c.Shortcut(0, ui.KeySpace) && !snap.launching && len(snap.profiles) > 0 && !snap.settingsOpen {
		*detailOpen = !*detailOpen
		c.Invalidate()
	}
	if snap.launching || len(snap.profiles) == 0 || snap.settingsOpen {
		*detailOpen = false
	}
	dark := snap.themeMode == ThemeDark || (snap.themeMode == ThemeAuto && c.Theme().Dark)
	// 原生磨砂由窗口提供；高对比与系统"降低透明度"都改用实色表面。
	// c.Vibrancy 仅反映窗口材质配置，不等同于无障碍偏好；降低透明度由
	// main.go 在应用启动后读取一次并随主题变化刷新，渲染帧不再访问
	// mygo.Theme（那会在离屏测试里 panic）。减少动态效果由框架处理，
	// 设置里的「强制开启」由 snapshot.reduceMotion 并进同一个判断。
	// 两层覆盖不透明度取当前外观那一侧；无障碍降级优先于设置，opaque
	// 分支不使用它们。
	backgroundTint, cardTint := snap.appearance.BackgroundTint.Dark, snap.appearance.CardTint.Dark
	if !dark {
		backgroundTint, cardTint = snap.appearance.BackgroundTint.Light, snap.appearance.CardTint.Light
	}
	pal := newPaletteTinted(dark, c.Vibrancy(), c.Preferences().HighContrast, snap.reduceTransparency, backgroundTint, cardTint)

	// 主题色半透明覆盖原生磨砂，手动主题也保持对比度；文字始终不透明。
	c.Root().Background(pal.window)

	// 原生磨砂与 Root 始终不变，进出场只作用于内容层。
	panel := ui.Column(c).Fill().Padding(panelPadding).Gap(menuGap)
	a.animateContent(c, panel, reduce)

	panel.Children(func() {
		a.header(c, pal, snap)
		a.destination(c, pal, snap)
		switch {
		case snap.settingsOpen:
			// 设置面板替换身份列表区；地址卡保留，用户仍看得到当前链接与
			// 剩余条数，它同时是「阅读面浓度」的实时预览对象。
			a.settingsPanel(c, pal, dark)
		case len(snap.profiles) == 0:
			emptyState(c, pal, snap.loadErr, func() {
				a.reloadRequested = true
				c.Invalidate()
			})
		default:
			a.profileListWith(c, pal, snap, detailOpen, launch)
		}
	})
}

// animateContent 在「减少动态效果」生效时把内容层钉在终态：入场不做缩放与
// 位移，出场也不动。动画通道仍由 motion.go 推进并完成，因此退出通路与原生
// 窗口的销毁时机完全不变——这里只覆盖本帧的变换与透明度，不改动生命周期。
func (a *selectorApp) animateContent(c *ui.Context, panel ui.Element, reduce bool) {
	a.animatePanel(c, panel)
	if reduce {
		panel.Transform(ui.Transform{}).Opacity(1)
	}
}

// snapshot 在持锁状态下复制本帧需要的状态。
func (a *selectorApp) snapshot() snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := snapshot{
		profiles:           a.profiles,
		chosen:             a.chosen,
		themeMode:          a.themeMode,
		launching:          a.launching,
		launchError:        a.launchError,
		loadErr:            a.loadErr,
		revealChosen:       a.revealChosen,
		reduceTransparency: a.reduceTransparency,
		appearance:         a.settings.Appearance.normalized(),
		motion:             a.settings.Motion.normalized(),
		settingsOpen:       a.settingsOpen,
		exiting:            a.motion.phase == exitNatural,
		delivered:          a.motion.delivered,
	}
	// revealChosen 是"下一帧把选中项滚进视野"的一次性标记，只有真正画列表
	// 的帧才消费它：设置面板打开时列表没画，标记必须留着，否则关闭面板后
	// 选中卡可能停在视口外。
	if !a.settingsOpen {
		a.revealChosen = false
	}
	return s
}

// handleKeys 处理本帧送达的按键，返回 true 表示已发起启动、本帧无需再渲染。
func (a *selectorApp) handleKeys(c *ui.Context) bool {
	return a.handleKeysWith(c, a.openWith)
}

// handleKeysWith 是 handleKeys 的实现，启动动作由调用方给出（测试注入记录
// 回调，生产路径永远传 a.openWith）。
func (a *selectorApp) handleKeysWith(c *ui.Context, launch func(Profile)) bool {
	// 只读状态，绝不清 revealChosen——那是渲染帧要消费的一次性标记。
	a.mu.Lock()
	profiles := a.profiles // 整体替换而非原地修改，复制切片头即可安全读取
	total := len(profiles)
	launching := a.launching
	chosen := a.chosen
	settingsOpen := a.settingsOpen
	a.mu.Unlock()

	// 设置面板打开时键盘只归面板：数字键、回车、⌘C 与方向键都不能启动身份
	// 或改写列表选择（避免误交付链接，也保证关闭面板后选中项、滚动位置与
	// 列表布局一律不变）。新链接仍照常入队，并在地址卡显示剩余条数。
	if settingsOpen {
		return false
	}

	// 启动中屏蔽重复触发；Esc、⌘Q 与 ⌘, 已在调用方处理，仍可随时退出。
	if total == 0 || launching {
		return false
	}
	if chosen < 0 || chosen >= total {
		chosen = 0
	}

	if c.Shortcut(0, ui.KeyEnter) {
		launch(profiles[chosen])
		return true
	}

	width, _ := c.Size()
	cols := profileColumns(total, width)
	grid := total > singleColumnLimit

	if !grid {
		// 1–4 个身份：单列列表保留左右循环漫游与上下相邻移动（既有预期）。
		if c.Shortcut(0, ui.KeyRight) {
			if n := (chosen + 1) % total; n != chosen {
				a.setChosen(n)
			}
			c.Invalidate()
		}
		if c.Shortcut(0, ui.KeyLeft) {
			if n := (chosen - 1 + total) % total; n != chosen {
				a.setChosen(n)
			}
			c.Invalidate()
		}
		if c.Shortcut(0, ui.KeyDown) {
			if chosen+1 < total {
				a.setChosen(chosen + 1)
				c.Invalidate()
			}
		}
		if c.Shortcut(0, ui.KeyUp) {
			if chosen > 0 {
				a.setChosen(chosen - 1)
				c.Invalidate()
			}
		}
	} else {
		// 5+ 个身份：网格模式按列数二维导航。
		// 左右在同行移动，边界停住；上下按列数跨行，下行短缺钳制到该行最后有效项，最后一行按下停住。
		row := chosen / cols
		col := chosen % cols
		rows := panelRows(total, cols)
		lastRow := rows - 1

		if c.Shortcut(0, ui.KeyRight) {
			if col < cols-1 && chosen+1 < total {
				a.setChosen(chosen + 1)
				c.Invalidate()
			}
		}
		if c.Shortcut(0, ui.KeyLeft) {
			if col > 0 {
				a.setChosen(chosen - 1)
				c.Invalidate()
			}
		}
		if c.Shortcut(0, ui.KeyDown) {
			if row < lastRow {
				target := chosen + cols
				if target >= total {
					target = total - 1 // 下行短缺：钳制到末行最后一个有效项
				}
				a.setChosen(target)
				c.Invalidate()
			}
			// row == lastRow 时按下停住，不触发 setChosen
		}
		if c.Shortcut(0, ui.KeyUp) {
			if row > 0 {
				a.setChosen(chosen - cols)
				c.Invalidate()
			}
			// row == 0 时按上停住
		}
	}

	digitKeys := []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for idx, key := range digitKeys {
		if idx < total && c.Shortcut(0, key) {
			a.setChosen(idx)
			launch(profiles[idx])
			return true
		}
	}
	return false
}

// header 是顶部一行：左侧关闭控件、右侧设置入口与主题开关，没有标题也没有
// 常驻说明。整行可拖动窗口，因为窗口隐藏了标题栏；高度固定，绝不因内容变化
// 而增高——设置图标与关闭控件同为 menuHeaderHeight 见方，因此不撑高这一行。
func (a *selectorApp) header(c *ui.Context, pal palette, snap snapshot) {
	ui.Row(c).
		Key("header").
		Height(menuHeaderHeight).
		AlignItems(ui.Center).
		Gap(4).
		DragWindow().
		Shrink(0).
		Children(func() {
			// 原生关闭：先把尚未落盘的设置冲刷掉，再走既有的短出场。
			closeControl(c, pal, func() {
				a.flushSettings()
				a.beginExit(exitClose)
			})

			ui.Spacer(c)

			settingsControl(c, pal, snap.settingsOpen, func() {
				a.toggleSettings()
				c.Invalidate()
			})

			// 分段开关整体压在 22pt 以内：外框 1pt 内边距加 12pt 行高的
			// 10pt 文字，保证控件不被 header 裁掉，也不把 header 撑高。
			ui.Row(c).
				Key("theme").
				Padding(1).
				Gap(1).
				Radius(6).
				Background(pal.segBg).
				Transition(fadeColors()).
				Shrink(0).
				Children(func() {
					segment(c, "seg:auto", "系统", snap.themeMode == ThemeAuto, pal, func() { a.setThemeMode(ThemeAuto, c) })
					segment(c, "seg:light", "浅色", snap.themeMode == ThemeLight, pal, func() { a.setThemeMode(ThemeLight, c) })
					segment(c, "seg:dark", "深色", snap.themeMode == ThemeDark, pal, func() { a.setThemeMode(ThemeDark, c) })
				})
		})
}

// settingsControl 是 header 里的设置入口：menuHeaderHeight 见方，与关闭控件
// 同一尺寸，因此不改变头部高度。它带无障碍名与 Tooltip，面板打开时保持
// 按下态底色。
//
// 图标是三条滑杆（每条一根横线加一个小方块），不是齿轮：13pt 的画布上 8 齿
// 齿轮会糊成一团，而"圆环 + 四齿"在真机上被误读成太阳/亮度。滑杆是 macOS
// 里"可调参数"的通用隐喻，1.2pt 线宽 + 2.6pt 方块在 13pt 内仍然分得清。
// 图形全部用 ui 图元自绘，不依赖字体里有没有 U+2699。
//
// 入口只有这一个加视图内的 ⌘,：不新增独立窗口、不加菜单栏常驻，也不改
// Panel 语义（面板窗口不激活应用，标准 ⌘, 菜单命令永远收不到）。
func settingsControl(c *ui.Context, pal palette, open bool, onToggle func()) {
	btn := ui.Row(c).
		Key("settingsGear").
		Size(menuHeaderHeight, menuHeaderHeight).
		Radius(5).
		Center().
		Background(ui.Transparent).
		Cursor(ui.CursorPointer).
		Role(ui.RoleButton).
		Label("设置").
		Tooltip("设置（" + shortcutLabel(",") + "）").
		Transition(fadeColors())

	switch {
	case open:
		btn.Background(pal.segActive)
	case btn.Hovered():
		btn.Background(pal.rowHover)
	}
	if btn.Pressed() {
		btn.Background(pal.segBg)
	}

	btn.Children(func() {
		// 13pt 画布：三条 1.2pt 滑轨，各带一枚 2.6pt 方块，方块错开位置才读
		// 得出是"三个可调参数"，而不是一段列表。
		ui.Box(c).Size(13, 13).Children(func() {
			sliderRow := func(top, knobLeft float32) {
				ui.Box(c).Absolute().Top(top).Left(0).Size(13, 1.2).
					Radius(0.6).Background(pal.glyph)
				ui.Box(c).Absolute().Top(top-0.7).Left(knobLeft).Size(2.6, 2.6).
					Radius(0.9).Background(pal.glyph)
			}
			sliderRow(3, 3.2)
			sliderRow(6.5, 8.4)
			sliderRow(10, 5.6)
		})
	})

	if btn.Clicked() {
		onToggle()
	}
}

// ============================================================================
// 控件
// ============================================================================

// closeControl 是左上角的小关闭控件：两条交叉笔画，带无障碍名称。
// 它在启动中、错误与空状态下都可用；首次短出场，再次点击立即退出。
// 边长等于 menuHeaderHeight，保证控件在 22pt 头部内完整显示而不被裁切。
func closeControl(c *ui.Context, pal palette, onClose func()) {
	btn := ui.Row(c).
		Key("close").
		Size(menuHeaderHeight, menuHeaderHeight).
		Radius(5).
		Center().
		Background(ui.Transparent).
		Cursor(ui.CursorPointer).
		Role(ui.RoleButton).
		Label("关闭").
		Tooltip("关闭").
		Transition(fadeColors())

	if btn.Hovered() {
		btn.Background(pal.rowHover)
	}
	if btn.Pressed() {
		btn.Background(pal.segBg)
	}

	btn.Children(func() {
		ui.Box(c).Size(9, 9).Children(func() {
			ui.Box(c).Absolute().Top(3.75).Left(0).Size(9, 1.5).Radius(0.75).
				Background(pal.glyph).Rotate(45)
			ui.Box(c).Absolute().Top(3.75).Left(0).Size(9, 1.5).Radius(0.75).
				Background(pal.glyph).Rotate(-45)
		})
	})

	if btn.Clicked() {
		onClose()
	}
}

// segment 是一枚轻量分段开关。它不可获得键盘焦点，因此点击主题后
// 方向键仍然用于选择身份。
func segment(c *ui.Context, key any, label string, active bool, pal palette, onPick func()) {
	bg := ui.Transparent
	text := pal.textMuted
	if active {
		bg = pal.segActive
		text = pal.textPrimary
	}

	seg := ui.Row(c).
		Key(key).
		Padding(2, 6).
		Radius(5).
		Background(bg).
		Cursor(ui.CursorPointer).
		Role(ui.RoleRadio).
		Label(label).
		Checked(active).
		Transition(fadeColors())

	if seg.Hovered() && !active {
		seg.Background(pal.rowHover)
		text = pal.textPrimary
	}

	seg.Children(func() {
		ui.Text(c, label).FontSize(10).FontWeight(500).FixedLineHeight(12).TextColor(text)
	})

	if seg.Clicked() {
		onPick()
	}
}

// emptyState 在无法识别任何身份时如实展示原因，并提供重新加载。
func emptyState(c *ui.Context, pal palette, loadErr string, onReload func()) {
	ui.Column(c).
		Key("empty").
		Grow(1).
		Padding(16).
		Gap(6).
		Radius(12).
		Background(pal.destination).
		Border(1, pal.border).
		Center().
		Children(func() {
			ui.Text(c, "未检测到有效的 Chrome 身份").
				FontSize(13).
				FontWeight(600).
				TextColor(pal.textPrimary)
			ui.Text(c, "请确认已安装并登录 Chrome 后重试。").
				FontSize(10.5).
				TextColor(pal.textSecond)
			if loadErr != "" {
				ui.Text(c, truncateRunes(loadErr, 58)).
					FontSize(10).
					TextColor(pal.textMuted).
					SingleLine()
			}

			reload := ui.Row(c).
				Key("reload").
				Padding(5, 12).
				Radius(7).
				Background(pal.accent).
				Cursor(ui.CursorPointer).
				Role(ui.RoleButton).
				Label("重新加载").
				Children(func() {
					ui.Text(c, "重新加载").FontSize(10.5).FontWeight(600).TextColor(ui.Hex("#FFFFFF"))
				})
			if reload.Clicked() {
				onReload()
			}
		})
}

// ============================================================================
// 状态写入
// ============================================================================

// setChosen 更新选中项，并要求下一帧把该行滚动到可见区域。
func (a *selectorApp) setChosen(i int) {
	a.mu.Lock()
	changed := a.chosen != i
	a.chosen = i
	if changed {
		a.revealChosen = true
	}
	a.mu.Unlock()
}

// trackPointer 把指针位置翻译成"是否该移动选择"，并报告选择是否被改变。
//
// 这是"静止指针不得撤销键盘选择"的核心保证：
//   - 只有指针在列表容器内的局部坐标真的变化（或指针刚进入列表）时，才
//     认为发生了一次真实的指针移动。内容滚动只改变偏移，不改变这个坐标，
//     所以滚动绝不会把用户滚走的行重新选中。
//   - 指针不动时只把"当前行"同步成指针下的那一行，从不改写选择；同步是
//     必要的，否则滚动后第一次真实移动会因行号未变而被吞掉。
//   - 只有真实的移动、且落到了与上一帧不同的一行，才改写选择。
//
// 于是键盘刚做出的选择会一直保持到用户真的移动鼠标或按下方向键。
func (a *selectorApp) trackPointer(row int, px, py float32, over bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !over {
		// 指针离开列表：下次进入算一次真实的进入。
		a.pointerSeen = false
		a.hoverValid = false
		return false
	}

	moved := !a.pointerSeen || !nearPoint(px, py, a.pointerX, a.pointerY)
	a.pointerSeen = true
	a.pointerX, a.pointerY = px, py

	// 指针同步到当前行，但不因此改变选择。
	changedRow := !a.hoverValid || a.hovered != row
	a.hoverValid = true
	a.hovered = row

	if row < 0 || !moved || !changedRow {
		return false
	}
	if a.launching || a.chosen == row {
		return false
	}
	a.chosen = row
	a.revealChosen = false // 指针自己找到了这一行，不必再滚动
	return true
}

// setReduceTransparency 记录系统"降低透明度"偏好，主线程调用。
func (a *selectorApp) setReduceTransparency(v bool) {
	a.mu.Lock()
	a.reduceTransparency = v
	a.mu.Unlock()
}

// setThemeMode 同步界面与原生磨砂外观，避免文字与材质使用相反的主题。
// 用户手动换主题时顺手把新值写回 settings.json——只在这一次写，不逐帧写。
func (a *selectorApp) setThemeMode(m ThemeMode, c *ui.Context) {
	a.mu.Lock()
	a.themeMode = m
	w := a.window
	a.mu.Unlock()
	a.persistTheme(m)
	if w != nil {
		source := themeSource(m)
		// 主题变更会重绘原生表面，放到下一轮主线程以避免渲染中途重入。
		w.Update(func() { mygo.Theme.SetSource(source) })
	}
	c.Invalidate()
}

// targetURLForDisplay 返回与 openWith 实际启动完全一致的地址。
//
// 无队列请求时必须返回 HomeURL，而不是空串：否则界面会展示一个与真实
// 启动无关的占位地址，用户看到的目标和 Chrome 真正打开的页面会对不上。
func (a *selectorApp) targetURLForDisplay() string {
	t, err := a.target()
	if err != nil {
		return HomeURL
	}
	return t
}

// initialRune 安全取出名字首字符，名字为空时不 panic。
func initialRune(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

// pointerEpsilon 是判定"指针移动"的容差。亚像素抖动来自布局重排时的
// 浮点重算，不应被当成一次真实的移动而改写键盘选择。
const pointerEpsilon float32 = 0.5

// nearPoint 报告两个坐标在容差内是否可视为同一点。
func nearPoint(x, y, ox, oy float32) bool {
	return absf(x-ox) < pointerEpsilon && absf(y-oy) < pointerEpsilon
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
