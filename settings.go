// [INPUT]: 依赖 mygo.PathUserData 给出的用户数据目录、mygo 的 Vibrancy/ThemeSource 常量与标准库 encoding/json、os。
// [OUTPUT]: 提供 <PathUserData>/settings.json 的读取、首次运行默认值写入、字段夹取与原子保存，外观与动效两组设置的内存写入（tint / material / 三个动效开关 / 减少动态效果档位）、标脏与冲刷，以及外观值到 mygo.Vibrancy / ThemeMode 的映射。
// [POS]: 根包的设置持久化层，只在启动、主题切换与设置面板改动时被 main.go / view.go 调用；不含界面、布局或渲染逻辑，也不碰原生窗口。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

const (
	settingsVersion  = 1
	settingsFileName = "settings.json"

	// 覆盖不透明度的可用范围。背景调过 0.70 基本把原生磨砂盖成实心板，
	// 卡片低于 0.30 会让地址卡上的小字读不清，因此两端都留有余量。
	backgroundTintMin = 0.00
	backgroundTintMax = 0.90
	cardTintMin       = 0.30
	cardTintMax       = 0.95

	// tintStep 是设置面板滑块的步长。0.02 让 0.00–0.90 与 0.30–0.95 都落在
	// 同一条刻度上，且每个可停位置都能用两位小数写清楚。
	tintStep = 0.02

	// 减少动态效果只有两档：跟随系统与强制开启。没有"强制关闭"——那会
	// 覆盖系统的 ReduceMotion，属于抢系统权威。
	motionReduceSystem = "system"
	motionReduceOn     = "on"
)

// tintPair 是按外观分开保存的覆盖不透明度：深色与浅色各一档，因为两套
// 外观的差值（0.04）是设计值而不是用户值，硬编码差值会让浅色下调不出
// 想要的结果。
type tintPair struct {
	Dark  float64 `json:"dark"`
	Light float64 `json:"light"`
}

// appearanceSettings 是设置文件里唯一的设置分组：主题、磨砂材质档位与
// 两层覆盖不透明度。material 是离散档位（NSVisualEffectView 的用途材质），
// 不是模糊半径；tint 是不透明度，不是模糊强度。
type appearanceSettings struct {
	Theme          string   `json:"theme"`
	Material       string   `json:"material"`
	BackgroundTint tintPair `json:"backgroundTint"`
	CardTint       tintPair `json:"cardTint"`
}

type settings struct {
	Version    int                `json:"version"`
	Appearance appearanceSettings `json:"appearance"`
	Motion     motionSettings     `json:"motion"`
}

// motionSettings 是动效分组：三个装饰性动效的开关，加一条"减少动态效果"
// 的显式声明。默认全开且跟随系统，因此缺这组键的文件与加入这组键之前的
// 版本行为一致。
//
// spin 关掉时选中卡只保留静态基础选中边框与柔光，不再推进流彩角度；
// hover 关掉时悬停只提亮底色与投影，放大与邻居让位走与 ReduceMotion 相同的
// 单位变换；tilt 关掉时不倾斜也不视差。reduceMotion 只允许 "system" 与
// "on"，没有 "off"。
type motionSettings struct {
	Spin         bool   `json:"spin"`
	Hover        bool   `json:"hover"`
	Tilt         bool   `json:"tilt"`
	ReduceMotion string `json:"reduceMotion"`
}

// defaultMotion 必须与加入这组键之前的行为等价：三个动效都在跑，减少动态
// 效果只跟随系统。
func defaultMotion() motionSettings {
	return motionSettings{Spin: true, Hover: true, Tilt: true, ReduceMotion: motionReduceSystem}
}

// normalized 把动效分组收敛成可渲染的值：整组为零值（离屏测试里从未读过
// 设置文件）时回落全部默认，未知的 reduceMotion 回落 "system"。
//
// 三个开关的零值 false 只有整组为零值时才被当成"未设置"：用户显式写下
// "spin": false 时 ReduceMotion 已被补成 "system"，整组不再等于零值，
// 因此不会被改回 true。
func (m motionSettings) normalized() motionSettings {
	def := defaultMotion()
	if m == (motionSettings{}) {
		return def
	}
	if m.ReduceMotion != motionReduceSystem && m.ReduceMotion != motionReduceOn {
		m.ReduceMotion = def.ReduceMotion
	}
	return m
}

