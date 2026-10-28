// [INPUT]: 依赖 settings.go 的读写、夹取与版本判定，以及标准库 os、encoding/json。
// [OUTPUT]: 保护设置文件的首次生成、损坏回落、越界夹取、未知键容错与更新版本不回写。
// [POS]: 根包的设置持久化回归，只读写临时目录，不触碰用户真实配置。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/egoist/mygo"
)

func writeSettingsFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(settingsPath(dir), []byte(body), 0o644); err != nil {
		t.Fatalf("写入测试设置失败: %v", err)
	}
}

func readSettingsFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(settingsPath(dir))
	if err != nil {
		t.Fatalf("读取测试设置失败: %v", err)
	}
	return string(data)
}

// TestMissingSettingsFileWritesDefaults 约束首次运行：没有文件时按默认值运行，
// 并留下可供用户发现和手改的默认值文件（默认值必须等于加入设置前的硬编码）。
func TestMissingSettingsFileWritesDefaults(t *testing.T) {
	dir := t.TempDir()
	got, writable := loadSettings(dir)
	if !writable {
		t.Fatal("首次运行生成的默认值文件必须可写回")
	}
	if got != defaultSettings() {
		t.Fatalf("缺失文件没有回落到默认值: %#v", got)
	}
	want := defaultAppearance()
	if want.Theme != "auto" || want.Material != "popover" ||
		want.BackgroundTint != (tintPair{Dark: 0.30, Light: 0.34}) ||
		want.CardTint != (tintPair{Dark: 0.62, Light: 0.66}) {
		t.Fatalf("默认外观与既有视觉基线不一致: %#v", want)
	}

	var onDisk settings
	if err := json.Unmarshal([]byte(readSettingsFile(t, dir)), &onDisk); err != nil {
		t.Fatalf("写出的默认值文件不是合法 JSON: %v", err)
	}
	if onDisk != defaultSettings() {
		t.Fatalf("写出的默认值文件与内存默认值不一致: %#v", onDisk)
	}
	if _, err := os.Stat(settingsPath(dir) + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("原子写留下了临时文件: %v", err)
	}
}

// TestCorruptSettingsFallBackWithoutRewriting 约束损坏文件：按默认值启动，
// 且不回写——用户可能正在手改，覆盖会吃掉他的内容。
func TestCorruptSettingsFallBackWithoutRewriting(t *testing.T) {
	dir := t.TempDir()
	const broken = "{ this is not json"
	writeSettingsFile(t, dir, broken)

	got, writable := loadSettings(dir)
	if got != defaultSettings() {
		t.Fatalf("损坏文件没有回落到默认值: %#v", got)
	}
	if writable {
		t.Fatal("损坏文件不允许被回写")
	}
	if body := readSettingsFile(t, dir); body != broken {
		t.Fatalf("读取失败时覆盖了用户文件: %q", body)
	}
}

// TestOutOfRangeTintsAreClamped 约束越界：夹到边界，且不丢失同文件里的其他字段。
func TestOutOfRangeTintsAreClamped(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, `{
  "version": 1,
  "appearance": {
    "material": "sidebar",
    "backgroundTint": { "dark": 1.5, "light": -0.4 },
    "cardTint": { "dark": 0.1, "light": 0.99 }
  }
}`)

	got, _ := loadSettings(dir)
	ap := got.Appearance
	if ap.BackgroundTint != (tintPair{Dark: backgroundTintMax, Light: backgroundTintMin}) {
		t.Fatalf("背景不透明度没有被夹到 [0.00,0.90]: %#v", ap.BackgroundTint)
	}
	if ap.CardTint != (tintPair{Dark: cardTintMin, Light: cardTintMax}) {
		t.Fatalf("卡片不透明度没有被夹到 [0.30,0.95]: %#v", ap.CardTint)
	}
	if ap.Material != "sidebar" {
		t.Fatalf("夹取丢掉了合法的材质字段: %q", ap.Material)
	}
	if ap.Theme != "auto" {
		t.Fatalf("缺失的主题字段没有补默认值: %q", ap.Theme)
	}
}

