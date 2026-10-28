//go:build !darwin

// [INPUT]: 无外部依赖，只把非 macOS 平台的修饰键名固化。
// [OUTPUT]: 提供 shortcutLabel：把键名渲染成 Ctrl+ 组合。
// [POS]: Linux/Windows 的键盘文案事实；快捷键绑定本身来自 ui.Cmd（非 macOS 是 Ctrl），与文案无关。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

// shortcutLabel 返回非 macOS 平台的快捷键文案，例如 shortcutLabel("C") == "Ctrl+C"。
func shortcutLabel(key string) string { return "Ctrl+" + key }
