// [INPUT]: 依赖 selectorApp 的状态快照、ui 包与 view_style.go 的调色板；卡片高度、间距、头像与徽标尺寸来自 layout.go。
// [OUTPUT]: 渲染身份卡片画廊——1–4 个身份的 56pt 单列卡与 5+ 的 60pt 三列网格卡共用同一结构（身份色细条、32pt 真实头像、13.5pt 名称、11.5pt 邮箱、数字键徽标），并提供无障碍描述、悬停 Tooltip 与空格详情浮层。
// [POS]: 根包的视图层分片，专管身份项呈现；状态写入与事件调度仍在 view.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// profileList 渲染可滚动的身份列表。1–4 个身份用单列卡片，5 个起按窗口
// 宽度铺三列网格卡片；两种模式渲染同一个卡片结构，只是高度与容器不同。
// 键盘改变选择的那一帧把目标项滚入视野。
func (a *selectorApp) profileList(c *ui.Context, pal palette, snap snapshot, detailOpen *bool) {
	canLaunch := !snap.launching
	hovered := -1
	total := len(snap.profiles)
	width, _ := c.Size()
	grid := total > singleColumnLimit
	itemGap := panelItemGap(total)
	itemHeight := panelItemHeight(total)

	var chosenAnchor ui.Element

	// 两种容器共用同一段填充逻辑：顺序、选择、悬停与锚点完全一致。
	items := func() {
		for i, p := range snap.profiles {
			el, hov := a.profileItem(c, pal, snap, p, i, i == snap.chosen, canLaunch, itemHeight)
			if hov {
				hovered = i
			}
			if i == snap.chosen {
				chosenAnchor = el
			}
		}
	}

	list := ui.Scroll(c).Key("list").Grow(1)
	list.Children(func() {
		if grid {
			ui.Grid(c).
				Key("grid").
				Columns(profileColumns(total, width)).
				Gap(itemGap).
				Padding(listShadowPad, listInset).
				Children(items)
			return
		}
		ui.Column(c).
			Key("rows").
			Gap(itemGap).
			Padding(listShadowPad, listInset).
			Children(items)
	})

	// 键盘用户查看完整值：按空格打开/收起选中身份的详情浮层。
	// 浮层以选中项为锚点，宽度受窗口限制，名称与邮箱自动换行完整可读。
	// 浮层通过 Overlay 呈现，避免拦截 Escape 键，保证全时按 Esc 优先退出。
	if detailOpen != nil && *detailOpen && chosenAnchor != (ui.Element{}) && snap.chosen >= 0 && snap.chosen < total {
		profileDetail(c, pal, snap.profiles[snap.chosen], snap.chosen, chosenAnchor, width)
	}

	// 指针坐标取列表容器内的局部坐标：内容滚动只改变偏移，不改变它，
	// 所以滚动绝不能被误判成"指针进入了另一行"。
	px, py, over := list.PointerPosition()
	if a.trackPointer(hovered, px, py, over) {
		// 本帧画的是 trackPointer 之前的选中项，补一帧让新选择显示出来。
		c.Invalidate()
	}
}

// profileDetail 是空格打开的详情浮层：完整名称、邮箱与 Chrome 身份目录。
//
// 只展示磁盘上真实存在的字段，不补充任何推测信息。浮层是"提升"而不是
// "另一张卡片"：深一档的表面、1pt 细边与轻投影把它从卡片层抬起来，同时
// PassThrough 让指针与 Esc 仍然直达底层菜单。
func profileDetail(c *ui.Context, pal palette, prof Profile, idx int, anchor ui.Element, width float32) {
	sub := profileSubtitle(prof)
	ui.Overlay(c, func() {
		panel := ui.Column(c).
			Key("detailOverlay").
			AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).
			Width(max(200, min(float32(360), width-2*panelPadding))).
			Margin(6, 0, 0, 0).
			Padding(10, 12).
			Gap(8).
			Radius(12).
			Background(pal.popover).
			Border(1, pal.border).
			Shadow(0, 6, 18, 0, pal.popShadow).
			PassThrough()

		panel.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
				profileAvatar(c, prof, pal.accent, profileAvatarSize-4, (profileAvatarSize-4)/2)
				ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
					ui.Text(c, prof.Name).
						FontSize(13).
						FontWeight(600).
						TextColor(pal.textPrimary).
						Wrap()
					if sub != "" && sub != prof.Name {
						ui.Text(c, sub).
							FontSize(11).
							TextColor(pal.textSecond).
							Wrap()
					}
				})
				if idx < 9 {
					profileShortcut(c, pal, idx, true)
				}
			})

			// 身份目录是唯一能把"卡片属于哪个 Chrome Profile"说清楚的
			// 真实字段；用更小的字号低调呈现，不与名称争夺层级。
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Text(c, "Chrome 身份目录").
					FontSize(10).
					TextColor(pal.textMuted).
					Shrink(0)
				ui.Text(c, prof.Dir).
					FontSize(10.5).
					TextColor(pal.textSecond).
					SingleLine().
					Shrink(1).
					MinWidth(0)
			})
		})
	})
}

