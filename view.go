// [INPUT]: 依赖 selectorApp 的队列/启动状态、ui 包，以及 view_style.go 的调色板与动效。
// [OUTPUT]: 渲染紧凑半透明的路由菜单，并把指针与键盘意图转成选择、启动与退出。
// [POS]: 根包的视图层；路由、启动与队列逻辑仍在 main.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const (
	// panelPadding 是窗口内容的内边距，顶部/目标/列表/页脚共用。
	panelPadding float32 = 16
	// rowHeight 与 rowGap 决定一行身份的尺寸：480×440 下约可见 5 行，
	// 更多身份时列表滚动，键盘选择仍会把目标行滚入视野。
	rowHeight float32 = 44
	rowGap    float32 = 6
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

	dark := snap.themeMode == ThemeDark || (snap.themeMode == ThemeAuto && c.Theme().Dark)
	// 原生磨砂由窗口提供；高对比模式使用实色，减少动态效果由框架处理。
	// c.Vibrancy 仅反映窗口材质配置，不等同于系统的降低透明度偏好。
	pal := newPalette(dark, c.Vibrancy(), c.Preferences().HighContrast)

	// 主题色半透明覆盖原生磨砂，手动主题也保持对比度；文字始终不透明。
	c.Root().Background(pal.window)

	// 面板作为一层材质"到位"：第一帧停在 0.99 与全透明（Jump 是瞬时赋值，
	// 不产生过渡），随后 150ms 内用快弹簧归位。只动变换与不透明度，
	// 不改布局；ReduceMotion 下框架直接取目标值，等于没有动画。
	panel := ui.Column(c).Fill().Padding(panelPadding).Gap(10)
	if a.introFrames == 0 {
		panel.Jump("introFade", 0)
		panel.Jump("introScale", 0.99)
	}
	a.introFrames++
	entry := springEntry()
	panel.Opacity(panel.AnimateTo("introFade", 1, entry))
	scale := panel.AnimateTo("introScale", 1, entry)
	panel.Scale(scale, scale)

	panel.Children(func() {
		a.header(c, pal, snap.themeMode)
		a.destination(c, pal)
		if snap.launching {
			statusBar(c, pal, "正在启动 Chrome…")
		}
		if snap.launchError != "" {
			errorBanner(c, pal, "启动失败", snap.launchError+"；请重试，或确认 Chrome 已安装。")
		}
		if len(snap.profiles) == 0 {
			emptyState(c, pal, snap.loadErr, func() {
				a.reloadRequested = true
				c.Invalidate()
			})
		} else {
			a.profileList(c, pal, snap)
		}
		a.footer(c, pal, len(snap.profiles))
	})
}

