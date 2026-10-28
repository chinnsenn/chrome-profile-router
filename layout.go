// [INPUT]: 只依赖 mygo 的 Rectangle 类型与固定的面板几何常量。
// [OUTPUT]: 提供共享布局常量（含头部/目标卡/间距、身份卡片与头像尺寸、设置面板固定高度）、列数推导与窗口内容尺寸预算（列表模式与设置面板模式），供 view.go 与 main.go 共用。
// [POS]: 根包的布局层，纯计算；原生窗口的尺寸与位置写入在 main.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import "github.com/egoist/mygo"

const (
	// panelPadding 是窗口内容的内边距，header/目标卡/列表共用。
	panelPadding float32 = 14
	// rowHeight 与 rowGap 决定单列身份卡片的高度与间距：1–4 个身份时保持
	// 单列，但卡片本身与网格卡片同构，只是略矮一点。
	rowHeight float32 = 56
	rowGap    float32 = 4
	// profileCardHeight 与 profileGridGap 决定网格卡片的高度与间距：5 个
	// 身份起改用网格，窄到单列时仍是同一张卡片，不退回矮行。
	profileCardHeight float32 = 60
	profileGridGap    float32 = 6

	// menuHeaderHeight 是顶部关闭/主题行的高度：只放控件，不放标题。
	menuHeaderHeight float32 = 22
	// menuDestinationHeight 是目标地址卡的高度（border-box）：30pt 域名主行
	// 加 3pt 行距加 15pt 次行，加上下 10pt 内边距与 1pt 描边，再留 2pt 余量，
	// 保证次行在任何字体度量下都不被裁切。
	menuDestinationHeight float32 = 72
	// menuGap 是相邻纵向区块（header、目标卡、列表）之间的间距；三处区块
	// 共两处间距，view 与尺寸预算必须一致。
	menuGap float32 = 8
)