// TestUnknownKeysAreIgnoredAndFileIsKept 约束未知键：不导致读取失败，也不被
// 我们顺手重写（降级版本不得吃掉更新版本写下的键）。
func TestUnknownKeysAreIgnoredAndFileIsKept(t *testing.T) {
	dir := t.TempDir()
	const body = `{
  "version": 1,
  "appearance": { "material": "sidebar", "futureKnob": 0.5 },
  "motion": { "reduceMotion": "system" },
  "unknownSection": { "a": 1 }
}`
	writeSettingsFile(t, dir, body)

	got, writable := loadSettings(dir)
	if !writable {
		t.Fatal("含未知键的合法文件应当可写回")
	}
	if got.Appearance.Material != "sidebar" || got.Appearance.Theme != "auto" {
		t.Fatalf("未知键影响了已知字段: %#v", got.Appearance)
	}
	if got.Appearance.BackgroundTint != defaultAppearance().BackgroundTint ||
		got.Appearance.CardTint != defaultAppearance().CardTint {
		t.Fatalf("缺失的 tint 字段没有补默认值: %#v", got.Appearance)
	}
	if readSettingsFile(t, dir) != body {
		t.Fatal("读取过程改写了用户文件")
	}
}

// TestNewerVersionRunsDefaultsWithoutWriting 约束版本保护：version > 1 时用
// 默认值运行，且绝不回写。
func TestNewerVersionRunsDefaultsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	const body = `{
  "version": 2,
  "appearance": {
    "theme": "dark",
    "material": "hud",
    "backgroundTint": { "dark": 0.15, "light": 0.15 }
  }
}`
	writeSettingsFile(t, dir, body)

	got, writable := loadSettings(dir)
	if got != defaultSettings() {
		t.Fatalf("更新版本的设置没有回落到默认值: %#v", got)
	}
	if writable {
		t.Fatal("更新版本的设置文件不允许被回写")
	}
	if readSettingsFile(t, dir) != body {
		t.Fatal("更新版本的设置文件被改写")
	}
}

// TestUnknownMaterialFallsBackToPopover 约束材质档位只认真实存在的常量名：
// 写错名字不会变成"没有材质"。
func TestUnknownMaterialFallsBackToPopover(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, `{"version":1,"appearance":{"material":"blur-50"}}`)

	if got, _ := loadSettings(dir); got.Appearance.Material != "popover" {
		t.Fatalf("未知材质没有回落到 popover: %q", got.Appearance.Material)
	}
	cases := map[string]mygo.Vibrancy{
		"popover": mygo.VibrancyPopover,
		"hud":     mygo.VibrancyHUD,
		"sidebar": mygo.VibrancySidebar,
		"blur-50": mygo.VibrancyPopover,
		"":        mygo.VibrancyPopover,
		"mica":    mygo.VibrancyPopover, // Windows 材质名在 macOS 上不存在
	}
	for name, want := range cases {
		if got := vibrancyForMaterial(name); got != want {
			t.Fatalf("材质 %q 映射为 %q，期望 %q", name, got, want)
		}
	}
}

// TestThemePersistsAndRoundTrips 约束主题写回：保存后重新读取得到同一主题，
// 且其余外观字段不被顺手改动。
func TestThemePersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	loadSettings(dir) // 首次运行生成默认值文件

	s := defaultSettings()
	s.Appearance.Theme = ThemeDark.settingsName()
	if err := saveSettings(dir, s); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}
	got, writable := loadSettings(dir)
	if !writable {
		t.Fatal("自写文件应当可写回")
	}
	if got != s {
		t.Fatalf("主题写回后读回不一致: %#v != %#v", got, s)
	}
	if themeModeForName(got.Appearance.Theme) != ThemeDark {
		t.Fatalf("主题名 %q 没有映射回深色", got.Appearance.Theme)
	}
	for _, m := range []ThemeMode{ThemeAuto, ThemeLight, ThemeDark} {
		if themeModeForName(m.settingsName()) != m {
			t.Fatalf("主题 %v 的名称往返失败: %q", m, m.settingsName())
		}
	}
	if _, err := os.Stat(settingsPath(dir) + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("原子写留下了临时文件: %v", err)
	}
}

