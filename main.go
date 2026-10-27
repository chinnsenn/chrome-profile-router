// [INPUT]: 依赖 Chrome 的 Local State、MyGo 应用/窗口/屏幕/主题 API、view.go 的渲染入口，以及 layout.go 的尺寸预算。
// [OUTPUT]: 提供 Profile 发现、链接请求队列、Chrome 启动与退出的唯一实现，创建并布局窗口，并把系统的降低透明度偏好同步给界面。
// [POS]: 根包的状态与路由层；界面在 view.go，配色与动效在 view_style.go，纯布局计算在 layout.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// launchTimeout 限制单次 Chrome 启动动作的最长等待时间，避免 open 卡死
// 时把整个 UI 拖进永久 loading 状态。
const launchTimeout = 8 * time.Second

// HomeURL 在没有收到任何外部链接请求时的兜底目标。
const HomeURL = "https://www.google.com"

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

// requestKind 区分外部请求的两种来源：URL 与本地文件路径。
type requestKind int

const (
	requestURL requestKind = iota
	requestFile
)

// request 是链接请求队列中的一项。文件请求保留原始路径，在实际启动时才
// 转成 file:// URL，以免在入队阶段就丢失路径信息。
//
// ID 是唯一标识，由 enqueue 单调递增分配。出队时必须靠它确认"本次启动
// 打开的确实是这一条"，否则只能用位置下标猜测，会把启动期间新入队的
// 请求误删。
type request struct {
	ID    uint64
	Kind  requestKind
	Value string
}

type selectorApp struct {
	mu sync.Mutex

	// pending 是"防覆写丢链"的核心：所有入口（命令行参数 / OnOpenURL /
	// OnOpenFile）都只入队，不覆盖当前正在处理的目标。
	pending []request
	// nextID 单调递增，为每条入队请求分配唯一 ID（仅在持有 mu 时读取）。
	nextID uint64

	profiles  []Profile
	loadErr   string
	chosen    int
	window    *mygo.Window
	themeMode ThemeMode

	// placed 与 layoutCount 记录窗口布局状态，只在主线程读写：placed 表示
	// 已经完成首次定位（首次居中，之后保持用户拖到的位置），layoutCount 是
	// 上次布局使用的身份数量。界面渲染从不读取它们，因此不进 snapshot。
	placed      bool
	layoutCount int

	// 启动状态：launching 防重入，launchError 用于界面回显失败原因。
	launching   bool
	launchError string

	// reduceTransparency 是系统"降低透明度"偏好，由 WhenReady 读取并在
	// 主题偏好变化时刷新；界面只通过 snapshot 读取，据此把半透明表面换成
	// 实色。
	reduceTransparency bool

	// revealChosen 表示"本帧需要把选中行滚动到可见区域"。只在选中项
	// 发生变化的那一帧置位，否则会持续把用户手动滚走的行拽回来。
	revealChosen bool

	// reloadRequested 由界面上的"重新加载"按钮置位，下一帧真正执行扫描。
	reloadRequested bool

	// hovered/hoverValid 记录指针当前所在的行。悬停只在"进入另一行"时
	// 才改变选择——指针静止时永远命中同一行，因此绝不会把键盘刚做出的
	// 选择改回去（这是 ui_test.go 锁定的回归）。
	hovered    int
	hoverValid bool

	// pointerX/pointerY 是上一帧指针在列表容器内的位置，pointerSeen 表示
	// 指针在列表里。只有位置真的变化才算"移动"，滚动不会伪造移动。
	pointerX, pointerY float32
	pointerSeen        bool

	// introFrames 数窗口显示后已经画过的帧数，只用于给出一次入场动画的
	// 起始位置（见 view）。
	introFrames int
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

// ============================================================================
// Chrome Profile 发现
// ============================================================================

// chromePaths 返回 Local State 与 Chrome 用户数据根目录。
func chromePaths() (statePath string, chromeBase string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("无法定位用户主目录: %w", err)
	}
	chromeBase = filepath.Join(home, "Library/Application Support/Google/Chrome")
	return filepath.Join(chromeBase, "Local State"), chromeBase, nil
}