// profileItem 渲染一张身份卡片：左侧身份色细条，32pt 圆形真实头像（无图
// 时为首字母），中间名称 (13.5pt/600) 与邮箱/Dir (11.5pt) 两行，右侧 1–9
// 数字键徽标。height 由 layout.go 给出（单列 56pt，网格 60pt）。
//
// 卡片常态是几乎不可见的薄底加 1pt 透明描边：九个身份因此读作一张材质
// 上的画廊，而不是九块玻璃。悬停只提亮底色并加一层极浅投影，选中用主题色
// 薄底、主题色细边与实心数字徽标；两者都不改变任何几何尺寸，也都不做颜色
// 过渡——键盘选择必须在按下的那一帧就可见，不能靠动画补上反馈。
// 长名称与邮箱通过 Tooltip 悬停或空格详情浮层查看完整值。
// 返回元素及它是否被指针悬停。
func (a *selectorApp) profileItem(c *ui.Context, pal palette, snap snapshot, prof Profile, idx int, isChosen, canLaunch bool, height float32) (ui.Element, bool) {
	sub := profileSubtitle(prof)
	fullTip := prof.Name
	if sub != "" && sub != prof.Name {
		fullTip = prof.Name + "\n" + sub
	}

	rowBg := pal.rowNormal
	rowBorder := pal.rowBorder
	subColor := pal.textMuted
	ringColor := pal.avatarRing
	if isChosen {
		rowBg = pal.rowChosen
		rowBorder = pal.accent.Alpha(0.55)
		subColor = pal.textSecond
		ringColor = pal.accent
	}

	// 边框始终占 1pt（常态透明），选中时只换颜色，布局不重排。
	item := ui.Row(c).
		Key("row:"+prof.Dir).
		Height(height).
		Padding(0, 8).
		Gap(6).
		AlignItems(ui.Center).
		Radius(profileCardRadius).
		Background(rowBg).
		Border(1, rowBorder).
		Shrink(0).
		Cursor(ui.CursorPointer).
		Role(ui.RoleButton).
		Label(prof.Name).
		Description(sub).
		Tooltip(fullTip)

	isHovered := false
	if item.Hovered() {
		isHovered = true
		if !isChosen {
			// 悬停只提亮底色并加一层极浅投影，不出现第二张卡片的体积感。
			item.Background(pal.rowHover).Border(1, pal.border)
			if !pal.opaque {
				item.Shadow(0, 1, 3, 0, pal.cardShadow)
			}
		}
	}
	if isChosen && !pal.opaque {
		// 选中比悬停再高一档：主题色薄底之外，一层更宽的软投影把它从
		// 相邻卡片里托起来。阴影不参与布局，因此不会推动网格。
		item.Shadow(0, 2, 6, 0, pal.cardShadow)
	}

	// 按压反馈：短弹簧、可被打断，只动缩放，不改变任何布局。
	press := float32(1)
	if item.Pressed() {
		press = 0.98
	}
	scale := item.AnimateTo("press", press, springPress())
	item.Scale(scale, scale)

	if snap.revealChosen && isChosen {
		item.ScrollIntoView()
	}

	item.Children(func() {
		// 身份色细条：把真实 Profile 颜色带进卡片，是除头像外唯一的
		// 个性标识。选中时它保持本色，不随主题色改变。
		ui.Box(c).
			Width(profileAccentBarWidth).
			Height(profileAccentBarHeight).
			Radius(profileAccentBarWidth / 2).
			Background(prof.Color.Alpha(0.85)).
			Shrink(0)

		profileAvatar(c, prof, ringColor, profileAvatarSize, profileAvatarSize/2)

		ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
			ui.Text(c, prof.Name).
				FontSize(profileNameSize).
				FontWeight(600).
				TextColor(pal.textPrimary).
				SingleLine().
				Shrink(1).
				MinWidth(0)

			ui.Text(c, sub).
				FontSize(profileEmailSize).
				TextColor(subColor).
				SingleLine().
				Shrink(1).
				MinWidth(0)
		})

		if idx < 9 {
			profileShortcut(c, pal, idx, isChosen)
		}
	})

	if item.Clicked() && canLaunch {
		a.openWith(prof)
	}
	return item, isHovered
}

// profileSubtitle 提取身份的第二行副文本：优先邮箱，空或与名称重复时回退为目录名。
func profileSubtitle(p Profile) string {
	sub := p.Email
	if sub == "" || sub == p.Name {
		sub = p.Dir
	}
	return sub
}

// profileAvatar 绘制圆形头像：有图片画图，无图片画首字母。
// size 是含描边的外径，细描边让头像从卡片底上分离出来；选中时描边换成
// 主题色。图片比外径小 2*ring，保证描边不被图片盖住。
func profileAvatar(c *ui.Context, prof Profile, ringColor ui.Color, size, radius float32) {
	inner := size - 2*profileAvatarRing
	ui.Row(c).
		Size(size, size).
		Radius(radius).
		Center().
		Border(profileAvatarRing, ringColor).
		Background(prof.Color.Alpha(0.16)).
		Shrink(0).
		Children(func() {
			if prof.Avatar != nil {
				ui.Image(c, prof.Avatar).Size(inner, inner).Radius(inner / 2)
			} else {
				ui.Text(c, initialRune(prof.Name)).
					FontSize(size * 0.42).
					FontWeight(600).
					TextColor(prof.Color)
			}
		})
}

// profileShortcut 绘制右侧的 1–9 数字键徽标。常态是极淡的浅底加低调字色；
// 选中时换成主题色实心徽标，作为"回车即打开这一项"的唯一强调点。徽标不做
// 颜色过渡，与"键盘选择立即生效"保持一致。
func profileShortcut(c *ui.Context, pal palette, idx int, isChosen bool) {
	bg := pal.shortcutBg
	color := pal.shortcutText
	weight := 500
	if isChosen {
		bg = pal.shortcutOnBg
		color = pal.shortcutOnTxt
		weight = 600
	}
	ui.Row(c).
		Size(profileShortcutSize, profileShortcutSize).
		Radius(profileShortcutSize / 2).
		Center().
		Background(bg).
		Shrink(0).
		Children(func() {
			ui.Text(c, fmt.Sprintf("%d", idx+1)).
				FontSize(10).
				FontWeight(weight).
				TextColor(color).
				TextAlign(ui.Center)
		})
}
