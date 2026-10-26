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
   open -n -a "Google Chrome" --args --profile-directory="<ProfileDir>" "<TargetURL>"
   ```
4. **快速操作**：
   弹窗展示目标 URL 域名，支持直接按数字键 `1`–`9` 快速以对应身份秒开，或使用方向键配合回车，按 `Esc` 退出。支持深色、浅色及跟随系统三段切换。

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

### 设为默认浏览器

1. 打开 macOS **「系统设置」** → **「桌面与程序坞」**；
2. 找到 **「默认网页浏览器」** 下拉菜单；
3. 选择 **「Chrome Profile Router」**。

---

## License

MIT