// loadChromeProfiles 读取 Chrome 的 Local State 并解析出可用 Profile。
//
// 关键约定：绝不返回虚构的占位 Profile。调用方必须通过 error 区分
//   - 文件不存在（Chrome 未安装 / 从未登录）
//   - JSON 解析失败（文件损坏或结构变更）
//   - 解析成功但没有任何有效 Profile（条目为空，或磁盘上目录已不存在）
//
// 这样 UI 才能如实展示空状态，而不是假装成功。
func loadChromeProfiles() ([]Profile, error) {
	statePath, chromeBase, err := chromePaths()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("未找到 Chrome 配置文件：%s", statePath)
		}
		return nil, fmt.Errorf("读取 Chrome 配置失败: %w", err)
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("Chrome 配置文件格式无法解析: %w", err)
	}

	if len(state.Profile.InfoCache) == 0 {
		return nil, errors.New("Chrome 配置中没有任何 Profile 条目")
	}

	list := make([]Profile, 0, len(state.Profile.InfoCache))
	colorIdx := 0
	// 固定遍历顺序，保证配色分配在不同次加载之间保持稳定。
	for _, dir := range sortedDirs(state.Profile.InfoCache) {
		info := state.Profile.InfoCache[dir]

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

		list = append(list, Profile{
			Dir:    dir,
			Name:   name,
			Email:  email,
			Avatar: avatarBitmap,
			Color:  modernGradients[colorIdx%len(modernGradients)],
		})
		colorIdx++
	}

	if len(list) == 0 {
		return nil, errors.New("检测到的 Profile 条目在磁盘上均不存在")
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

	return list, nil
}

// sortedDirs 让 InfoCache 的 map 遍历顺序变得确定。
func sortedDirs(cache map[string]struct {
	Name                string `json:"name"`
	GaiaName            string `json:"gaia_name"`
	UserName            string `json:"user_name"`
	GaiaPictureFileName string `json:"gaia_picture_file_name"`
}) []string {
	dirs := make([]string, 0, len(cache))
	for dir := range cache {
		dirs = append(dirs, dir)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i] == "Default" {
			return true
		}
		if dirs[j] == "Default" {
			return false
		}
		return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j])
	})
	return dirs
}

// reload 重新扫描磁盘上的 Chrome Profile，并如实记录失败原因。
//
// 主线程调用。扫描与状态写入都在锁外/锁内明确分段：持锁时只改内存字段，
// 释放锁之后才决定窗口尺寸——MyGo 的窗口调用可能回到主线程并触发重绘，
// 绝不能在持有 a.mu 时调用。
func (a *selectorApp) reload() {
	list, err := loadChromeProfiles()

	a.mu.Lock()
	if err != nil {
		a.profiles = nil
		a.loadErr = err.Error()
		a.chosen = 0
	} else {
		a.profiles = list
		a.loadErr = ""
		if a.chosen >= len(list) {
			a.chosen = len(list) - 1
		}
		if a.chosen < 0 {
			a.chosen = 0
		}
		a.revealChosen = true
	}
	count := len(a.profiles)
	a.mu.Unlock()

	// 只有身份数量真的变化才重排窗口：选择、悬停、启动状态与错误提示
	// 都不会改变列数，也不应该让窗口跳动。
	a.requestLayout(count)
}

// ============================================================================
// 请求队列
// ============================================================================

// enqueue 是 URL 与文件的统一入队通道，可从任意 goroutine 调用。
func (a *selectorApp) enqueue(r request) {
	if r.Value == "" {
		return
	}

	a.mu.Lock()
	a.nextID++
	r.ID = a.nextID
	a.pending = append(a.pending, r)
	a.launchError = ""
	a.mu.Unlock()

	// 新请求到达时清掉上一轮的失败提示，并把窗口拉回前台。
	if w := a.windowRef(); w != nil {
		w.Update(func() {})
		w.Show()
	}
}

func (a *selectorApp) windowRef() *mygo.Window {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.window
}