const (
	// singleColumnLimit 是仍使用单列的身份数量上限；超过它才启用网格。
	singleColumnLimit = 4
	// profileColumnsMax 是网格的列数上限，再多列会让卡片失去可读宽度。
	profileColumnsMax = 3
	// profileMinCardWidth 是网格卡片的可用宽度下限（不含间距）。200pt 让
	// 名称与邮箱并排两行仍有余量，同时保证 700pt 面板正好铺满三列。
	profileMinCardWidth float32 = 200
	// listInset 是列表容器左右各留的内边距，与 profile_view.go 的
	// Padding(listShadowPad, listInset) 对应：卡片投影在左右边缘也要有
	// 落点，否则选中第一列或最后一列时阴影会被滚动视口裁掉。
	listInset float32 = 5
	// listShadowPad 是列表内容上下留出的余量：卡片的选中/悬停投影与
	// 按压缩放都要落在滚动视口内，否则第一行与最后一行的阴影会被裁掉。
	listShadowPad float32 = 5

	// profileCardRadius 是身份卡片的圆角；比目标地址卡小一档，形成层级。
	profileCardRadius float32 = 11
	// profileAvatarSize 是卡片里真实头像的直径（含描边）。32pt 让头像
	// 成为卡片里最大的视觉元素，而不是一枚缩略图标。
	profileAvatarSize float32 = 32
	// profileAvatarRing 是头像描边宽度；描边始终占位，选中只换颜色。
	profileAvatarRing float32 = 1.5
	// profileAccentBar 是卡片左侧的身份色细条：宽度、高度与圆角。
	profileAccentBarWidth  float32 = 3
	profileAccentBarHeight float32 = 24
	// profileShortcutSize 是右侧数字键徽标的边长。
	profileShortcutSize float32 = 18
	// profileNameSize / profileEmailSize 是卡片两行文字的字号。
	profileNameSize  float32 = 13.5
	profileEmailSize float32 = 11.5

	// panelPreferredNarrow 是 1–4 个身份时的首选内容宽度。
	panelPreferredNarrow float32 = 460
	// panelPreferredWide 是网格模式的首选内容宽度：700 恰好放下 3 列卡片，
	// 且每列都超过 profileMinCardWidth。
	panelPreferredWide float32 = 700
	// panelChromeBase 是 header、目标卡、内边距与区块间距的合计预算：
	// 2*panelPadding + menuHeaderHeight + menuDestinationHeight + 2*menuGap
	// = 28 + 22 + 72 + 16 = 138。这里绝不再加"状态预留"——启动中提示与失败
	// 原因都写在目标卡的次行，替换路径而不是另占高度，否则窗口会永久多出
	// 一段没有内容的空白。
	panelChromeBase float32 = 2*panelPadding + menuHeaderHeight + menuDestinationHeight + 2*menuGap
	// panelChrome 是面板总高度里除卡片本身以外的全部固定开销：列表内容
	// 上下还各留 listShadowPad，让首末行的投影与按压缩放不被滚动视口裁掉。
	panelChrome float32 = panelChromeBase + 2*listShadowPad
	// panelEmptyHeight 是空状态下的窗口总高度：没有列表，所以不含列表余量，
	// 但要放得下说明、可选的失败原因与重新加载按钮（16pt 内边距 + 三行
	// 文字 + 按钮，共约 140pt）。
	panelEmptyHeight float32 = panelChromeBase + 150
	// panelSettingsHeight 是设置面板（替换身份列表区）的内容高度：三组控件
	// （外观 2 个滑块 + 1 个材质档位、动效 3 个开关 + 1 个减少动态效果档位、
	// 操作 1 个恢复默认）加组标题与分组间距的固定预算。它不随身份数量变化，
	// 因此打开/关闭设置只改窗口高度，不改列数、滚动位置或身份顺序。
	//
	// 它只是"列表区"的预算，不参与 panelChromeBase / panelChrome 的推导：
	// 那两条仍只描述列表模式，既有窗口尺寸预算逐像素不变。
	panelSettingsHeight float32 = 352
	// panelMinWidth / panelMinHeight 是极端窄工作区下的下限：窗口不能因为
	// 扣掉四周留白而变成零或负数尺寸，否则原生窗口会拒绝创建。
	panelMinWidth  float32 = 260
	panelMinHeight float32 = 160
	// panelWorkAreaFraction 限制窗口最多占工作区高度的比例。
	panelWorkAreaFraction float32 = 0.8
	// panelScreenMargin 是窗口与工作区边缘之间保留的余地。
	panelScreenMargin int = 32
)

// centeredOrigin 返回把 width×height 的窗口外框在工作区内居中的左上角。
func centeredOrigin(workArea mygo.Rectangle, width, height int) (int, int) {
	return workArea.X + (workArea.Width-width)/2, workArea.Y + (workArea.Height-height)/2
}

// clampOrigin 保持窗口左上角不变，只把它夹回工作区内，保证整个窗口都落在
// 可见范围里（换显示器、分辨率变化后用户拖出去的位置会被收回）。
func clampOrigin(workArea mygo.Rectangle, x, y, width, height int) (int, int) {
	x = max(workArea.X, min(x, workArea.X+max(0, workArea.Width-width)))
	y = max(workArea.Y, min(y, workArea.Y+max(0, workArea.Height-height)))
	return x, y
}

// profileColumns 推导 count 个身份在给定内容宽度下能放下的列数。
//
// 1–4 个身份固定单列；更多身份按可用宽度铺网格：扣掉左右内边距和列表
// 内缩后，保证每张卡片至少 profileMinCardWidth 宽、间距 profileGridGap，
// 结果钳制在 1..3。700pt 与 660pt 宽放下 3 列，480pt 宽放下 2 列，
// 低于 444pt 退化为单列。
func profileColumns(count int, width float32) int {
	if count <= singleColumnLimit {
		return 1
	}
	avail := width - 2*panelPadding - 2*listInset
	cols := 1
	for cols < profileColumnsMax {
		need := float32(cols+1)*profileMinCardWidth + float32(cols)*profileGridGap
		if need > avail {
			break
		}
		cols++
	}
	return cols
}

