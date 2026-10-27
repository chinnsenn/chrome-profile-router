# Chrome Profile Router - macOS 临时浏览器身份选择菜单
Go 1.27.3 + MyGo v0.6.4 + macOS 原生 Vibrancy

<directory>
resources/ - 应用图标源与打包资产
.github/workflows/ - macOS 校验与双架构打包
</directory>

成员清单
main.go: Profile 发现、请求队列、Chrome 启动与应用生命周期；按屏幕工作区调整窗口，成功启动后才将对应请求 ID 出队。
layout.go: 单列与三列画廊共享的列数、卡片高度、头像/徽标尺寸和窗口尺寸预算；固定控件高度推导窗口尺寸，不预留未显示状态的空白。
view.go: 整幅宽度的目标地址卡加身份画廊与用户输入，状态在地址卡次行内反馈；手动主题同步原生磨砂外观。
profile_view.go: 单列与三列共用身份卡片（身份色细条、32pt 真实头像、名称/邮箱两行、数字键徽标），提供悬停反馈、无障碍描述与空格详情浮层。
view_style.go: 原生磨砂上的地址卡与卡片薄层、细边界、卡片投影与轻量反馈，保持文字对比；高对比或降低透明度时改用不透明表面并去掉投影。
ui_test.go: 混合输入、九身份同屏、末行边界和窄屏导航回归，防止静止悬停覆盖键盘选择。
routing_test.go: 请求队列、失败重试与文件 URL 编码的行为边界。
build.sh: 根据本机架构构建，在目标卷暂存验签后替换应用，失败时恢复旧包。
README.md: 用户操作、构建安装与验证说明。
go.mod / go.sum: Go 工具链及固定依赖版本。
mygo.json: Bundle 标识、URL scheme、文档类型和版本的唯一声明来源。

约束
- `mygo.App.OnOpenURL` / `OnOpenFile` 统一入队；不再手动遍历启动参数，避免重复请求。
- `openWith` 持锁时只调用 `targetLocked` / `currentLocked`；Chrome 启动在后台运行，结果经 Window.Update 回到主线程。
- Esc、关闭按钮、系统退出不受队列或动效拦截；装饰性动效不能延迟键盘选择与链接交付。
- 原生材料负责背景模糊，文本保持不透明；不得通过全窗口 Opacity 降低可读性。系统开启「降低透明度」或高对比时，界面改用实色表面并去掉投影。
- 1–4 个身份使用单列卡片，更多身份按可用宽度排成最多三列画廊；列数、卡片高度与头像尺寸只来自 layout.go，状态提示不能改变窗口尺寸或身份顺序。
- 只有地址卡与详情浮层是带投影的表面，身份卡常态近透明；不得把每张卡片做成独立玻璃，也不得用投影改变布局尺寸。
- URL 域名独占地址卡主行、路径独占次行；卡片上的 1–9 数字键徽标是身份标识，不另设快捷键说明文字；错误或启动状态替换次行，不能预留底部空白。
- 图标修改同步 SVG 与 PNG，MyGo 自动生成 ICNS；不得在安装后另行修改已签名资源。

验证
`go test ./...`、`go vet ./...`、`bash -n build.sh`；打包使用 `go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -platform darwin/arm64,darwin/amd64`。`./build.sh` 会替换 `/Applications` 中的应用，仅在需要本机安装时执行。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