// current 返回队首请求但不移除它。只有启动成功才出队——失败时请求
// 仍然保留，用户重试时不会丢失这条链接。
func (a *selectorApp) current() (request, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentLocked()
}

// currentLocked 是 current 的加锁版内核：调用方必须已持有 a.mu。
//
// 这样拆分是必须的——sync.Mutex 不可重入。openWith 等已经持锁的函数
// 必须走这个 *_Locked 变体，绝不能再调用会二次 Lock 的公开方法，否则
// UI 主线程会当场永久死锁。
func (a *selectorApp) currentLocked() (request, bool) {
	if len(a.pending) == 0 {
		return request{}, false
	}
	return a.pending[0], true
}

func (a *selectorApp) pendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

// hasPendingRequests 报告队列里是否还有待处理的外部请求。
//
// 界面必须靠它判断"是否收到了外部链接"，绝不能拿展示用 URL 与 HomeURL 做
// 字符串比较：当外部应用点击的正好就是 HomeURL 时，两者相等，界面会误判成
// "未收到外部请求"，把真实存在的外部请求说成默认打开主页。
func (a *selectorApp) hasPendingRequests() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending) > 0
}

// ============================================================================
// 本地文件 → file:// URL
// ============================================================================

// fileTarget 把本地路径转换成规范转义后的 file:// URL。
//
// 直接拼接 "file://" + path 会在路径包含 '#'、'?'、空格等字符时被 Chrome
// 当成 fragment / query 而截断。这里先取绝对路径，再交给 net/url 做
// 百分号编码，保证整段路径都被当作 Path 传递。
func fileTarget(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("空的文件路径")
	}

	// 已经是被规范化过的 file URL 时直接返回，避免二次编码。
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("无法解析 file URL: %w", err)
		}
		return u.String(), nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("无法解析绝对路径: %w", err)
	}

	// Path 字段让 url 包负责转义：空格 → %20，'#' → %23，'?' → %3F。
	u := url.URL{Scheme: "file", Path: abs}
	return u.String(), nil
}

// ============================================================================
// 目标 URL
// ============================================================================

// target 解析当前要打开的地址：优先队首请求，队列为空时退回家页。
func (a *selectorApp) target() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.targetLocked()
}

// targetLocked 是 target 的加锁版内核：调用方必须已持有 a.mu。
// 它只读取 a.pending 队首，不会二次加锁。
func (a *selectorApp) targetLocked() (string, error) {
	r, ok := a.currentLocked()
	if !ok {
		return HomeURL, nil
	}
	if r.Kind == requestFile {
		return fileTarget(r.Value)
	}
	return r.Value, nil
}