// snapshot 在持锁状态下复制本帧需要的状态。
func (a *selectorApp) snapshot() snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := snapshot{
		profiles:     a.profiles,
		chosen:       a.chosen,
		themeMode:    a.themeMode,
		launching:    a.launching,
		launchError:  a.launchError,
		loadErr:      a.loadErr,
		revealChosen: a.revealChosen,
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

	// 左右循环漫游，保留既有快捷键预期；上下改为相邻移动，符合竖向列表。
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

// header 是顶部一行：左上角关闭控件、克制的标题、右侧主题开关。
// 整行可拖动窗口，因为窗口隐藏了标题栏。
func (a *selectorApp) header(c *ui.Context, pal palette, mode ThemeMode) {
	ui.Row(c).Height(22).AlignItems(ui.Center).DragWindow().Children(func() {
		closeControl(c, pal, a.requestQuit)

		ui.Box(c).Width(8)
		ui.Text(c, "Chrome Profile Router").
			FontSize(10.5).
			FontWeight(600).
			LetterSpacing(0.4).
			TextColor(pal.textMuted)

		ui.Spacer(c)

		ui.Row(c).
			Padding(2).
			Gap(1).
			Radius(7).
			Background(pal.segBg).
			Transition(fadeColors()).
			Children(func() {
				segment(c, "seg:auto", "系统", mode == ThemeAuto, pal, func() { a.setThemeMode(ThemeAuto, c) })
				segment(c, "seg:light", "浅色", mode == ThemeLight, pal, func() { a.setThemeMode(ThemeLight, c) })
				segment(c, "seg:dark", "深色", mode == ThemeDark, pal, func() { a.setThemeMode(ThemeDark, c) })
			})
	})
}

// destination 显示即将打开的地址；队列里还有链接时说明剩余条数。
func (a *selectorApp) destination(c *ui.Context, pal palette) {
	host, pathStr, _ := parseURLDetails(a.targetURLForDisplay())
	// 是否收到外部请求由队列本身判断，绝不与 HomeURL 做字符串比较。
	hasPending := a.hasPendingRequests()

	label := "目标地址"
	if !hasPending {
		label = "未收到外部链接 · 默认打开 Google"
	}
	note := ""
	if n := a.pendingCount(); n > 1 {
		note = fmt.Sprintf("队列剩余 %d 条", n)
	}

	ui.Column(c).
		Key("destination").
		Padding(9, 12).
		Gap(2).
		Radius(12).
		Background(pal.destination).
		Border(1, pal.border).
		Shadow(0, 2, 10, 0, ui.RGBA(0, 0, 0, 0.06)).
		Transition(fadeColors()).
		Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Box(c).Size(5, 5).Radius(2.5).Background(pal.accent)
				ui.Text(c, label).
					FontSize(9.5).
					FontWeight(600).
					LetterSpacing(0.3).
					TextColor(pal.textMuted).
					SingleLine()
				ui.Spacer(c)
				if note != "" {
					ui.Text(c, note).FontSize(10).TextColor(pal.textMuted).SingleLine()
				}
			})

			ui.Row(c).AlignItems(ui.Baseline).Gap(5).Children(func() {
				ui.Text(c, host).
					FontSize(15.5).
					FontWeight(600).
					TextColor(pal.textPrimary).
					SingleLine()
				if pathStr != "" && pathStr != "/" {
					ui.Text(c, truncateRunes(pathStr, 34)).
						FontSize(11).
						TextColor(pal.textSecond).
						SingleLine()
				}
			})
		})
}

// profileList 渲染可滚动的身份列表。行高固定，超出窗口时滚动；
// 键盘改变选择的那一帧把目标行滚入视野。
func (a *selectorApp) profileList(c *ui.Context, pal palette, snap snapshot) {
	canLaunch := !snap.launching
	hovered := -1

	list := ui.Scroll(c).Key("list").Grow(1)
	list.Children(func() {
		ui.Column(c).Gap(rowGap).Padding(0, 2).Children(func() {
			for i, p := range snap.profiles {
				idx := i
				prof := p
				isChosen := idx == snap.chosen

				rowBg := pal.rowNormal
				rowBorder := ui.Transparent
				nameColor := pal.textPrimary
				subColor := pal.textMuted
				ringColor := pal.border
				if isChosen {
					rowBg = pal.rowChosen
					rowBorder = pal.accent.Alpha(0.45)
					nameColor = pal.accent
					subColor = pal.textSecond
					ringColor = pal.accent
				}

				row := ui.Row(c).
					Key("row:"+prof.Dir).
					Height(rowHeight).
					Padding(0, 10).
					Gap(9).
					AlignItems(ui.Center).
					Radius(10).
					Background(rowBg).
					Border(1, rowBorder).
					Cursor(ui.CursorPointer).
					Role(ui.RoleButton).
					Label(prof.Name).
					Transition(fadeColors())

				// Hovered 只报告"指针在哪一行"，绝不在这里改选择：选择由
				// trackPointer 根据指针是否真的移动决定。悬停底色是诚实的
				// 指针反馈，静止时保持原样。
				if row.Hovered() {
					hovered = idx
					if !isChosen {
						row.Background(pal.rowHover)
					}
				}

				// 按压反馈：短弹簧、可被打断，不改变任何布局。
				press := float32(1)
				if row.Pressed() {
					press = 0.985
				}
				scale := row.AnimateTo("press", press, springPress())
				row.Scale(scale, scale)

				if snap.revealChosen && isChosen {
					row.ScrollIntoView()
				}

				row.Children(func() {
					ui.Row(c).
						Size(28, 28).
						Radius(14).
						Center().
						Border(1.25, ringColor).
						Background(prof.Color.Alpha(0.16)).
						Transition(fadeColors()).
						Children(func() {
							if prof.Avatar != nil {
								ui.Image(c, prof.Avatar).Size(26, 26).Radius(13)
							} else {
								ui.Text(c, initialRune(prof.Name)).
									FontSize(11.5).
									FontWeight(600).
									TextColor(prof.Color)
							}
						})

					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, prof.Name).
							FontSize(12.5).
							FontWeight(600).
							TextColor(nameColor).
							SingleLine()
						sub := prof.Email
						if sub == "" || sub == prof.Name {
							sub = prof.Dir
						}
						ui.Text(c, truncateRunes(sub, 40)).
							FontSize(10.5).
							TextColor(subColor).
							SingleLine()
					})

					if idx < 9 {
						keyBg := pal.keycapBg
						keyText := pal.keycapText
						keyBorder := pal.border
						if isChosen {
							keyBg = pal.accent
							keyText = ui.Hex("#FFFFFF")
							keyBorder = pal.accent
						}
						ui.Row(c).
							Size(18, 18).
							Radius(5).
							Center().
							Background(keyBg).
							Border(1, keyBorder).
							Transition(fadeColors()).
							Children(func() {
								ui.Text(c, fmt.Sprintf("%d", idx+1)).
									FontSize(9.5).
									FontWeight(600).
									TextColor(keyText)
							})
					}
				})

				if row.Clicked() && canLaunch {
					a.openWith(prof)
					return
				}
			}
		})
	})

	// 指针坐标取列表容器内的局部坐标：内容滚动只改变偏移，不改变它，
	// 所以滚动绝不能被误判成"指针进入了另一行"。
	px, py, over := list.PointerPosition()
	if a.trackPointer(hovered, px, py, over) {
		// 本帧画的是 trackPointer 之前的选中项，补一帧让新选择显示出来。
		c.Invalidate()
	}
}