// panelRows 是 count 个身份在 cols 列下占用的行数。cols 由 profileColumns
// 给出，恒 ≥1。
func panelRows(count, cols int) int {
	if count <= 0 {
		return 0
	}
	return (count + cols - 1) / cols
}

// panelItemHeight 是单个身份占用的高度：单列用紧凑卡片，网格用标准卡片。
func panelItemHeight(count int) float32 {
	if count > singleColumnLimit {
		return profileCardHeight
	}
	return rowHeight
}

// panelItemGap 是相邻身份之间的间距，与 panelItemHeight 同步切换。
func panelItemGap(count int) float32 {
	if count > singleColumnLimit {
		return profileGridGap
	}
	return rowGap
}

// panelMaxHeight 是窗口内容高度的工作区上限：最多占工作区 80%，且四周留
// panelScreenMargin，并保底 panelMinHeight（极窄工作区下窗口仍是非零尺寸）。
func panelMaxHeight(areaH float32) float32 {
	return max(panelMinHeight, min(areaH*panelWorkAreaFraction, areaH-float32(panelScreenMargin)))
}

// settingsPanelSize 返回设置面板打开时的窗口内容尺寸。
//
// 宽度沿用 profilePanelSize 的列表预算——关闭设置后窗口必须回到原宽度，
// 横向跳动会让用户以为窗口被重置了。高度是固定预算：panelChromeBase 加上
// panelSettingsHeight（面板内容），再受工作区上限约束；放不下时面板内部
// 滚动，窗口本身绝不超出工作区。
func settingsPanelSize(count int, workArea mygo.Rectangle) (width, height int) {
	width, _ = profilePanelSize(count, workArea)
	want := panelChromeBase + panelSettingsHeight
	return width, int(min(want, panelMaxHeight(float32(workArea.Height))))
}

// profilePanelSize 返回面板内容区的尺寸预算。
//
// workArea 必须是 MyGo 报告的真实可用工作区（正宽高）；没有显示器就没有
// 用户窗口，调用方不需要也不应该伪造一个。宽度先按首选宽取值，再被工作区
// 宽度减两侧留白限制；列数由最终宽度决定，因为列数只随身份数量与真实宽度
// 变化。高度是固定界面预算加上所有行的占用，并同时受"工作区 80%"与"四周
// 留白"两道上限约束——放不下的行在列表内部滚动，窗口本身绝不超出工作区。
//
// 预算里没有隐藏状态的高度：9 个身份在 700 宽下是 148 + 3*60 + 2*6 = 340
// （138 的固定开销 + 上下各 5pt 列表余量 + 三行 60pt 卡片与 6pt 间距），
// 列表刚好铺满剩余空间，最后一行到窗口底只剩 panelPadding 的余量。
func profilePanelSize(count int, workArea mygo.Rectangle) (width, height int) {
	areaW := float32(workArea.Width)
	areaH := float32(workArea.Height)

	preferred := panelPreferredNarrow
	if count > singleColumnLimit {
		preferred = panelPreferredWide
	}
	// 先扣留白再取下限：极窄工作区（或系统报告 0 宽）下窗口仍然是一个
	// 可用的、非零的尺寸，而不是被负值交给原生层。
	width = int(max(panelMinWidth, min(preferred, areaW-float32(panelScreenMargin))))

	maxHeight := panelMaxHeight(areaH)

	want := panelEmptyHeight
	if count > 0 {
		rows := panelRows(count, profileColumns(count, float32(width)))
		want = panelChrome + float32(rows)*panelItemHeight(count) + float32(rows-1)*panelItemGap(count)
	}
	height = int(min(want, maxHeight))
	return width, height
}