// materialVibrancy 只收 mygo 在 macOS 上真实存在的材质常量名，因此文件里
// 写错一个名字不会变成"没材质"，而是回落到 popover。
var materialVibrancy = map[string]mygo.Vibrancy{
	"window":        mygo.VibrancyWindow,
	"content":       mygo.VibrancyContent,
	"sidebar":       mygo.VibrancySidebar,
	"header":        mygo.VibrancyHeader,
	"titlebar":      mygo.VibrancyTitlebar,
	"under-window":  mygo.VibrancyUnderWindow,
	"under-page":    mygo.VibrancyUnderPage,
	"menu":          mygo.VibrancyMenu,
	"popover":       mygo.VibrancyPopover,
	"hud":           mygo.VibrancyHUD,
	"sheet":         mygo.VibrancySheet,
	"tooltip":       mygo.VibrancyTooltip,
	"selection":     mygo.VibrancySelection,
	"fullscreen-ui": mygo.VibrancyFullScreenUI,
}

var themeModes = map[string]ThemeMode{
	"auto":  ThemeAuto,
	"light": ThemeLight,
	"dark":  ThemeDark,
}

// materialOrder 是设置面板给出的四个材质档位，顺序即分段控件的顺序。文件里
// 仍可手写 materialVibrancy 认得的其它用途材质（window / menu / tooltip…），
// 面板只把这四个做成可选档位：档位越多越难分辨，而它们都不是模糊半径。
var materialOrder = []string{"popover", "hud", "sidebar", "under-window"}

// materialIndex 把材质名翻成面板里的档位下标；文件里手写的其它材质不在
// 面板档位里，显示为第一档（popover），但不会被改写，直到用户主动换档。
func materialIndex(name string) int {
	for i, m := range materialOrder {
		if m == name {
			return i
		}
	}
	return 0
}

// defaultAppearance 必须与加入设置文件之前的硬编码完全一致：删除
// settings.json 后的外观与之前的版本逐像素相同。
func defaultAppearance() appearanceSettings {
	return appearanceSettings{
		Theme:          "auto",
		Material:       "popover",
		BackgroundTint: tintPair{Dark: 0.30, Light: 0.34},
		CardTint:       tintPair{Dark: 0.62, Light: 0.66},
	}
}

func defaultSettings() settings {
	return settings{Version: settingsVersion, Appearance: defaultAppearance(), Motion: defaultMotion()}
}

// normalized 把任意来源的外观值收敛成可渲染的值：未知主题/材质回落默认，
// 整对缺失（零值）的 tint 用默认值，越界夹到边界。它同时兜住离屏测试里
// 零值 selectorApp 的情况，因此不会出现 0 覆盖。
func (ap appearanceSettings) normalized() appearanceSettings {
	def := defaultAppearance()
	if _, ok := themeModes[ap.Theme]; !ok {
		ap.Theme = def.Theme
	}
	if _, ok := materialVibrancy[ap.Material]; !ok {
		ap.Material = def.Material
	}
	if ap.BackgroundTint == (tintPair{}) {
		ap.BackgroundTint = def.BackgroundTint
	}
	if ap.CardTint == (tintPair{}) {
		ap.CardTint = def.CardTint
	}
	ap.BackgroundTint.Dark = clampTint(ap.BackgroundTint.Dark, backgroundTintMin, backgroundTintMax)
	ap.BackgroundTint.Light = clampTint(ap.BackgroundTint.Light, backgroundTintMin, backgroundTintMax)
	ap.CardTint.Dark = clampTint(ap.CardTint.Dark, cardTintMin, cardTintMax)
	ap.CardTint.Light = clampTint(ap.CardTint.Light, cardTintMin, cardTintMax)
	return ap
}

