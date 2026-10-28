// [INPUT]: 无外部依赖，只把 macOS 的修饰键字形固化。
// [OUTPUT]: 提供 shortcutLabel：把键名渲染成 macOS 的 ⌘ 组合。
// [POS]: macOS 的键盘文案事实；快捷键绑定本身来自 ui.Cmd，与文案无关。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

// shortcutLabel 返回 macOS 上的快捷键文案，例如 shortcutLabel("C") == "⌘C"。
func shortcutLabel(key string) string { return "⌘" + key }