// parseURLDetails 把即将打开的地址拆成 Host / Path 供界面展示。
//
// 关键：空输入必须与真实启动行为一致——openWith 在没有外部请求时启动的是
// HomeURL，所以这里也回落到 HomeURL 的解析结果，绝不显示一个硬编码的
// 假地址，否则用户看到的目标与即将打开的页面不符。
func parseURLDetails(raw string) (host string, pathStr string, fullStr string) {
	if strings.TrimSpace(raw) == "" {
		raw = HomeURL
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

// truncateRunes 按 rune 截断，避免切坏 UTF-8 多字节字符。
func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

// ============================================================================
// Chrome 启动
// ============================================================================

// openWith 在后台 goroutine 中启动 Chrome，绝不阻塞 UI 线程。
//
// 成功且没有错误时：把本次消费的请求 ID 交给队列推进逻辑，由 Update 决定
// 是打开下一条还是退出。失败时：记录 launchError 并让界面重绘，用户可原地
// 重试。
//
// 锁纪律：整个函数在同一把 a.mu 下只能加锁一次。目标解析必须走
// targetLocked()（非重入内核），绝不能调用会二次 Lock 的 target() / current()，
// 否则 sync.Mutex 不可重入会导致 UI 主线程永久死锁。
func (a *selectorApp) openWith(p Profile) {
	a.mu.Lock()
	if a.launching {
		a.mu.Unlock()
		return
	}
	target, err := a.targetLocked()
	if err != nil {
		a.launchError = err.Error()
		a.mu.Unlock()
		if w := a.windowRef(); w != nil {
			w.Invalidate()
		}
		return
	}

	// 记录本次启动实际消费的请求 ID（0 表示并无队列请求，走 HomeURL）。
	// 出队必须以 ID 为准，不能用"删队首"这种位置假设。
	var launchedID uint64
	if head, ok := a.currentLocked(); ok {
		launchedID = head.ID
	}

	a.launching = true
	a.launchError = ""
	a.mu.Unlock()

	w := a.windowRef()
	if w != nil {
		w.Invalidate()
	}

	go func() {
		err := launchChrome(p.Dir, target)

		if w == nil {
			// 窗口尚未就绪（极早期触发），直接同步收尾。
			if err != nil {
				a.finishLaunch(err)
				return
			}
			a.advanceQueue(launchedID)
			return
		}

		// 所有状态写回都在 main thread 上完成，避免数据竞争。
		w.Update(func() {
			a.finishLaunch(err)
			if err == nil {
				a.advanceQueue(launchedID)
			}
		})
	}()
}

// launchChrome 使用带超时的 Context 调用 /usr/bin/open 拉起 Chrome。
func launchChrome(profileDir, target string) error {
	ctx, cancel := context.WithTimeout(context.Background(), launchTimeout)
	defer cancel()

	args := []string{
		"-n",                      // 新实例，允许同一 Profile 多开窗口
		"-b", "com.google.Chrome", // 按 bundle id 定位，避免中文环境下的名称匹配问题
		"--args", // 之后的参数透传给 Chrome
		"--profile-directory=" + profileDir,
		target,
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/open", args...)
	out, err := cmd.CombinedOutput()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("启动 Chrome 超时（%s）", launchTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("启动 Chrome 失败: %w", err)
		}
		return fmt.Errorf("启动 Chrome 失败: %s", msg)
	}
	return nil
}

// finishLaunch 收起 loading 状态；err 非空时保留为界面可读的错误信息。
func (a *selectorApp) finishLaunch(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.launching = false
	if err != nil {
		a.launchError = err.Error()
	}
}

// advanceQueue 在成功打开一条请求后按 ID 出队；队列空则退出，否则继续处理。
// 必须在 main thread 上调用。
//
// launchedID 是本次启动真正消费的请求 ID。出队前必须确认队首仍然是它：
//   - launchedID == 0：本次是 HomeURL（无队列请求）启动，绝不触碰队列，
//     否则会把 Chrome 启动那几秒里新进来的外部链接删掉。
//   - 队首 ID 已不是 launchedID：说明该请求已被其它路径消费，同样不动队列。
func (a *selectorApp) advanceQueue(launchedID uint64) {
	a.mu.Lock()
	if launchedID != 0 && len(a.pending) > 0 && a.pending[0].ID == launchedID {
		a.pending = a.pending[1:]
	}
	// 只有队列真的空了才能退出：HomeURL 启动时队列里可能已有新请求，
	// 直接 Quit 会把这些链接连同窗口一起吞掉。
	remaining := len(a.pending)
	a.mu.Unlock()

	if remaining == 0 {
		// 队列全部处理完毕：自然结束进程。这里不做任何拦截复核，退出就是
		// 退出——保留一个"退出前再检查一次"的钩子只会让状态机重新变复杂。
		a.requestQuit()
		return
	}

	// 还有链接在排队：窗口留在前台并更新界面，等待用户选择下一个身份。
	if w := a.windowRef(); w != nil {
		w.Update(func() {})
		w.Show()
	}
}

// requestQuit 是发起退出的统一入口：无条件、直接退出。
//
// 应用自身在队列清空后调用它，用户按 Esc 也调用它；⌘Q、菜单栏 Quit、
// Dock 退出、系统关机 / 注销则由 macOS 原生投递到同一条退出管道。所有
// 路径共享同一个语义——退出就是退出，没有任何拦截器、布尔标志或竞态可
// 能把用户意图吞掉。
func (a *selectorApp) requestQuit() {
	mygo.App.Quit()
}

// ============================================================================
// 窗口布局
// ============================================================================

// profileCount 返回当前身份数量；只在持锁时读切片长度。
func (a *selectorApp) profileCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.profiles)
}

