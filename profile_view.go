// [INPUT]: 依赖 selectorApp 的状态快照、ui 包与 view_style.go 的调色板；卡片高度、间距、头像与徽标尺寸来自 layout.go；悬停倾斜与内层视差来自 card_motion.go 的 cardTilt；悬停放大与邻居让位的几何来自 hover_layout.go 的 hoverLayout；选中卡的流彩描边与柔光参数来自 card_glow.go。
// [OUTPUT]: 渲染身份卡片画廊——1–4 个身份的 56pt 单列卡与 5+ 的 60pt 网格卡（身份色细条、32pt 真实头像、13.5pt 名称、11.5pt 邮箱、数字键徽标）。每张卡拆成三层：slot 是稳定布局盒，承载 Height / Shrink / FillWidth / ScrollIntoView，从不加变换；layer 是交互层，承载 Key / 命中 / Cursor / Role / Label / Description / Tooltip / Hovered / Pressed / Clicked 与悬停放大、邻居让位、按压弹簧，因命中测试会逆变换指针，放大出来的边缘同样可点；surface 是可见表面，承载 Padding / Gap / Radius / Border / Background / Shadow 与全部内容，并接受悬停 3D 倾斜与内层视差。选中且非不透明回退时，surface 的 1pt 描边换成预合成的锥形流彩、并叠加两级同色柔光。另提供无障碍描述、悬停 Tooltip 与空格详情浮层。
// [POS]: 根包的视图层分片，专管身份项呈现；状态写入与事件调度仍在 view.go，倾斜与视差的参数与变换链在 card_motion.go，空间再分配在 hover_layout.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// profileList 渲染可滚动的身份列表，启动动作固定为 openWith。
//
// 1–4 个身份用单列卡片，5 个起按窗口宽度铺网格卡片；两种模式渲染同一个
// 卡片结构，只是高度与容器不同。键盘改变选择的那一帧把目标项滚入视野。
func (a *selectorApp) profileList(c *ui.Context, pal palette, snap snapshot, detailOpen *bool) {
	a.profileListWith(c, pal, snap, detailOpen, a.openWith)
}

