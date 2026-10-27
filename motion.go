// [INPUT]: 依赖 selectorApp 的请求队列、MyGo 内容动画及主线程投递。
// [OUTPUT]: 提供仅内容层的入场与可中断异步出场；新请求取消自然退出，显式关闭保留退出意图。
// [POS]: 根包的内容生命周期；不改变原生窗口透明度，不拦截系统退出。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type panelPhase uint8

const (
	panelReady panelPhase = iota
	panelPrepare
	panelEntering
	exitNatural
	exitClose
	panelStopped
)

type panelMotion struct {
	phase  panelPhase
	run    uint64
	first  time.Time
	queued bool
}

// resetEntry 在 Show 前调用；同一帧的所有布局 pass 都保留起始画面。
func (a *selectorApp) resetEntry() {
	a.mu.Lock()
	a.motion.run++
	a.motion.phase = panelPrepare
	a.motion.first = time.Time{}
	a.motion.queued = false
	a.mu.Unlock()
}

// cancelNaturalExitLocked 与入队共用锁，旧完成回调因此不能吞掉新请求。
func (a *selectorApp) cancelNaturalExitLocked() {
	if a.motion.phase == exitNatural {
		a.motion.phase = panelEntering
		a.motion.run++
		a.motion.queued = false
	}
}

// beginExit 只改变动画目标；不等待动画，不延后 Chrome 交付。
func (a *selectorApp) beginExit(reason panelPhase) {
	a.mu.Lock()
	phase := a.motion.phase
	if phase == panelStopped || reason == exitNatural && (len(a.pending) > 0 || phase == exitClose || phase == exitNatural) {
		a.mu.Unlock()
		return
	}
	if reason == exitClose && phase == exitClose {
		a.mu.Unlock()
		a.requestQuit()
		return
	}
	a.motion.phase = reason
	a.motion.run++
	a.motion.queued = false
	w := a.window
	a.mu.Unlock()
	if w != nil {
		w.Invalidate()
	}
}

// requestQuit 是立即退出通路，供 Esc、⌘Q 与重复关闭使用。
func (a *selectorApp) requestQuit() {
	a.mu.Lock()
	if a.motion.phase == panelStopped {
		a.mu.Unlock()
		return
	}
	a.motion.phase = panelStopped
	a.motion.run++
	quit := a.quit
	a.mu.Unlock()
	if quit != nil {
		quit()
	} else {
		mygo.App.Quit()
	}
}

func (a *selectorApp) animatePanel(c *ui.Context, panel ui.Element) {
	a.mu.Lock()
	if a.motion.phase == panelPrepare {
		if c.Preferences().ReduceMotion {
			a.motion.phase = panelReady
		} else if a.motion.first.IsZero() {
			a.motion.first = c.Now()
		} else if c.Now().After(a.motion.first) {
			a.motion.phase = panelEntering
		}
	}
	phase, run := a.motion.phase, a.motion.run
	a.mu.Unlock()

	if phase == panelPrepare {
		panel.Opacity(panel.Jump("panelFade", 0))
		scale := panel.Jump("panelScale", 0.97)
		y := panel.Jump("panelY", 6)
		panel.Transform(ui.Scaling(scale, scale).Then(ui.Translation(0, y)))
		c.AnimationFrame()
		return
	}

	scale, y, fade := float32(1), float32(0), float32(1)
	animation := springEntry()
	exiting := phase == exitNatural || phase == exitClose
	if exiting {
		scale, y, fade = 0.98, 4, 0
		animation = springExit()
	}
	scale = panel.AnimateTo("panelScale", scale, animation)
	y = panel.AnimateTo("panelY", y, animation)
	fade = panel.AnimateTo("panelFade", fade, animation)
	panel.Transform(ui.Scaling(scale, scale).Then(ui.Translation(0, y))).Opacity(fade)
	if exiting && panel.Settled("panelScale") && panel.Settled("panelY") && panel.Settled("panelFade") {
		a.queueExitCompletion(run)
	}
}

// 完成依据真实动画通道；投递让最后一帧结束后再销毁原生窗口。
func (a *selectorApp) queueExitCompletion(run uint64) {
	a.mu.Lock()
	if a.motion.run != run || a.motion.queued {
		a.mu.Unlock()
		return
	}
	a.motion.queued = true
	w := a.window
	a.mu.Unlock()
	if w != nil {
		w.Update(func() {
			if !w.IsDestroyed() {
				a.completeExit(run)
			}
		})
	} else {
		a.completeExit(run)
	}
}

func (a *selectorApp) completeExit(run uint64) {
	a.mu.Lock()
	phase := a.motion.phase
	if a.motion.run != run || phase != exitClose && phase != exitNatural {
		a.mu.Unlock()
		return
	}
	if phase == exitNatural && len(a.pending) > 0 {
		a.cancelNaturalExitLocked()
		a.mu.Unlock()
		return
	}
	a.motion.phase = panelStopped
	a.motion.run++
	quit := a.quit
	a.mu.Unlock()
	if quit != nil {
		quit()
	} else {
		mygo.App.Quit()
	}
}