func clampTint(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// vibrancyForMaterial 把设置里的档位名翻成窗口材质；未知名字回落 popover，
// 与默认值一致。
func vibrancyForMaterial(name string) mygo.Vibrancy {
	if v, ok := materialVibrancy[name]; ok {
		return v
	}
	return mygo.VibrancyPopover
}

func themeModeForName(name string) ThemeMode {
	if m, ok := themeModes[name]; ok {
		return m
	}
	return ThemeAuto
}

func (m ThemeMode) settingsName() string {
	for name, mode := range themeModes {
		if mode == m {
			return name
		}
	}
	return "auto"
}

func themeSource(m ThemeMode) mygo.ThemeSource {
	switch m {
	case ThemeLight:
		return mygo.ThemeLight
	case ThemeDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

func settingsPath(dir string) string { return filepath.Join(dir, settingsFileName) }

// raw* 用指针区分"字段缺失"与"显式写了零值"，缺失的字段由默认值补齐，
// 因此部分手写的文件也能用。
type rawTint struct {
	Dark  *float64 `json:"dark"`
	Light *float64 `json:"light"`
}

type rawAppearance struct {
	Theme          *string  `json:"theme"`
	Material       *string  `json:"material"`
	BackgroundTint *rawTint `json:"backgroundTint"`
	CardTint       *rawTint `json:"cardTint"`
}

type rawMotion struct {
	Spin         *bool   `json:"spin"`
	Hover        *bool   `json:"hover"`
	Tilt         *bool   `json:"tilt"`
	ReduceMotion *string `json:"reduceMotion"`
}

type rawSettings struct {
	Version    *int           `json:"version"`
	Appearance *rawAppearance `json:"appearance"`
	Motion     *rawMotion     `json:"motion"`
}

// loadSettings 读取 dir 下的设置文件。第二个返回值表示这份设置能否写回：
// 文件来自更新版本、已损坏或目录不可用时为 false，避免我们覆盖用户的文件。
//
// 容错规则：文件缺失 → 写入一份默认值（让用户能发现并手改）；JSON 损坏、
// 字段缺失、未知键 → 用默认值继续运行且不重写文件；越界 → 夹到边界；
// version > 1 → 用默认值运行且不回写。
func loadSettings(dir string) (settings, bool) {
	def := defaultSettings()
	if dir == "" {
		return def, false
	}
	data, err := os.ReadFile(settingsPath(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// 首次运行：写一份默认值文件。写失败只影响可发现性，不阻塞启动。
			if saveSettings(dir, def) == nil {
				return def, true
			}
			return def, false
		}
		return def, false
	}

	var raw rawSettings
	if err := json.Unmarshal(data, &raw); err != nil {
		// 用户可能正在手改这个文件，回写会把他的内容吃掉，所以只读不写。
		return def, false
	}
	if raw.Version != nil && *raw.Version > settingsVersion {
		// 来自更新版本的设置：不认识就不猜，按默认外观运行且不写回。
		return def, false
	}

	ap := def.Appearance
	if raw.Appearance != nil {
		in := raw.Appearance
		if in.Theme != nil {
			ap.Theme = *in.Theme
		}
		if in.Material != nil {
			ap.Material = *in.Material
		}
		if in.BackgroundTint != nil {
			if in.BackgroundTint.Dark != nil {
				ap.BackgroundTint.Dark = *in.BackgroundTint.Dark
			}
			if in.BackgroundTint.Light != nil {
				ap.BackgroundTint.Light = *in.BackgroundTint.Light
			}
		}
		if in.CardTint != nil {
			if in.CardTint.Dark != nil {
				ap.CardTint.Dark = *in.CardTint.Dark
			}
			if in.CardTint.Light != nil {
				ap.CardTint.Light = *in.CardTint.Light
			}
		}
	}
	motion := def.Motion
	if raw.Motion != nil {
		in := raw.Motion
		if in.Spin != nil {
			motion.Spin = *in.Spin
		}
		if in.Hover != nil {
			motion.Hover = *in.Hover
		}
		if in.Tilt != nil {
			motion.Tilt = *in.Tilt
		}
		if in.ReduceMotion != nil {
			motion.ReduceMotion = *in.ReduceMotion
		}
	}
	return settings{Version: settingsVersion, Appearance: ap.normalized(), Motion: motion.normalized()}, true
}

// saveSettings 原子写入：同目录 .tmp 再 rename，中途失败不会留下半份文件，
// 也不会破坏旧文件。失败只返回错误，调用方不弹窗。
//
// 写盘前先读回现有文件并按层合并：用户手改时加的未知键（顶层与 appearance
// 内都算）必须原样保留——这个文件是用户会手改的配置文件，丢键比丢主题严重。
// 已知字段一律以内存里的当前值（已夹取）为准；文件缺失或已损坏时从空 map
// 开始，写入照常进行。
func saveSettings(dir string, s settings) error {
	if dir == "" {
		return errors.New("settings: 用户数据目录不可用")
	}
	s.Version = settingsVersion
	// 写盘前再收敛一次：调用方可能只改了内存里的一个字段，缺省值不能因此
	// 变成零值写进文件（尤其是 reduceMotion 的空串）。
	s.Appearance = s.Appearance.normalized()
	s.Motion = s.Motion.normalized()

	merged := map[string]json.RawMessage{}
	if data, err := os.ReadFile(settingsPath(dir)); err == nil {
		var existing map[string]json.RawMessage
		if err := json.Unmarshal(data, &existing); err == nil {
			merged = existing
		}
	}

	if err := mergeGroup(merged, "appearance", map[string]any{
		"theme":          s.Appearance.Theme,
		"material":       s.Appearance.Material,
		"backgroundTint": s.Appearance.BackgroundTint,
		"cardTint":       s.Appearance.CardTint,
	}); err != nil {
		return err
	}
	if err := mergeGroup(merged, "motion", map[string]any{
		"spin":         s.Motion.Spin,
		"hover":        s.Motion.Hover,
		"tilt":         s.Motion.Tilt,
		"reduceMotion": s.Motion.ReduceMotion,
	}); err != nil {
		return err
	}

	versionRaw, err := json.Marshal(s.Version)
	if err != nil {
		return err
	}
	merged["version"] = versionRaw

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := settingsPath(dir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// mergeGroup 把 known 写回 merged[name] 这一层：用户在该层手写的未知键原样
// 保留，已知字段以内存里的当前值为准。层缺失或已损坏时从空 map 开始，写入
// 照常进行。
func mergeGroup(merged map[string]json.RawMessage, name string, known map[string]any) error {
	group := map[string]json.RawMessage{}
	if raw, ok := merged[name]; ok {
		var existing map[string]json.RawMessage
		if err := json.Unmarshal(raw, &existing); err == nil {
			group = existing
		}
	}
	for key, value := range known {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		group[key] = raw
	}
	raw, err := json.Marshal(group)
	if err != nil {
		return err
	}
	merged[name] = raw
	return nil
}

// settingsDir 返回设置文件所在的用户数据目录。dirOverride 只给测试注入临时
// 目录用；生产路径永远是 mygo.App.Path(PathUserData)。
func (a *selectorApp) settingsDir() (string, error) {
	a.mu.Lock()
	dir := a.settingsDirOverride
	a.mu.Unlock()
	if dir != "" {
		return dir, nil
	}
	return mygo.App.Path(mygo.PathUserData)
}

// applySettings 在窗口创建之前把设置读进 selectorApp：Vibrancy 是构造参数，
// 晚读会先闪一帧默认材质。目录不可用或文件损坏都按默认值继续启动。
func (a *selectorApp) applySettings() {
	dir, err := a.settingsDir()
	if err != nil {
		a.mu.Lock()
		a.settings, a.settingsWritable = defaultSettings(), false
		a.mu.Unlock()
		return
	}
	s, writable := loadSettings(dir)
	a.mu.Lock()
	a.settings, a.settingsWritable = s, writable
	a.themeMode = themeModeForName(s.Appearance.Theme)
	a.mu.Unlock()
}

// persistSettingsNow 立刻把内存里的设置原子写回文件，供主题、材质、开关与
// 恢复默认这类离散改动使用；写失败不影响运行，也不弹窗。
func (a *selectorApp) persistSettingsNow() {
	a.mu.Lock()
	writable, s := a.settingsWritable, a.settings
	a.settingsDirty = false
	a.mu.Unlock()
	if !writable {
		return
	}
	if dir, err := a.settingsDir(); err == nil {
		_ = saveSettings(dir, s)
	}
}

// markSettingsDirty 记录"内存与文件已经不一致"。滑块拖动期间每帧都在变，
// 逐帧原子写盘没有意义，因此在面板关闭、离散项改动与退出前各冲刷一次。
func (a *selectorApp) markSettingsDirty() {
	a.mu.Lock()
	a.settingsDirty = true
	a.mu.Unlock()
}

// flushSettings 把标脏的设置写回文件；不脏时什么也不做。
func (a *selectorApp) flushSettings() {
	a.mu.Lock()
	dirty := a.settingsDirty
	a.mu.Unlock()
	if dirty {
		a.persistSettingsNow()
	}
}

// persistTheme 把用户刚选的主题写回设置文件。只在主题真的变化时调用一次，
// 不逐帧写盘；写失败不影响运行。
func (a *selectorApp) persistTheme(m ThemeMode) {
	a.mu.Lock()
	a.settings.Appearance.Theme = m.settingsName()
	a.mu.Unlock()
	a.persistSettingsNow()
}

// quantizeTint 把任意值夹到 [lo, hi] 并量化到 tintStep 的整数倍，保证写进
// 设置文件与界面上读到的数字是同一个（不会出现 0.3199999）。
//
// 量化之后再夹一次：cardTintMax（0.95）不是 0.02 的整数倍，四舍五入会到
// 0.96，必须回到边界值。
func quantizeTint(v, lo, hi float64) float64 {
	v = clampTint(v, lo, hi)
	return clampTint(math.Round(v/tintStep)*tintStep, lo, hi)
}

// setTint 把滑块的值写进 dark 那一侧的覆盖不透明度。background 为真时写
// backgroundTint，否则写 cardTint。只改内存并标脏——拖动滑块时每帧都会调用，
// 落盘由面板关闭/退出前的 flushSettings 负责。返回是否真的变化。
func (a *selectorApp) setTint(background, dark bool, v float64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	ap := &a.settings.Appearance
	pair := &ap.BackgroundTint
	lo, hi := backgroundTintMin, backgroundTintMax
	if !background {
		pair = &ap.CardTint
		lo, hi = cardTintMin, cardTintMax
	}
	v = quantizeTint(v, lo, hi)
	side := &pair.Light
	if dark {
		side = &pair.Dark
	}
	if *side == v {
		return false
	}
	*side = v
	a.settingsDirty = true
	return true
}

// setMaterialSetting 写入材质档位名并立刻写盘；换原生材质由调用方经
// Window.Update 落到窗口（settings.go 不碰窗口）。返回是否真的变化。
func (a *selectorApp) setMaterialSetting(name string) bool {
	a.mu.Lock()
	if a.settings.Appearance.Material == name {
		a.mu.Unlock()
		return false
	}
	a.settings.Appearance.Material = name
	a.mu.Unlock()
	a.persistSettingsNow()
	return true
}

// setMotionReduce 写入减少动态效果的档位（"system" 或 "on"）并立刻写盘。
// 未知值回落 "system"；不存在"强制关闭"。返回是否真的变化。
func (a *selectorApp) setMotionReduce(mode string) bool {
	if mode != motionReduceOn {
		mode = motionReduceSystem
	}
	a.mu.Lock()
	if a.settings.Motion.ReduceMotion == mode {
		a.mu.Unlock()
		return false
	}
	a.settings.Motion.ReduceMotion = mode
	a.mu.Unlock()
	a.persistSettingsNow()
	return true
}

// resetSettings 把外观与动效恢复成默认值（含材质与四个 tint），并立刻写盘。
// 面板的滑块绑定值由调用方在下一帧重新同步。
func (a *selectorApp) resetSettings() {
	a.mu.Lock()
	a.settings.Appearance = defaultAppearance()
	a.settings.Motion = defaultMotion()
	a.settingsDirty = true
	a.mu.Unlock()
	a.persistSettingsNow()
}
