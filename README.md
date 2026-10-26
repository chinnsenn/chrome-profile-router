# Chrome Profile Router

一个用 Go 编写的轻量 macOS 默认浏览器分流工具，用于在外部打开链接时选择使用哪个 Google Chrome 账号 Profile。

---

## 解决的问题

在日常工作中，本地 Chrome 通常会同时登录多个账号（如个人、公司主账号、海外账号、测试环境等）。

当你在微信、飞书、Slack、邮件客户端或终端里点击一个网页链接时，macOS 默认只能把链接交给系统默认浏览器打开；而 Chrome 默认只会将链接丢给**最后活跃的那个窗口**，经常导致：
- 用私人账号点开了公司内网或权限系统，提示权限不足；
- 用工作账号点开了私人社交或娱乐链接，账号环境混乱。

该工具的作用是在链接到达 Chrome 之前作为中间层拦截，弹出一个轻量选择器，由用户决定使用哪个 Profile 进行访问。

---

## 实现方案

1. **注册为系统默认浏览器**：
   在 App 的 `Info.plist` 中声明 `CFBundleURLTypes`（http/https）与 `CFBundleDocumentTypes`（public.html），使其符合 macOS 系统设置中「默认网页浏览器」的注册门槛。
2. **动态读取本地 Profile 配置**：
   启动时直接解析 Chrome 本地的 `~/Library/Application Support/Google/Chrome/Local State`，动态获取所有已创建 Profile 的真实名称、绑定邮箱及物理目录，并直接读取本地缓存的头像，无需手动配置清单。
3. **命令行唤起与即用即走**：
   用户选择对应身份后，工具通过系统命令行直接调起 Chrome 对应的 Profile 窗口打开目标链接，随后立即退出自身，不常驻后台占用系统资源：
   ```bash
   open -n -b com.google.Chrome --args --profile-directory="<ProfileDir>" "<TargetURL>"
   ```
4. **轻量浮层与快速操作**：
   使用 480×440 的原生磨砂面板展示目标域名与账号列表；左上角关闭按钮、`Esc` 或 `⌘Q` 可随时退出。数字键 `1`–`9` 直接打开，上下键选择相邻账号，左右键循环选择，回车确认；超过可见范围时自动滚动到选中项，静止的鼠标不会覆盖键盘选择。
5. **外观与反馈**：
   支持系统、浅色、深色三种外观。背景轻度透明，文字保持不透明；短促淡入、颜色过渡和按压弹簧不阻塞操作，跟随系统减少动态效果，高对比模式使用实色背景。新图标以蓝色分流路径与青绿色身份入口表示链接分流，`resources/icon.svg` 为源，`icon.png` 由 MyGo 自动转换为 ICNS。

---

## 构建与使用

### 环境要求

- macOS 12.0+
- [Go](https://go.dev/) 1.27.3+

### 编译与安装

```bash
git clone https://github.com/chinnsenn/chrome-profile-router.git
cd chrome-profile-router

# 一键编译并安装至 /Applications/Chrome Profile Router.app
./build.sh
```

脚本自动选择 Apple Silicon 或 Intel 的本机架构，先在 `/Applications` 内暂存并校验新应用的签名，再替换现有版本；复制、验签或注册失败时保留或恢复旧版本。构建产物位于 `build/darwin-arm64/` 或 `build/darwin-amd64/`。

仅构建、不安装：

```bash
go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -platform darwin/arm64,darwin/amd64
```

本地校验使用 `go test ./...`、`go vet ./...` 和 `bash -n build.sh`；macOS CI 执行这些检查并打包两种架构。

### 设为默认浏览器

1. 打开 macOS **「系统设置」** → **「桌面与程序坞」**；
2. 找到 **「默认网页浏览器」** 下拉菜单；
3. 选择 **「Chrome Profile Router」**。

---

## License

MIT
