# Chrome Profile Router - macOS 临时浏览器身份选择菜单
Go 1.27.3 + MyGo v0.6.4 + macOS 原生 Vibrancy

<directory>
resources/ - 应用图标源与打包资产
.github/workflows/ - macOS 校验与双架构打包
</directory>

成员清单
main.go: Profile 发现、请求队列、Chrome 启动与应用生命周期；只有成功启动的请求 ID 可以出队，失败保留原请求。
view.go: 临时菜单视图与用户输入，关闭、选择、主题及错误反馈由同一入口组织。
view_style.go: 视图颜色与轻量动效，按系统主题和辅助功能偏好调整显示。
ui_test.go: 混合鼠标与键盘输入的真实视图回归，防止静止悬停覆盖键盘选择。
routing_test.go: 请求队列、失败重试与文件 URL 编码的行为边界。
build.sh: 根据本机架构构建，在目标卷暂存验签后替换应用，失败时恢复旧包。
README.md: 用户操作、构建安装与验证说明。
go.mod / go.sum: Go 工具链及固定依赖版本。
mygo.json: Bundle 标识、URL scheme、文档类型和版本的唯一声明来源。

约束
- `mygo.App.OnOpenURL` / `OnOpenFile` 统一入队；不再手动遍历启动参数，避免重复请求。
- `openWith` 持锁时只调用 `targetLocked` / `currentLocked`；Chrome 启动在后台运行，结果经 Window.Update 回到主线程。
- Esc、关闭按钮、系统退出不受队列或动效拦截；装饰性动效不能延迟键盘选择与链接交付。
- 原生材料负责背景模糊，文本保持不透明；不得通过全窗口 Opacity 降低可读性。
- 图标修改同步 SVG 与 PNG，MyGo 自动生成 ICNS；不得在安装后另行修改已签名资源。

验证
`go test ./...`、`go vet ./...`、`bash -n build.sh`；打包使用 `go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -platform darwin/arm64,darwin/amd64`。`./build.sh` 会替换 `/Applications` 中的应用，仅在需要本机安装时执行。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