// profileListWith 是 profileList 的实现，启动动作由调用方给出。
//
// 生产路径永远传 a.openWith；测试传记录用的回调，这样点击语义（点到的究竟
// 是哪一张卡）可以在不启动任何系统进程的前提下被断言。这里没有全局可变的
// launcher，也不改变 openWith 本身的行为。
func (a *selectorApp) profileListWith(c *ui.Context, pal palette, snap snapshot, detailOpen *bool, launch func(Profile)) {
	total := len(snap.profiles)
	width, _ := c.Size()
	grid := total > singleColumnLimit
	itemGap := panelItemGap(total)
	itemHeight := panelItemHeight(total)
	// 本帧生效的减少动态效果：系统偏好与设置里的「强制开启」取并集。
	// 放大、让位、倾斜、视差与流彩角度都看这一个值。
	reduce := snap.reduceMotion(c.Preferences().ReduceMotion)

	cols := 1
	if grid {
		cols = profileColumns(total, width)
	}

	var chosenAnchor ui.Element

	list := ui.Scroll(c).Key("list").Grow(1)
	gallery := ui.Local(list, "hoverGallery", func() hoverGallery {
		return hoverGallery{}
	})

	// 悬停归属只看「根坐标指针」与「上一帧的几何」，绝不读交互层的
	// Hovered()：命中盒带着放大变换，动画帧本身就能让它在"在卡上/不在卡上"
	// 之间摆动，把悬停切走。判定顺序是上一帧的可见矩形（已让位、互不相交，
	// 放大出来的那一圈只属于放大它的那张卡）→ 上一帧的静止布局盒（首帧兜底）。
	px, py, _ := c.Root().PointerPosition()
	_, _, over := list.PointerPosition()
	hovered := -1
	if over {
		hovered = gallery.hoverAt(px, py)
	}

	// 两种容器共用同一段填充逻辑：顺序、选择、悬停与锚点完全一致。
	// slot/layer 句柄只在本帧有效，缓冲由 gallery 复用。
	slots, layers, pressed := gallery.elements(total)
	items := func() {
		for i, p := range snap.profiles {
			slot, layer, down := a.profileItem(c, pal, snap, p, i, i == snap.chosen, !snap.launching, hovered == i, launch, itemHeight, reduce)
			slots[i], layers[i], pressed[i] = slot, layer, down
			if i == snap.chosen {
				chosenAnchor = slot
			}
		}
	}

	list.Children(func() {
		if grid {
			ui.Grid(c).
				Key("grid").
				Columns(cols).
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

	// 悬停放大与邻居让位：整条带一起算，因为放大必须在同带邻居之间重新
	// 分配空间。入参是各卡的静止布局盒（slot 从不加变换，LayoutBounds 就是
	// 静止盒子），结果写进本帧缓冲。悬停索引为 -1 时全部回到静止值，离开
	// 因此是平滑复位而不是停在放大状态。
	//
	// 设置里关掉「悬停放大与让位」时，这里走与 ReduceMotion 完全相同的
	// 归零通路（hoverLayout 的 reduceMotion 入参），不另写一套几何：悬停仍
	// 提亮底色与投影，但槽位、命中盒与窗口尺寸都不动。变换通道照旧收敛到
	// 单位变换，因此不会留下空转的动画帧。
	//
	// 变换只施加给**交互层**：它与可见表面严格重合，MyGo 的命中测试会逆变换
	// 指针坐标，所以放大出来的边缘同样可点；slot 保持不动，滚动定位与详情
	// 浮层锚点因此不受动画影响。
	rects := gallery.rects(total)
	for i := range rects {
		rects[i] = slots[i].LayoutBounds()
	}
	shifts := gallery.shifts(total)
	hoverLayout(shifts, rects, cols, itemGap, hovered, reduce || !snap.motion.Hover)
	visible := gallery.visible(total)
	for i := range layers {
		press := float32(1)
		if pressed[i] {
			press = 0.98
		}
		press = layers[i].AnimateTo("press", press, springPress())
		shift := hoverShift{
			SX: layers[i].AnimateTo("hoverSX", shifts[i].SX, cardTiltSpring()),
			SY: layers[i].AnimateTo("hoverSY", shifts[i].SY, cardTiltSpring()),
			DX: layers[i].AnimateTo("hoverDX", shifts[i].DX, cardTiltSpring()),
			DY: layers[i].AnimateTo("hoverDY", shifts[i].DY, cardTiltSpring()),
		}
		visible[i] = visibleRect(rects[i], shift)
		if press == 1 && shift == (hoverShift{SX: 1, SY: 1}) {
			continue // 静止：不设变换，不为常态付任何绘制代价。
		}
		layers[i].Transform(hoverTransform(shift).Then(ui.Scaling(press, press)))
	}
	gallery.record(rects, visible)

	if detailOpen != nil && *detailOpen && chosenAnchor != (ui.Element{}) && snap.chosen >= 0 && snap.chosen < total {
		profileDetail(c, pal, snap.profiles[snap.chosen], snap.chosen, chosenAnchor, width)
	}

	// 指针坐标必须取根元素（= 窗口坐标）：列表容器的局部坐标会被祖先的
	// 变换逐帧改写——面板进出场的 scale/translate 都在改变换，静止的指针
	// 会因此每帧算出不同的局部坐标，被误判成"真实移动"并覆盖键盘选择。
	// 根元素不带任何变换，坐标稳定；"指针是否在列表视口内"仍由列表容器
	// 判定（它随面板缩放，这是正确的命中语义）。
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
// 卡片分三层，三层严格等大，只有中间一层接收悬停放大：
//   - slot 是稳定布局盒：Height、Shrink、ScrollIntoView 在它身上，从不加
//     任何变换，所以网格、滚动位置与详情浮层锚点都只由布局决定。
//   - layer 是交互层：Key、命中形状、Cursor、Role/Label/Description、
//     Tooltip、Hovered/Pressed/Clicked 都在它身上，并承载悬停放大与邻居让位
//     的缩放/位移。MyGo 的命中测试会逆变换指针坐标，所以放大出来的边缘同样
//     可点、可悬停——点到的永远是看见的那张卡。
//   - surface 是可见表面：背景、1pt 描边、投影、圆角与全部内容，整块接受
//     5.5° 倾斜。表面与内容一起转，内容永远不会越出自己的边框；若只倾斜
//     内容而让表面保持平直，文字与身份色细条就会在卡片边缘外露出来。
//
// 内边距只放在 surface：slot 与 layer 若也留 Padding(0,8)，表面会被再缩进
// 8pt，卡片表面跟着变窄。
//
// 卡片常态是几乎不可见的薄底加 1pt 透明描边：九个身份因此读作一张材质上的
// 画廊，而不是九块玻璃。悬停只提亮底色并加一层极浅投影，选中用主题色薄底、
// 主题色细边与实心数字徽标；两者都不做颜色过渡——键盘选择必须在按下的那一
// 帧就可见，不能靠动画补上反馈。长名称与邮箱通过 Tooltip 悬停或空格详情
// 浮层查看完整值。
//
// 返回稳定 slot、交互层、是否被指针悬停、是否处于按下状态。按下与悬停的
// 动画通道都由 profileList 统一施加（整条带要一起算空间分配）。
//
// reduce 是本帧生效的减少动态效果（系统偏好与设置里的强制开启取并集）：
// 它让倾斜归零、流彩角度不推进，但**不**退回实色——静态渐变与柔光照常
// 保留，选中标识不消失。snap.motion 决定三个装饰性动效各自的开关。
func (a *selectorApp) profileItem(c *ui.Context, pal palette, snap snapshot, prof Profile, idx int, isChosen, canLaunch, hovered bool, launch func(Profile), height float32, reduce bool) (slot, layer ui.Element, pressed bool) {
	sub := profileSubtitle(prof)
	fullTip := prof.Name
	if sub != "" && sub != prof.Name {
		fullTip = prof.Name + "\n" + sub
	}

	rowBg := pal.rowNormal
	rowBorder := pal.rowBorder
	subColor := pal.textSecond
	ringColor := pal.avatarRing
	if isChosen {
		rowBg = pal.rowChosen
		rowBorder = pal.accent.Alpha(chosenBaseBorderAlpha)
		subColor = pal.textOnChosen
		ringColor = pal.accent
	}

	// slot：纯布局盒，不画像素、不做变换。
	//
	// PassThrough 让 slot 退出命中登记：命中按"布局盒"登记，slot 的盒子不随
	// 放大移动，会盖住前一张卡伸过来的那一圈区域（放大 6% 的一半正好超过
	// 6pt 间距），使点击落到邻居的空 slot 上。退出登记后，命中只由已变换的
	// 交互层决定，可见边缘与可点边缘严格一致。
	slot = ui.Row(c).
		Key("slot:" + prof.Dir).
		Height(height).
		AlignItems(ui.Center).
		Shrink(0).
		PassThrough()

	// layer 与 surface 都必须建在 slot 内部，否则它们会成为网格/列的直接
	// 子元素，拿到的是整行宽高而不是卡片自己的格子。
	slot.Children(func() {
		// layer：交互层。命中与交互语义只在这里；样式写到 surface。
		layer = ui.Row(c).
			Key("row:" + prof.Dir).
			Fill().
			AlignItems(ui.Center).
			Radius(profileCardRadius).
			Cursor(ui.CursorPointer).
			Role(ui.RoleButton).
			Label(prof.Name).
			Description(sub).
			Tooltip(fullTip)

		// 悬停归属由 profileList 依据静止布局与指针坐标判定后传入；交互层
		// 自己的 Hovered() 会随放大变换摆动，不能作为悬停源。
		pressed = layer.Pressed()
		if hovered && !isChosen {
			// 悬停只提亮底色并加一层极浅投影，不出现第二张卡片的体积感。
			rowBg = pal.rowHover
			rowBorder = pal.border
		}
		var surfaceShadow ui.Color
		if hovered && !isChosen && !pal.opaque {
			surfaceShadow = pal.cardShadow
		}

		if snap.revealChosen && isChosen {
			slot.ScrollIntoView()
		}

		// 倾斜作用于 surface；动画通道挂在 layer 上，跨帧保持同一个元素。
		// 设置里关掉「3D 倾斜」时 cardTilt 走与 ReduceMotion 相同的归零
		// 通路，不倾斜也不视差。
		tilt, shiftX, shiftY := cardTilt(layer, snap.motion.Tilt, reduce)
		// 徽标是最靠边缘的元素，视差略强一档，与 Atropos children 的深度差
		// 一致；两个位移都取反，得到与倾斜方向相反的视差。
		badgeShiftX, badgeShiftY := shiftX*cardParallaxBadgeRatio, shiftY*cardParallaxBadgeRatio

		layer.Children(func() {
			// 选中且非不透明回退时，描边换成预合成的锥形流彩：stops 已经
			// 与基础选中边框合成过，所以这里不叠第二层，也不额外 DrawOver。
			//
			// 三条互不覆盖的条件：
			//   - glow：选中且非不透明（ReduceMotion 与关掉流彩都不影响它，
			//     选中标识不消失）；
			//   - spinning：glow 成立且设置里「选中卡流彩」开着——关掉时只
			//     保留静态基础选中边框，不再请求 Keyframes；
			//   - 角度：强制减少动态效果时不推进（不调 chosenSpin，因此没有
			//     空转的动画帧），停在 0° 与系统 ReduceMotion 的终态一致。
			glow := isChosen && !pal.opaque
			spinning := glow && snap.motion.Spin
			surface := ui.Row(c).
				Fill().
				Padding(0, 8).
				Gap(6).
				AlignItems(ui.Center).
				Radius(profileCardRadius).
				Background(rowBg).
				Border(chosenBorderWidth, rowBorder).
				Transform(tilt)
			if spinning {
				var angle float32
				if !reduce {
					angle = chosenSpin(layer)
				}
				surface.BorderConicGradient(chosenBorderGradient(layer, pal, angle))
			}
			if glow {
				// 选中比悬停再高一档：主题色薄底之外，Pow 的内两级单色辉光
				// 把它从相邻卡片里托起来。阴影不参与布局，因此不会推动网格；
				// 它替换选中原有的单层黑影，不叠加。
				surface.Shadow(0, 0, chosenGlowInnerRadius, 0, pal.accent.Alpha(chosenGlowAlpha(1.2)))
				surface.Shadow(0, 0, chosenGlowOuterRadius, 0, pal.accent.Alpha(chosenGlowAlpha(4)))
			} else if surfaceShadow.A > 0 {
				surface.Shadow(0, 1, 3, 0, surfaceShadow)
			}

			surface.Children(func() {
				// 身份色细条：把真实 Profile 颜色带进卡片，是除头像外唯一的
				// 个性标识。选中时它保持本色，不随主题色改变。
				ui.Box(c).
					Width(profileAccentBarWidth).
					Height(profileAccentBarHeight).
					Radius(profileAccentBarWidth / 2).
					Background(prof.Color.Alpha(0.85)).
					Shrink(0)

				// 头像与文字列同向位移；它们是表面的子元素，独立变换与表面
				// 的倾斜自然组合，不会互相覆盖。
				profileAvatar(c, prof, ringColor, profileAvatarSize, profileAvatarSize/2).
					Transform(ui.Translation(-shiftX, -shiftY))

				ui.Column(c).Grow(1).Gap(2).MinWidth(0).
					Transform(ui.Translation(-shiftX, -shiftY)).
					Children(func() {
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
					profileShortcut(c, pal, idx, isChosen).
						Transform(ui.Translation(-badgeShiftX, -badgeShiftY))
				}
			})
		})
	})

	if layer.Clicked() && canLaunch {
		launch(prof)
	}
	return slot, layer, pressed
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
// 返回头像元素，卡片用它承载内层视差位移。
func profileAvatar(c *ui.Context, prof Profile, ringColor ui.Color, size, radius float32) ui.Element {
	inner := size - 2*profileAvatarRing
	return ui.Row(c).
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
// 返回徽标元素，卡片用它承载略强一档的内层视差位移。
func profileShortcut(c *ui.Context, pal palette, idx int, isChosen bool) ui.Element {
	bg := pal.shortcutBg
	color := pal.shortcutText
	weight := 500
	if isChosen {
		bg = pal.shortcutOnBg
		color = pal.shortcutOnTxt
		weight = 600
	}
	return ui.Row(c).
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
