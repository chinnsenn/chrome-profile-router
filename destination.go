// [INPUT]: 依赖 selectorApp 的目标地址与队列状态、view_style.go 的调色板、shortcut_<goos>.go 的快捷键文案，以及 mygo 的剪贴板。
// [OUTPUT]: 渲染整幅宽度的地址卡（域名主行、路径/启动状态/失败原因次行、队列剩余徽标、复制按钮），并把按钮与快捷键的复制动作交给系统剪贴板。
// [POS]: 面板顶部的目标区；复制走 MyGo 的跨平台剪贴板，不再依赖平台外部命令。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// destination 是面板的视觉重点：一张整幅宽度的地址卡，域名以 24pt 粗体
// 独占主行，路径、启动状态或失败原因写在独立的次行，队列剩余条数做成贴在
// 主行右侧的小徽标。
//
// 高度固定为 menuDestinationHeight：任何状态都只替换次行文字，绝不新增一块
// 卡片或撑高面板，因此错误与启动中不会挤压身份列表，也不会留下空白。
// 地址文字区域可拖动窗口，悬停给出完整 URL；右侧按钮或快捷键复制原始地址。
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
		// 写剪贴板是主线程操作，且只在用户动作里发生；MyGo 负责三个平台的
		// 原生实现，失败可观测，所以"复制失败"仍然如实显示。
		if err := mygo.Clipboard.Write(transfer.TextData(full)); err != nil {
			copyState.message = "复制失败"
		} else {
			copyState.message = "已复制"
		}
		c.Invalidate()
	}
	// 设置面板打开时快捷键不复制：面板里没有可复制的输入，误触只会把链接
	// 悄悄放进剪贴板，掩盖"面板期间不交付链接"的契约。
	if c.Shortcut(ui.Cmd, ui.KeyC) && !snap.settingsOpen {
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
					Label("复制完整 URL").Tooltip("复制完整 URL（" + shortcutLabel("C") + "）")
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