// footer 是最轻的键盘提示与身份计数。
func (a *selectorApp) footer(c *ui.Context, pal palette, total int) {
	ui.Row(c).Height(14).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "↑↓ 选择 · ↵ 打开 · 1–9 直达 · esc 退出").
			FontSize(10).
			TextColor(pal.textMuted).
			SingleLine()
		ui.Spacer(c)
		if total > 0 {
			ui.Text(c, fmt.Sprintf("%d 个身份", total)).
				FontSize(10).
				TextColor(pal.textMuted)
		}
	})
}

// ============================================================================
// 控件
// ============================================================================

// closeControl 是左上角的小关闭控件：两条交叉笔画，带无障碍名称。
// 它无条件调用 requestQuit，因此在启动中、错误与空状态下都可用。
func closeControl(c *ui.Context, pal palette, onClose func()) {
	btn := ui.Row(c).
		Key("close").
		Size(22, 22).
		Radius(6).
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
		ui.Box(c).Size(10, 10).Children(func() {
			ui.Box(c).Absolute().Top(4.25).Left(0).Size(10, 1.5).Radius(0.75).
				Background(pal.glyph).Rotate(45)
			ui.Box(c).Absolute().Top(4.25).Left(0).Size(10, 1.5).Radius(0.75).
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
		Padding(3, 7).
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
		ui.Text(c, label).FontSize(10).FontWeight(500).TextColor(text)
	})

	if seg.Clicked() {
		onPick()
	}
}

// statusBar 渲染进行中提示：一个转动的指示器加一行说明。
func statusBar(c *ui.Context, pal palette, label string) {
	ui.Row(c).
		Key("status").
		Padding(7, 10).
		Gap(8).
		Radius(9).
		Background(pal.rowNormal).
		Border(1, pal.border).
		AlignItems(ui.Center).
		Children(func() {
			ui.Spinner(c).Size(11, 11).TextColor(pal.accent)
			ui.Text(c, label).FontSize(10.5).FontWeight(500).TextColor(pal.textSecond)
		})
}

// errorBanner 渲染可读的失败原因，用户可原地重试。
func errorBanner(c *ui.Context, pal palette, title, msg string) {
	ui.Column(c).
		Key("error").
		Padding(8, 10).
		Gap(2).
		Radius(9).
		Background(pal.rowNormal).
		Border(1, pal.danger.Alpha(0.45)).
		Children(func() {
			ui.Text(c, title).FontSize(10.5).FontWeight(600).TextColor(pal.danger)
			ui.Text(c, msg).FontSize(10).TextColor(pal.textSecond).SingleLine()
		})
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

// setThemeMode 记录主题偏好并请求重绘。
func (a *selectorApp) setThemeMode(m ThemeMode, c *ui.Context) {
	a.mu.Lock()
	a.themeMode = m
	a.mu.Unlock()
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