// cursorWorkArea 返回光标所在显示器的工作区，用于窗口首次弹出。
func cursorWorkArea() mygo.Rectangle {
	return mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()).WorkArea
}

// windowWorkArea 返回窗口当前主要落在的显示器工作区。窗口被拖到另一块
// 屏幕或被拔掉屏幕后，这里给出的是它现在真正所属的可用区域。
func windowWorkArea(w *mygo.Window) mygo.Rectangle {
	return mygo.Screen.DisplayMatching(w.Bounds()).WorkArea
}

// applyLayout 按身份数量与工作区写入窗口的内容区尺寸和位置，必须在主线程
// 调用，且不能在渲染帧中途调用（见 requestLayout）。
//
// recenter 只在首次弹出时为真：那一次把面板居中在参考屏幕上；之后一律
// 保留用户拖到的位置，仅把它夹回工作区内，避免重算尺寸时窗口无端跳回
// 屏幕中央。
func (a *selectorApp) applyLayout(w *mygo.Window, count int, workArea mygo.Rectangle, recenter bool) {
	width, height := profilePanelSize(count, workArea)

	// 先记住当前左上角：改尺寸时系统可能把外框原点挪动（macOS 以左下角
	// 为锚点），重排后必须按用户原来的位置写回，而不是接受漂移。
	b := w.Bounds()
	x, y := b.X, b.Y

	// 尺寸用内容区表达（布局预算是按界面内容算的），位置用窗口外框表达
	// （工作区是屏幕坐标）。
	if curW, curH := w.ContentSize(); curW != width || curH != height {
		w.SetContentSize(width, height)
		b = w.Bounds()
	}

	if recenter || !a.placed {
		x, y = centeredOrigin(workArea, b.Width, b.Height)
	}
	// 外框可能比内容区略高（系统标题栏高度），居中后再夹一次，保证整块
	// 面板始终完整落在工作区内。用户自己拖到的位置只做夹取，不重排。
	if tx, ty := clampOrigin(workArea, x, y, b.Width, b.Height); tx != b.X || ty != b.Y {
		w.SetPosition(tx, ty)
	}

	a.placed = true
	a.layoutCount = count
}

// requestLayout 安排一次窗口重排，身份数量没变时什么也不做。
//
// 真正的原生写入走 Window.Update，投递到下一轮主线程循环：reload 可能
// 正在渲染帧内被"重新加载"按钮触发，此刻直接改窗口尺寸会让内容宿主在
// 渲染途中收到 resize 回调。投递既避开重入，也不需要任何锁——闭包内的
// 状态只在主线程读写。
func (a *selectorApp) requestLayout(count int) {
	// 还没排过版：启动路径会用 showWithLayout 在显示前一次性定位。
	if !a.placed || a.layoutCount == count {
		return
	}
	w := a.windowRef()
	if w == nil {
		return
	}
	w.Update(func() {
		// 投递期间可能已经有别的路径排过版。
		if !a.placed || a.layoutCount == count {
			return
		}
		a.applyLayout(w, count, windowWorkArea(w), false)
	})
}

// showWithLayout 在显示前完成首次尺寸与居中，避免面板先出现在旧位置再
// 跳到正确大小。启动路径专用：此时还没有任何渲染帧，可以同步写入。
func (a *selectorApp) showWithLayout(w *mygo.Window) {
	a.applyLayout(w, a.profileCount(), cursorWorkArea(), true)
	w.Show()
}

// handleDisplaysChanged 在显示器增删或分辨率变化后，按窗口当前所在屏幕
// 重新限制尺寸与位置。窗口可能已经被移到不存在的坐标上，因此这里总是
// 夹回工作区，但仍然保留用户的相对位置。
//
// 回调可能来自任意线程，所以只做投递，真正的原生写入留在主线程。
func (a *selectorApp) handleDisplaysChanged() {
	w := a.windowRef()
	if w == nil {
		return
	}
	w.Update(func() {
		if a.placed {
			a.relayout(w)
		}
	})
}

