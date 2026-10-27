// [INPUT]: 依赖 selectorApp 队列/启动状态、MyGo ui 包、view_style.go 调色板、motion.go 内容生命周期，以及 layout.go 布局常量。
// [OUTPUT]: 渲染整幅宽度的地址卡与身份画廊，支持按钮/⌘C 复制完整 URL，并把指针与键盘意图转成选择、启动与退出。
// [POS]: 根包的视图层主入口；身份项渲染委托给 profile_view.go，路由与队列逻辑仍在 main.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"

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
}

// view 绘制整个窗口。它只读 selectorApp 的状态，所有副作用都在本帧内
// 明确发生：键盘选择、悬停选择、主题切换与重新加载。
func (a *selectorApp) view(c *ui.Context) {
	if a.reloadRequested {
		a.reloadRequested = false
		a.reload()
	}

	// Esc 永远有效：无论是否正在启动、是否有错误或空状态，都直接退出。
	if c.Shortcut(0, ui.KeyEscape) {
		a.requestQuit()
		return
	}
	// 面板窗口不激活应用，系统菜单栏收不到 ⌘Q，所以在这里直接接管，
	// 保持"⌘Q 立即退出"这一既有契约。
	if c.Shortcut(ui.Cmd, ui.KeyQ) {
		a.requestQuit()
		return
	}
	// 键盘先于渲染处理：按键在送达的这一帧就改变选择，没有一帧延迟。
	if a.handleKeys(c) {
		return
	}

	snap := a.snapshot()
	if snap.chosen < 0 || snap.chosen >= len(snap.profiles) {
		snap.chosen = 0
	}

	detailOpen := ui.Local(c.Root(), "profileDetailOpen", func() bool { return false })
	if c.Shortcut(0, ui.KeySpace) && !snap.launching && len(snap.profiles) > 0 {
		*detailOpen = !*detailOpen
		c.Invalidate()
	}
	if snap.launching || len(snap.profiles) == 0 {
		*detailOpen = false
	}
	dark := snap.themeMode == ThemeDark || (snap.themeMode == ThemeAuto && c.Theme().Dark)
	// 原生磨砂由窗口提供；高对比与系统"降低透明度"都改用实色表面。
	// c.Vibrancy 仅反映窗口材质配置，不等同于无障碍偏好；降低透明度由
	// main.go 在应用启动后读取一次并随主题变化刷新，渲染帧不再访问
	// mygo.Theme（那会在离屏测试里 panic）。减少动态效果由框架处理。
	pal := newPalette(dark, c.Vibrancy(), c.Preferences().HighContrast, snap.reduceTransparency)

	// 主题色半透明覆盖原生磨砂，手动主题也保持对比度；文字始终不透明。
	c.Root().Background(pal.window)

	// 原生磨砂与 Root 始终不变，进出场只作用于内容层。
	panel := ui.Column(c).Fill().Padding(panelPadding).Gap(menuGap)
	a.animatePanel(c, panel)

	panel.Children(func() {
		a.header(c, pal, snap.themeMode)
		a.destination(c, pal, snap)
		if len(snap.profiles) == 0 {
			emptyState(c, pal, snap.loadErr, func() {
				a.reloadRequested = true
				c.Invalidate()
			})
		} else {
			a.profileList(c, pal, snap, detailOpen)
		}
	})
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
	}
	a.revealChosen = false
	return s
}

// handleKeys 处理本帧送达的按键，返回 true 表示已发起启动、本帧无需再渲染。
func (a *selectorApp) handleKeys(c *ui.Context) bool {
	// 只读状态，绝不清 revealChosen——那是渲染帧要消费的一次性标记。
	a.mu.Lock()
	profiles := a.profiles // 整体替换而非原地修改，复制切片头即可安全读取
	total := len(profiles)
	launching := a.launching
	chosen := a.chosen
	a.mu.Unlock()

	// 启动中屏蔽重复触发；Esc 与 ⌘Q 已在调用方处理，仍可随时退出。
	if total == 0 || launching {
		return false
	}
	if chosen < 0 || chosen >= total {
		chosen = 0
	}

	if c.Shortcut(0, ui.KeyEnter) {
		a.openWith(profiles[chosen])
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
			a.openWith(profiles[idx])
			return true
		}
	}
	return false
}