// TestSaveKeepsUnknownKeysAcrossThemeWrite 约束写盘合并：切主题那一次写盘
// 不得吃掉用户手改时加的未知键（顶层与 appearance 内都算），而已知字段必须
// 更新为内存里的当前值。
func TestSaveKeepsUnknownKeysAcrossThemeWrite(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, `{
  "version": 1,
  "note": "keep me",
  "appearance": {
    "theme": "light",
    "material": "popover",
    "backgroundTint": { "dark": 0.30, "light": 0.34 },
    "cardTint": { "dark": 0.62, "light": 0.66 },
    "unknown": 1
  }
}`)

	s, writable := loadSettings(dir)
	if !writable {
		t.Fatal("合法文件应当可写回")
	}
	s.Appearance.Theme = ThemeDark.settingsName()
	if err := saveSettings(dir, s); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readSettingsFile(t, dir)), &top); err != nil {
		t.Fatalf("写出的文件不是合法 JSON: %v", err)
	}
	note, ok := top["note"]
	if !ok {
		t.Fatal("写盘吃掉了未知顶层键 note")
	}
	var noteText string
	if err := json.Unmarshal(note, &noteText); err != nil || noteText != "keep me" {
		t.Fatalf("未知顶层键的值被改写: %s (%v)", note, err)
	}
	var appearance map[string]json.RawMessage
	if err := json.Unmarshal(top["appearance"], &appearance); err != nil {
		t.Fatalf("appearance 不是对象: %v", err)
	}
	if _, ok := appearance["unknown"]; !ok {
		t.Fatal("写盘吃掉了 appearance 内的未知子键")
	}

	got, _ := loadSettings(dir)
	if got.Appearance.Theme != "dark" {
		t.Fatalf("主题没有写进文件: %q", got.Appearance.Theme)
	}
	if got.Appearance.BackgroundTint != defaultAppearance().BackgroundTint ||
		got.Appearance.CardTint != defaultAppearance().CardTint {
		t.Fatalf("写盘改动了已知的 tint 字段: %#v", got.Appearance)
	}
	if _, err := os.Stat(settingsPath(dir) + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("原子写留下了临时文件: %v", err)
	}
}

// TestUnavailableSettingsDirRunsOnDefaults 约束目录不可用：不阻塞启动，
// 也不尝试写盘。
func TestUnavailableSettingsDirRunsOnDefaults(t *testing.T) {
	got, writable := loadSettings("")
	if got != defaultSettings() || writable {
		t.Fatalf("目录不可用时没有回落到只读默认值: %#v writable=%v", got, writable)
	}
	if err := saveSettings("", defaultSettings()); err == nil {
		t.Fatal("目录不可用时保存必须报错而不是静默成功")
	}
}

// TestMotionKeysFallBackToDefaults 约束动效分组的容错：缺键、部分缺键与未知
// 的 reduceMotion 都回落默认（三个开关 true、reduceMotion "system"），而用户
// 显式写下的 false 必须保留。
func TestMotionKeysFallBackToDefaults(t *testing.T) {
	cases := []struct {
		name string
		body string
		want motionSettings
	}{
		{
			name: "missing motion group",
			body: `{"version":1,"appearance":{"theme":"dark"}}`,
			want: defaultMotion(),
		},
		{
			name: "only reduceMotion on",
			body: `{"version":1,"motion":{"reduceMotion":"on"}}`,
			want: motionSettings{Spin: true, Hover: true, Tilt: true, ReduceMotion: motionReduceOn},
		},
		{
			name: "explicit false is kept",
			body: `{"version":1,"motion":{"spin":false,"hover":false}}`,
			want: motionSettings{Spin: false, Hover: false, Tilt: true, ReduceMotion: motionReduceSystem},
		},
		{
			name: "unknown reduceMotion falls back",
			body: `{"version":1,"motion":{"tilt":false,"reduceMotion":"off"}}`,
			want: motionSettings{Spin: true, Hover: true, Tilt: false, ReduceMotion: motionReduceSystem},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSettingsFile(t, dir, tc.body)
			got, writable := loadSettings(dir)
			if !writable {
				t.Fatal("这份文件应当可以写回")
			}
			if got.Motion != tc.want {
				t.Fatalf("动效分组回落错误: got %+v, want %+v", got.Motion, tc.want)
			}
		})
	}
}

