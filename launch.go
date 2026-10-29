// [INPUT]: 依赖 launch_<goos>.go 提供的 launchChrome、Chrome 的 Local State 之外的运行状态，以及 mygo.Window 的 Update/Show。
// [OUTPUT]: 在后台交付请求：成功才按请求 ID 出队，失败保留并回显原因；提供跨平台一致的「启动后观察窗口」语义，以及本次启动的完成信号（唯一 Wait 所有者的出口）。
// [POS]: 根包的启动与交付层；平台差异在 launch_darwin.go / launch_linux.go / launch_windows.go。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"fmt"
	"os/exec"
	"time"
)

// launchTimeout 限制 macOS open 路径的最长等待时间，避免 open 卡死时把
// 整个 UI 拖进永久 loading 状态。
const launchTimeout = 8 * time.Second

// launchGrace 是直接启动 Chrome 后观察「立即失败」的窗口。
//
// 直接启动的进程有两种已知形态：交给已在运行的 Chrome 后很快退出，或者自己
// 一直活到浏览器关闭。所以不能等它退出（那会把每次启动变成无限等待），也
// 不能完全不看：窗口内以非零码退出说明这次交付失败了，必须留在队列里可重试。
//
// 语义边界：窗口内没有退出只说明「启动动作没有报错」，**不证明**页面已经
// 导航完成；窗口内以 0 退出只说明进程正常结束，同样不证明 Chrome 接受了这
// 条链接。要更强的保证只能读 Chrome 的调试端口或进程状态，那超出本工具的
// 职责。700ms 是观察窗长度，跑在后台 goroutine 上，不阻塞渲染；真实 Chrome
// 的退出时序记录在 docs/platforms.md 的 smoke 结果里。
const launchGrace = 700 * time.Millisecond

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
		a.showPanel()
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
			// 窗口尚未就绪（极早期触发），直接同步收尾。收尾顺序必须与
			// 窗口路径一致：先收起 loading，再推进队列——否则成功交付后
			// launching 会一直为真，面板永远停在「正在启动 Chrome…」。
			a.finishLaunch(err)
			if err == nil {
				a.advanceQueue(launchedID)
			}
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

// chromeArgs 返回交给 Chrome 的参数：先选 Profile 目录，再给目标地址。
//
// 两个元素各自独立：Profile 目录可能带空格与中文（"Profile 1"、"工作"），
// 目标地址可能带查询串与片段。exec 负责按平台的命令行规则加引号，所以这里
// 绝不自己拼接一个字符串。
func chromeArgs(profileDir, target string) []string {
	return []string{"--profile-directory=" + profileDir, target}
}

// runDetached 启动 cmd 并给出「这次交付是否失败」的结论，绝不等待浏览器
// 进程的生命周期。
//
//   - Start 失败：可执行文件不存在或不可执行，直接报错（没有完成信号）。
//   - 在 grace 内以非零码退出：交付失败，请求必须留在队列里。
//   - 在 grace 内以 0 退出：启动动作没有报错。
//   - grace 内没有退出：启动动作没有报错；进程的回收交给后台 Wait，应用退出
//     时不会被一起带走（见 launch_linux.go 的 detach）。
//
// 后两种都只表示"没有观察到失败"，不证明页面已经导航完成。
//
// done 是这次启动的完成信号，也是进程回收的唯一出口：下面的后台 goroutine 是
// cmd.Wait 的唯一调用者，它把 Wait 的结果放进 done 并关闭它。生产路径不读
// done（不等待浏览器生命周期）；只有需要确认「子进程已经结束」的调用方读它，
// 读到即已被回收。进程在窗口内就退出时结果已经由返回的 error 给出，done 最终
// 关闭——所以 done 只允许一个读者，且绝不能再有第二个 Wait。
func runDetached(cmd *exec.Cmd, grace time.Duration) (<-chan error, error) {
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 Chrome 失败: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		close(done)
	}()

	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return done, fmt.Errorf("Chrome 启动后立即退出（%v）", err)
		}
		return done, nil
	case <-timer.C:
		return done, nil
	}
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