// relayout 立即按窗口当前所在屏幕重排一次，主线程调用。
//
// 与 requestLayout 的区别只在时机：它不做"身份数量没变就跳过"的判断，
// 因为显示器本身变了，即使列数相同也要重新夹回新的工作区。
func (a *selectorApp) relayout(w *mygo.Window) {
	a.applyLayout(w, a.profileCount(), windowWorkArea(w), false)
}

// ============================================================================
// 入口
// ============================================================================

func main() {
	a := &selectorApp{
		chosen:    0,
		themeMode: ThemeAuto,
	}

	// 启动参数（URL / 文件路径）不在这里手动入队：MyGo 在 WhenReady 之后会
	// 通过 launchArgs() → deliverArgs() 把 os.Args[1:] 中属于 urlSchemes 与
	// fileAssociations 的项分发给下面这两个监听器。手动再遍历一遍 os.Args
	// 会让同一条链接入队两次——第一次打开后队列里仍残留一份，弹窗再次弹出。
	mygo.App.OnOpenURL(func(u string) {
		a.enqueue(request{Kind: requestURL, Value: u})
	})

	mygo.App.OnOpenFile(func(f string) {
		a.enqueue(request{Kind: requestFile, Value: f})
	})

	// 退出没有任何拦截器：用户意图（Esc / ⌘Q / 菜单 Quit / Dock 退出 /
	// 系统关机注销）与应用自身队列清空后的退出，全部无条件直接生效。
	mygo.App.WhenReady(func() {
		// 屏幕查询只在 WhenReady 之后可用，所以首次尺寸也在这里算：
		// 先按空状态拿到一个保守的起始尺寸，避免窗口以默认 800×600 出现
		// 在用户眼前；真实尺寸由 showWithLayout 在显示前写入。
		area := cursorWorkArea()
		startW, startH := profilePanelSize(0, area)

		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "Chrome Profile Router",
			// 尺寸由 profilePanelSize 依据身份数量与工作区决定：1–4 个
			// 身份是 460 宽的单列卡片，5 个起是 700 宽的三列画廊（窄屏
			// 自动降到 2 或 1 列）。窗口仍不可拖动改变大小，但不设固定
			// 最小尺寸——否则窄屏上无法把面板收进工作区。
			Width:             startW,
			Height:            startH,
			UseContentSize:    true,
			Hidden:            true,
			DisableResize:     true,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			AlwaysOnTop:       true,
			// Panel 让它像 Spotlight / Raycast 一样浮在当前 Space（含全屏
			// 应用之上），不进入 Dock 与 App 切换器，并且在隐藏时把键盘
			// 交还给用户原先所在的 App。这正是链接路由弹窗需要的语义。
			Panel:    true,
			Vibrancy: mygo.VibrancyPopover,
			Content:  ui.View(a.view),
		})

		a.mu.Lock()
		a.window = w
		a.mu.Unlock()

		// 显示器增删或分辨率变化后，按窗口当前所在屏幕重新限制尺寸与
		// 位置。回调可能来自任意线程，因此只做投递，真正的原生写入留在
		// 主线程。
		mygo.Screen.OnDisplaysChanged(a.handleDisplaysChanged)

		// 无障碍偏好只在应用运行后可读，因此在窗口就绪后读取一次，并在
		// 主题设置变化时刷新；界面帧不再直接访问 mygo.Theme。
		a.setReduceTransparency(mygo.Theme.Preferences().ReduceTransparency)
		mygo.Theme.OnUpdated(func() {
			a.setReduceTransparency(mygo.Theme.Preferences().ReduceTransparency)
			if w := a.windowRef(); w != nil {
				w.Update(func() {})
			}
		})

		// 窗口就绪后再扫描 Profile，保证空状态 / 重新加载能安全重绘。
		// 扫描在显示之前完成，首次出现的面板就已经是最终尺寸。
		a.reload()

		a.showWithLayout(w)
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