// TestSaveWritesMotionGroupAndKeepsUnknownKeys 约束写盘合并：motion 这一层的
// 未知键必须原样保留，已知字段以内存值为准。
func TestSaveWritesMotionGroupAndKeepsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, `{
  "version": 1,
  "appearance": {"theme": "auto", "material": "popover", "backgroundTint": {"dark": 0.3, "light": 0.34}, "cardTint": {"dark": 0.62, "light": 0.66}},
  "motion": {"spin": true, "hover": true, "tilt": true, "reduceMotion": "system", "futureKey": {"keep": 7}}
}`)

	s := defaultSettings()
	s.Motion = motionSettings{Spin: false, Hover: true, Tilt: false, ReduceMotion: motionReduceOn}
	if err := saveSettings(dir, s); err != nil {
		t.Fatalf("写盘失败: %v", err)
	}

	var onDisk map[string]any
	if err := json.Unmarshal([]byte(readSettingsFile(t, dir)), &onDisk); err != nil {
		t.Fatalf("写出的文件不是合法 JSON: %v", err)
	}
	motion, ok := onDisk["motion"].(map[string]any)
	if !ok {
		t.Fatalf("文件里没有 motion 分组: %#v", onDisk["motion"])
	}
	if motion["spin"] != false || motion["tilt"] != false || motion["hover"] != true {
		t.Fatalf("motion 的已知字段没有按内存值写入: %#v", motion)
	}
	if motion["reduceMotion"] != motionReduceOn {
		t.Fatalf("reduceMotion 没有写回: %#v", motion["reduceMotion"])
	}
	if _, ok := motion["futureKey"]; !ok {
		t.Fatalf("写盘吃掉了 motion 里的未知键: %#v", motion)
	}
}

// TestResetSettingsWritesDefaultsImmediately 约束恢复默认：内存与磁盘同时回到
// 默认外观与默认动效，且立刻落盘（不走延迟冲刷）。
func TestResetSettingsWritesDefaultsImmediately(t *testing.T) {
	dir := t.TempDir()
	writeSettingsFile(t, dir, `{
  "version": 1,
  "appearance": {"theme": "dark", "material": "hud", "backgroundTint": {"dark": 0.10, "light": 0.12}, "cardTint": {"dark": 0.90, "light": 0.92}},
  "motion": {"spin": false, "hover": false, "tilt": false, "reduceMotion": "on"}
}`)

	app := &selectorApp{settingsDirOverride: dir, settingsWritable: true}
	app.applySettings()
	if got := app.settings.Appearance.Material; got != "hud" {
		t.Fatalf("测试前置条件不成立: material=%q", got)
	}
	app.resetSettings()

	if app.settings.Appearance != defaultAppearance() || app.settings.Motion != defaultMotion() {
		t.Fatalf("恢复默认没有改内存: %+v %+v", app.settings.Appearance, app.settings.Motion)
	}
	onDisk, writable := loadSettings(dir)
	if !writable {
		t.Fatal("恢复默认后文件应当仍可写回")
	}
	if onDisk != defaultSettings() {
		t.Fatalf("恢复默认没有落盘: %#v", onDisk)
	}
	if app.settingsDirty {
		t.Fatal("恢复默认后仍标脏，说明没有真正写盘")
	}
}

// TestSetTintWritesCurrentSideOnly 约束浓度写入：只改当前外观那一侧，越界被
// 夹取、量化到 0.02，并且标脏等待冲刷（拖动期间不逐帧写盘）。
func TestSetTintWritesCurrentSideOnly(t *testing.T) {
	app := &selectorApp{settings: defaultSettings()}
	if !app.setTint(true, true, 0.3123) {
		t.Fatal("合法的浓度改动应当报告变化")
	}
	if got := app.settings.Appearance.BackgroundTint.Dark; got != 0.32 {
		t.Fatalf("深色侧浓度没有量化到 0.02: %v", got)
	}
	if got := app.settings.Appearance.BackgroundTint.Light; got != defaultAppearance().BackgroundTint.Light {
		t.Fatalf("改深色侧时动了浅色侧: %v", got)
	}
	if !app.settingsDirty {
		t.Fatal("浓度改动没有标脏")
	}
	if app.setTint(true, true, 0.32) {
		t.Fatal("同一个值不应报告变化")
	}
	if !app.setTint(false, false, 5) {
		t.Fatal("越界值应当被夹取后写入")
	}
	if got := app.settings.Appearance.CardTint.Light; got != cardTintMax {
		t.Fatalf("越界浓度没有夹到上限: %v", got)
	}
	if got := app.settings.Appearance.CardTint.Dark; got != defaultAppearance().CardTint.Dark {
		t.Fatalf("改浅色侧时动了深色侧: %v", got)
	}
}
