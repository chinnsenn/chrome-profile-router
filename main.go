package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

type Profile struct {
	Dir    string
	Name   string
	Email  string
	Avatar *ui.Bitmap
	Color  ui.Color
}

type LocalState struct {
	Profile struct {
		InfoCache map[string]struct {
			Name                string `json:"name"`
			GaiaName            string `json:"gaia_name"`
			UserName            string `json:"user_name"`
			GaiaPictureFileName string `json:"gaia_picture_file_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

type ThemeMode int

const (
	ThemeAuto ThemeMode = iota
	ThemeLight
	ThemeDark
)

type selectorApp struct {
	targetURL string
	profiles  []Profile
	chosen    int
	window    *mygo.Window
	themeMode ThemeMode
}

var modernGradients = []ui.Color{
	ui.Hex("#3B82F6"), // Blue
	ui.Hex("#EC4899"), // Pink
	ui.Hex("#10B981"), // Emerald
	ui.Hex("#8B5CF6"), // Purple
	ui.Hex("#F59E0B"), // Amber
	ui.Hex("#06B6D4"), // Cyan
	ui.Hex("#6366F1"), // Indigo
	ui.Hex("#EF4444"), // Red
	ui.Hex("#14B8A6"), // Teal
}

func loadChromeProfiles() []Profile {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultProfiles()
	}

	statePath := filepath.Join(home, "Library/Application Support/Google/Chrome/Local State")
	chromeBase := filepath.Join(home, "Library/Application Support/Google/Chrome")

	data, err := os.ReadFile(statePath)
	if err != nil {
		return defaultProfiles()
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return defaultProfiles()
	}

	var list []Profile
	colorIdx := 0
	for dir, info := range state.Profile.InfoCache {
		dirPath := filepath.Join(chromeBase, dir)
		if fi, err := os.Stat(dirPath); err != nil || !fi.IsDir() {
			continue
		}

		name := strings.TrimSpace(info.Name)
		if name == "" {
			name = strings.TrimSpace(info.GaiaName)
		}
		if name == "" {
			name = strings.TrimSpace(info.UserName)
		}
		if name == "" {
			name = dir
		}

		email := strings.TrimSpace(info.UserName)

		var avatarBitmap *ui.Bitmap
		picFileName := info.GaiaPictureFileName
		if picFileName == "" {
			picFileName = "Google Profile Picture.png"
		}
		avatarPath := filepath.Join(dirPath, picFileName)
		if imgData, err := os.ReadFile(avatarPath); err == nil && len(imgData) > 0 {
			if bm, err := ui.DecodeBitmap(imgData); err == nil {
				avatarBitmap = bm
			}
		}

		c := modernGradients[colorIdx%len(modernGradients)]
		colorIdx++

		list = append(list, Profile{
			Dir:    dir,
			Name:   name,
			Email:  email,
			Avatar: avatarBitmap,
			Color:  c,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Dir == "Default" {
			return true
		}
		if list[j].Dir == "Default" {
			return false
		}
		return strings.ToLower(list[i].Dir) < strings.ToLower(list[j].Dir)
	})

	if len(list) == 0 {
		return defaultProfiles()
	}
	return list
}

func defaultProfiles() []Profile {
	return []Profile{
		{Dir: "Default", Name: "默认配置", Email: "", Color: modernGradients[0]},
		{Dir: "Profile 1", Name: "配置 1", Email: "", Color: modernGradients[1]},
	}
}

func (a *selectorApp) openWith(p Profile) {
	target := a.targetURL
	if target == "" {
		target = "https://www.google.com"
	}
	cmd := exec.Command("/usr/bin/open", "-n", "-a", "Google Chrome", "--args", "--profile-directory="+p.Dir, target)
	_ = cmd.Run()
	mygo.App.Quit()
}

func parseURLDetails(raw string) (host string, pathStr string, fullStr string) {
	if raw == "" {
		return "github.com", "/features/actions", "https://github.com/features/actions"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, "", raw
	}
	p := u.Path
	if u.RawQuery != "" {
		p += "?" + u.RawQuery
	}
	return u.Host, p, raw
}

func clamp(val, min, max float32) float32 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

func (a *selectorApp) view(c *ui.Context) {
	isDark := true
	switch a.themeMode {
	case ThemeDark:
		isDark = true
	case ThemeLight:
		isDark = false
	case ThemeAuto:
		isDark = c.Theme().Dark
	}

	var (
		bgWindow      ui.Color
		heroCardBg    ui.Color
		cardNormalBg  ui.Color
		cardChosenBg  ui.Color
		borderNormal  ui.Color
		borderChosen  ui.Color
		accentColor   ui.Color
		textPrimary   ui.Color
		textSecondary ui.Color
		textMuted     ui.Color
		keycapNormal  ui.Color
		keycapChosen  ui.Color
		segBg         ui.Color
		segActiveBg   ui.Color
	)

	if isDark {
		bgWindow = ui.Hex("#0D0F14")
		heroCardBg = ui.Hex("#151821")
		cardNormalBg = ui.Hex("#151822")
		cardChosenBg = ui.RGBA(28, 55, 102, 0.9)
		borderNormal = ui.RGBA(255, 255, 255, 0.08)
		borderChosen = ui.Hex("#3B82F6")
		accentColor = ui.Hex("#3B82F6")
		textPrimary = ui.Hex("#FFFFFF")
		textSecondary = ui.Hex("#94A3B8")
		textMuted = ui.Hex("#64748B")
		keycapNormal = ui.Hex("#0A0C10")
		keycapChosen = ui.Hex("#3B82F6")
		segBg = ui.Hex("#13161F")
		segActiveBg = ui.Hex("#252A38")
	} else {
		bgWindow = ui.Hex("#F8FAFC")
		heroCardBg = ui.Hex("#FFFFFF")
		cardNormalBg = ui.Hex("#FFFFFF")
		cardChosenBg = ui.Hex("#EFF6FF")
		borderNormal = ui.RGBA(0, 0, 0, 0.08)
		borderChosen = ui.Hex("#2563EB")
		accentColor = ui.Hex("#2563EB")
		textPrimary = ui.Hex("#0F172A")
		textSecondary = ui.Hex("#475569")
		textMuted = ui.Hex("#94A3B8")
		keycapNormal = ui.Hex("#F1F5F9")
		keycapChosen = ui.Hex("#2563EB")
		segBg = ui.Hex("#E2E8F0")
		segActiveBg = ui.Hex("#FFFFFF")
	}

	root := c.Root()
	root.Background(bgWindow)

	total := len(a.profiles)

	// 全局键盘快捷键
	if c.Shortcut(0, ui.KeyEscape) {
		mygo.App.Quit()
		return
	}
	if c.Shortcut(0, ui.KeyEnter) {
		if a.chosen >= 0 && a.chosen < total {
			a.openWith(a.profiles[a.chosen])
			return
		}
	}
	if c.Shortcut(0, ui.KeyRight) {
		a.chosen = (a.chosen + 1) % total
		c.Invalidate()
	}
	if c.Shortcut(0, ui.KeyLeft) {
		a.chosen = (a.chosen - 1 + total) % total
		c.Invalidate()
	}
	if c.Shortcut(0, ui.KeyDown) {
		if a.chosen+3 < total {
			a.chosen += 3
			c.Invalidate()
		}
	}
	if c.Shortcut(0, ui.KeyUp) {
		if a.chosen-3 >= 0 {
			a.chosen -= 3
			c.Invalidate()
		}
	}

	digitKeys := []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	for idx, key := range digitKeys {
		if idx < total && c.Shortcut(0, key) {
			a.openWith(a.profiles[idx])
			return
		}
	}

	host, pathStr, _ := parseURLDetails(a.targetURL)

	ui.Column(c).Fill().Padding(24).Gap(18).Children(func() {
		// =====================================================================
		// 1. TOP BAR: 品牌小徽标 + 严谨的三段式颜色模式切换开关
		// =====================================================================
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(3.5).Background(accentColor)
				ui.Text(c, "CHROME PROFILE ROUTER").
					FontSize(11).
					Bold().
					TextColor(textSecondary)
			})

			ui.Box(c).Grow(1)

			ui.Row(c).
				Padding(2).
				Gap(2).
				Radius(8).
				Background(segBg).
				Border(1, borderNormal).
				Children(func() {
					renderSegment(c, "跟随系统", a.themeMode == ThemeAuto, segActiveBg, textPrimary, textMuted, func() {
						a.themeMode = ThemeAuto
						c.Invalidate()
					})
					renderSegment(c, "浅色", a.themeMode == ThemeLight, segActiveBg, textPrimary, textMuted, func() {
						a.themeMode = ThemeLight
						c.Invalidate()
					})
					renderSegment(c, "深色", a.themeMode == ThemeDark, segActiveBg, textPrimary, textMuted, func() {
						a.themeMode = ThemeDark
						c.Invalidate()
					})
				})
		})

		// =====================================================================
		// 2. HERO DESTINATION BLOCK (核心视觉看板：大字号突出 URL)
		// =====================================================================
		ui.Column(c).
			Padding(14, 18).
			Gap(6).
			Radius(14).
			Background(heroCardBg).
			Border(1.2, borderNormal).
			Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, 0.04)).
			Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "TARGET URL").
						FontSize(9).
						Bold().
						TextColor(accentColor)

					ui.Box(c).Grow(1)

					ui.Text(c, "选择开启身份").
						FontSize(11).
						TextColor(textMuted)
				})

				ui.Row(c).AlignItems(ui.Baseline).Gap(6).Children(func() {
					ui.Text(c, host).
						FontSize(17).
						Bold().
						TextColor(textPrimary).
						SingleLine()

					if pathStr != "" {
						dispPath := pathStr
						if len(dispPath) > 42 {
							dispPath = dispPath[:39] + "..."
						}
						ui.Text(c, dispPath).
							FontSize(12).
							TextColor(textSecondary).
							SingleLine()
					}
				})
			})

		// =====================================================================
		// 3. FLUID APPLE TV 3D PARALLAX (全量物理弹簧 AnimateTo 插值，告别生硬瞬切)
		// =====================================================================
		ui.Grid(c).Columns(3).Gap(12).Children(func() {
			for i, p := range a.profiles {
				idx := i
				prof := p
				isChosen := (idx == a.chosen)

				cardBg := cardNormalBg
				cardBorder := borderNormal
				cardBorderW := float32(1)
				cardShadowBlur := float32(6)
				cardShadowCol := ui.RGBA(0, 0, 0, 0.04)
				nameColor := textPrimary
				subColor := textMuted
				avatarRingCol := borderNormal

				if isChosen {
					cardBg = cardChosenBg
					cardBorder = borderChosen
					cardBorderW = 1.8
					cardShadowBlur = 18
					cardShadowCol = accentColor.Alpha(0.32)
					nameColor = accentColor
					avatarRingCol = accentColor
					if isDark {
						subColor = ui.Hex("#BFDBFE")
					} else {
						subColor = accentColor.Alpha(0.85)
					}
				}

				const cardW float32 = 196
				const cardH float32 = 86

				card := ui.Row(c).Key(prof.Dir).
					Height(cardH).
					Padding(10, 12).
					Gap(10).
					AlignItems(ui.Center).
					Radius(12).
					Background(cardBg).
					Border(cardBorderW, cardBorder).
					Shadow(0, 3, cardShadowBlur, 0, cardShadowCol).
					Material(glass.Glass{Style: glass.Clear}).
					Transition(ui.ElementTransition{Duration: 150 * time.Millisecond, Colors: true})

				// 实时光标相对坐标监听
				px, py, isOver := card.PointerPosition()

				targetRotX := float32(0)
				targetRotY := float32(0)
				targetLift := float32(0)
				targetScale := float32(1.0)

				if isOver {
					if a.chosen != idx {
						a.chosen = idx
					}

					// Apple TV 3D 细腻视差倾斜计算
					normX := clamp((px-(cardW/2))/(cardW/2), -1.0, 1.0)
					normY := clamp((py-(cardH/2))/(cardH/2), -1.0, 1.0)

					const maxTilt float32 = 8.5
					targetRotX = -normY * maxTilt
					targetRotY = normX * maxTilt

					targetLift = -3.5
					targetScale = 1.03
				} else if isChosen {
					targetLift = -2.5
					targetScale = 1.02
				}

				// 核心关键：使用 MyGo 原生 AnimateTo 进行物理弹簧平滑阻尼插值！
				// 无论光标如何移动或离开，角度、位移和缩放绝不生硬跳变，而是由弹簧丝滑带过去！
				springTilt := ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.12}}
				springLift := ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: 0.16}}

				smoothRotX := card.AnimateTo("rotX", targetRotX, springTilt)
				smoothRotY := card.AnimateTo("rotY", targetRotY, springTilt)
				smoothLift := card.AnimateTo("lift", targetLift, springLift)
				smoothScale := card.AnimateTo("scale", targetScale, springLift)

				// 统一单链条 3D 透视变换：透视 -> 位移 -> 缩放 -> 3D双轴倾斜
				card.Perspective(750).
					Translate(0, smoothLift).
					Scale(smoothScale, smoothScale).
					RotateX(smoothRotX).
					RotateY(smoothRotY)

				card.Children(func() {
					// 头像圆环 (带有独立 3D 浮雕微阴影)
					ui.Row(c).
						Padding(1.5).
						Radius(20).
						Border(1.2, avatarRingCol).
						Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.15)).
						Transition(ui.ElementTransition{Duration: 150 * time.Millisecond, Colors: true}).
						Children(func() {
							if prof.Avatar != nil {
								ui.Image(c, prof.Avatar).Size(36, 36).Radius(18)
							} else {
								ui.Row(c).
									Size(36, 36).
									Radius(18).
									Center().
									AlignItems(ui.Center).
									Background(prof.Color.Alpha(0.18)).
									Border(1, prof.Color.Alpha(0.4)).
									Children(func() {
										letter := string([]rune(prof.Name)[0])
										ui.Text(c, letter).FontSize(14).Bold().TextColor(prof.Color)
									})
							}
						})

					// 中部文字信息
					ui.Column(c).Grow(1).Gap(2).Children(func() {
						ui.Text(c, prof.Name).
							FontSize(13).
							Bold().
							TextColor(nameColor).
							SingleLine()

						sub := prof.Email
						if sub == "" || sub == prof.Name {
							sub = prof.Dir
						}
						if len(sub) > 18 {
							sub = sub[:15] + "..."
						}
						ui.Text(c, sub).
							FontSize(10).
							TextColor(subColor).
							SingleLine()
					})

					// 右侧实体键帽
					if idx < 9 {
						kBg := keycapNormal
						kText := textMuted
						kBorder := borderNormal
						if isChosen {
							kBg = keycapChosen
							kText = ui.Hex("#FFFFFF")
							kBorder = accentColor
						}

						ui.Row(c).
							Padding(2, 6).
							Radius(4).
							Background(kBg).
							Border(1, kBorder).
							Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.2)).
							Children(func() {
								ui.Text(c, fmt.Sprintf("%d", idx+1)).
									FontSize(10).
									Bold().
									TextColor(kText)
							})
					}
				})

				if card.Clicked() {
					a.openWith(prof)
					return
				}
			}
		})
	})
}

func renderSegment(c *ui.Context, label string, active bool, activeBg, activeTxt, inactiveTxt ui.Color, onClick func()) {
	bg := ui.Transparent
	border := ui.Transparent
	txtCol := inactiveTxt

	if active {
		bg = activeBg
		border = ui.RGBA(0, 0, 0, 0.08)
		txtCol = activeTxt
	}

	seg := ui.Row(c).
		Padding(3, 8).
		Radius(6).
		Background(bg).
		Border(1, border).
		Children(func() {
			ui.Text(c, label).FontSize(10).Bold().TextColor(txtCol)
		})

	if seg.Clicked() {
		onClick()
	}
}

func main() {
	a := &selectorApp{
		targetURL: "",
		profiles:  loadChromeProfiles(),
		chosen:    0,
		themeMode: ThemeAuto,
	}

	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "file://") {
			a.targetURL = arg
			break
		}
	}

	mygo.App.OnOpenURL(func(u string) {
		a.targetURL = u
		if a.window != nil {
			a.window.Update(func() {})
			a.window.Show()
		}
	})

	mygo.App.OnOpenFile(func(f string) {
		a.targetURL = "file://" + f
		if a.window != nil {
			a.window.Update(func() {})
			a.window.Show()
		}
	})

	mygo.App.WhenReady(func() {
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title:             "Chrome Profile Router",
			Width:             660,
			Height:            490,
			MinWidth:          620,
			MinHeight:         460,
			DisableResize:     true,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			AlwaysOnTop:       true,
			Vibrancy:          mygo.VibrancyPopover,
			Content:           ui.View(a.view),
		})
		a.window.Show()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