// header 是顶部一行：左侧关闭控件、右侧主题开关，没有标题也没有常驻说明。
// 整行可拖动窗口，因为窗口隐藏了标题栏；高度固定，绝不因内容变化而增高。
func (a *selectorApp) header(c *ui.Context, pal palette, mode ThemeMode) {
	ui.Row(c).
		Key("header").
		Height(menuHeaderHeight).
		AlignItems(ui.Center).
		DragWindow().
		Shrink(0).
		Children(func() {
			closeControl(c, pal, func() { a.beginExit(exitClose) })

			ui.Spacer(c)

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
					segment(c, "seg:auto", "系统", mode == ThemeAuto, pal, func() { a.setThemeMode(ThemeAuto, c) })
					segment(c, "seg:light", "浅色", mode == ThemeLight, pal, func() { a.setThemeMode(ThemeLight, c) })
					segment(c, "seg:dark", "深色", mode == ThemeDark, pal, func() { a.setThemeMode(ThemeDark, c) })
				})
		})
}

// destination 是面板的视觉重点：一张整幅宽度的地址卡，域名以 24pt 粗体
// 独占主行，路径、启动状态或失败原因写在独立的次行，队列剩余条数做成贴在
// 主行右侧的小徽标。
//
// 高度固定为 menuDestinationHeight：任何状态都只替换次行文字，绝不新增一块
// 卡片或撑高面板，因此错误与启动中不会挤压身份列表，也不会留下空白。
// 地址文字区域可拖动窗口，悬停给出完整 URL；右侧按钮与 ⌘C 复制原始地址。
func (a *selectorApp) destination(c *ui.Context, pal palette, snap snapshot) {
	full := a.targetURLForDisplay()
	host, pathStr, _ := parseURLDetails(full)
	// 是否收到外部请求由队列本身判断，绝不与 HomeURL 做字符串比较。
	hasPending := a.hasPendingRequests()
	copyState := ui.Local(c.Root(), "destinationCopy", func() struct{ url, message string } {
		return struct{ url, message string }{}
	})
	if copyState.url != full {
		copyState.url, copyState.message = full, ""
	}
	copyURL := func() {
		cmd := exec.Command("/usr/bin/pbcopy")
		cmd.Stdin = strings.NewReader(full)
		if err := cmd.Run(); err != nil {
			copyState.message = "复制失败"
		} else {
			copyState.message = "已复制"
		}
		c.Invalidate()
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) {
		copyURL()
	}

	// 本地文件没有域名：主行改用文件名，次行给出完整路径，避免 24pt 粗体
	// 铺满一整条地址。网页地址仍以域名为主行。
	mainLine, secondLine := host, pathStr
	if rest, ok := strings.CutPrefix(full, "file://"); ok {
		if decoded, err := url.PathUnescape(rest); err == nil {
			rest = decoded
		}
		if i := strings.LastIndex(rest, "/"); i >= 0 && i+1 < len(rest) {
			mainLine = rest[i+1:]
		}
		secondLine = rest
	}

	// 次行优先级：失败原因 > 启动中 > 真实路径 > 未收到链接的说明。
	// 失败原因用 danger 色，并在自己的 Tooltip 里给完整原文。
	note, noteColor, noteTip := "", pal.textSecond, ""
	switch {
	case snap.launchError != "":
		note = "启动失败：" + snap.launchError
		noteColor = pal.danger
		noteTip = snap.launchError + "；请重试，或确认 Chrome 已安装。"
	case snap.launching:
		note = "正在启动 Chrome…"
	case secondLine != "" && secondLine != "/":
		note = secondLine
	case hasPending:
		// 没有路径分量的地址（例如 https://example.com）：次行给出根路径，
		// 保持与其它状态一致的视觉节奏。
		note = "/"
	default:
		note = "未收到外部链接 · 默认打开主页"
		noteColor = pal.textMuted
	}

	queueNote := ""
	if n := a.pendingCount(); n > 1 {
		queueNote = fmt.Sprintf("剩余 %d 条", n)
	}

	// 目标区是整幅宽度的地址卡：唯一带圆角、描边与轻投影的表面，因此它
	// 而不是任何一张身份卡成为视觉第一优先。卡片高度固定，任何状态都只
	// 替换次行文字，不改变几何。
	box := ui.Column(c).
		Key("destination").
		Height(menuDestinationHeight).
		FillWidth().
		Shrink(0).
		Padding(10, 12).
		Gap(3).
		Radius(12).
		Background(pal.destination).
		Border(1, pal.border).
		Tooltip(full)

	if !pal.opaque {
		box.Shadow(0, 2, 9, 0, pal.cardShadow)
	}

	box.Children(func() {
		// 主行：域名（24pt / 700，30pt 行高）。队列剩余条数只占主行右侧的
		// 固定宽度，域名自身 Shrink + 省略，绝不与它挤成两段。
		ui.Row(c).
			Height(30).
			AlignItems(ui.Center).
			Gap(8).
			Shrink(0).
			Children(func() {
				ui.Text(c, mainLine).
					FontSize(24).
					FontWeight(700).
					FixedLineHeight(30).
					LetterSpacing(-0.3).
					TextColor(pal.textPrimary).
					SingleLine().
					Shrink(1).
					MinWidth(0).
					DragWindow()
				ui.Spacer(c)
				if queueNote != "" {
					ui.Row(c).
						Padding(2, 7).
						Radius(7).
						Background(pal.shortcutBg).
						Shrink(0).
						Children(func() {
							ui.Text(c, queueNote).
								FontSize(10).
								FontWeight(500).
								TextColor(pal.textMuted).
								SingleLine()
						})
				}
				label := "复制"
				if copyState.message != "" {
					label = copyState.message
				}
				button := ui.Row(c).Key("copyURL").Padding(4, 7).Radius(5).
					Shrink(0).Cursor(ui.CursorPointer).Role(ui.RoleButton).
					Label("复制完整 URL").Tooltip("复制完整 URL（⌘C）")
				if button.Hovered() {
					button.Background(pal.rowHover)
				}
				button.Children(func() {
					ui.Text(c, label).FontSize(11).TextColor(pal.textSecond).SingleLine()
				})
				if button.Clicked() {
					copyURL()
				}
			})

		// 次行：路径 / 状态 / 错误，永远单行省略，高度固定。
		ui.Row(c).
			Height(15).
			AlignItems(ui.Center).
			Gap(5).
			Shrink(0).
			Children(func() {
				if snap.launching && snap.launchError == "" {
					ui.Spinner(c).Size(10, 10).TextColor(pal.accent).Shrink(0)
				}
				line := ui.Text(c, note).
					FontSize(12.5).
					TextColor(noteColor).
					FixedLineHeight(15).
					SingleLine().
					Shrink(1).
					MinWidth(0)
				if noteTip != "" {
					line.Tooltip(noteTip)
				}
			})
	})
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
func (a *selectorApp) setThemeMode(m ThemeMode, c *ui.Context) {
	a.mu.Lock()
	a.themeMode = m
	w := a.window
	a.mu.Unlock()
	if w != nil {
		source := mygo.ThemeSystem
		switch m {
		case ThemeLight:
			source = mygo.ThemeLight
		case ThemeDark:
			source = mygo.ThemeDark
		}
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
