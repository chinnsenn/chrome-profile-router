# 平台支持与验证

本文件记录 Chrome Profile Router 在 macOS / Linux / Windows 上**实际做过**的
事情与**还没做过**的事情。界面细节与视觉参数不在这里，见 `visual-design.md`
与源码常量。

## 支持矩阵

| 能力 | macOS | Linux | Windows |
| --- | --- | --- | --- |
| Chrome Profile 发现 | `~/Library/Application Support/Google/Chrome` | `$XDG_CONFIG_HOME/google-chrome`（为空或相对则用 `~/.config/google-chrome`） | `%LOCALAPPDATA%\Google\Chrome\User Data` |
| 拉起 Chrome | `/usr/bin/open -n -b com.google.Chrome` | PATH 上的 `google-chrome` / `google-chrome-stable` | `%LOCALAPPDATA%`、`%ProgramFiles%`、`%ProgramFiles(x86)%` 下的 `Google\Chrome\Application\chrome.exe`（按此顺序） |
| 默认浏览器注册 | 应用包声明 http/https（系统设置里可选） | 安装时写 `.desktop` 的 `x-scheme-handler/http;https` 与 `%U`，只成为候选 | 安装脚本写 per-user 的候选键，只成为候选 |
| 单实例 | 由 LaunchServices 保证，不再加锁 | 应用内单实例锁，后启动的实例把命令行交给第一个 | 同 Linux |
| 材质 | 原生磨砂（NSVisualEffectView） | 无材质，界面走实色表面 | Windows 11 有 Acrylic/Mica，其他版本走实色表面 |
| 快捷键文案 | `⌘C` / `⌘,` | `Ctrl+C` / `Ctrl+,` | 同 Linux |

范围只有 **Chrome Stable**。Chromium、Chrome Beta、Flatpak 与 Snap 的路径都
不解析。需要它们时是新增功能，不是回退。

验证状态：macOS 长期自用；Linux 已在隔离环境用真实 Chrome 端到端验证（详见文末
「已知限制与未验证项」）；Windows 只有交叉编译与 Wine 下的功能证据：原生 Windows 未跑，
arm64 只构建过、没运行过，Wine 里中文仍有缺字（最小 MyGo 基线同现）、第二个实例的转发也没通，
不要当成可用版本分发。

## 安装与验证

### Linux

```bash
tar -xzf chrome-profile-router-<版本>-linux-amd64.tar.gz
./install.sh ./chrome-profile-router-<版本>-linux-amd64.tar.gz
```

官方安装脚本要读压缩包本身，所以在安装完成前**保留下载的那个 tar.gz**（别删、也别把它移走），
并把压缩包路径作为参数传给 `install.sh`。脚本把应用装到 `~/.local`，把 `.desktop` 写进
`$XDG_DATA_HOME/applications`，然后跑 `update-desktop-database`。它**不**调用
`xdg-mime default` 或 `xdg-settings set default-web-browser`：装完只是候选，
默认浏览器仍由用户在系统设置里选。

依赖只有 GTK3：界面是原生 UI（`mygo.json` 里 `linux.webview: false`），
不会加载网页，所以 `.deb` 的依赖与安装脚本都不要求 WebKitGTK。

验证：

```bash
grep -E '^(Exec|MimeType)=' ~/.local/share/applications/*.desktop
xdg-mime query default x-scheme-handler/http   # 安装前后应当不变
```

### Windows

Windows 只分发 zip，里面只有三个文件：`Chrome Profile Router.exe`、`install.cmd`、
`uninstall.cmd`（没有额外资源与 manifest）。不包含
MyGo 生成的 `Setup.exe`：那个安装器会为 `urlSchemes` 写
`HKCU\Software\Classes\http`，等于把 http/https 抢成本应用，卸载时还会连带删
掉同名的真实类。zip 里的 `install.cmd` 双击即可 per-user 安装到
`%LOCALAPPDATA%\Programs\Chrome Profile Router`，只写候选键：

- `HKCU\Software\Clients\StartMenuInternet\ChromeProfileRouter`（含
  `Capabilities`：`ApplicationName`、`ApplicationDescription`、
  `ApplicationIcon`、`URLAssociations\http|https`、`FileAssociations\.htm|.html|.xhtml`、
  `Startmenu\StartMenuInternet`）
- `HKCU\Software\RegisteredApplications\ChromeProfileRouter`
- `HKCU\Software\Classes\ChromeProfileRouterHTML`（ProgID，含 `DefaultIcon` 与
  `shell\open\command`）
- `HKCU\Software\Classes\.htm|.html|.xhtml\OpenWithProgids\ChromeProfileRouterHTML`

安装脚本不写 `HKCU\Software\Classes\http|https`、不改 `.html` 的默认值、不碰
`UserChoice`。装完请在「设置 → 应用 → 默认应用」里手动把 http/https 选成本
应用。`uninstall.cmd` 只删上面这些命名空间内的键，保留 `settings.json`。

## 启动握手的语义

macOS 用 `/usr/bin/open`，它把请求交给 LaunchServices 后立即返回；超过 8 秒
没返回才算失败。

Linux 与 Windows 直接启动 Chrome 可执行文件，因此用一个观察窗（`launchGrace`，
700ms）判断这次交付是否失败：

- 进程在窗口内以非零码退出 → 交付失败，请求留在队列里，可以重试；
- 进程在窗口内以 0 退出 → 启动动作没有报错；
- 窗口结束时进程还在 → 启动动作没有报错，进程继续运行。

后两种都**只表示没有观察到失败**，不证明页面已经导航完成。不等待浏览器
进程的生命周期是刻意的：Chrome 没在运行时，那个进程就是浏览器本身，等它
退出等于等用户关掉浏览器。

Linux 上被启动的 Chrome 会进入独立会话（`setsid`），本应用交付完退出时不会
把它一起带走。

## 已知限制与未验证项

- **Linux 已在隔离环境用真实 Chrome 验证**（Debian 13 + Xvfb/Xvnc，官方 Chrome Stable
  解包运行，隔离的 `HOME`/`XDG_*`）：Profile 发现、选择身份后进入正确的 Profile
  （按 per-profile History 与 CDP 双重确认）、队列依次出队并退出、启动失败保留请求
  可重试、后启动的实例只把命令行转给第一个实例、`Ctrl+C` 复制（含应用退出后仍可粘贴）、
  `Ctrl+,` 与 Esc 分层。尚未在真实桌面会话里长期使用。
- **Windows 未在原生机器上验证**：目前只有交叉编译与 Wine 下的证据。Wine 里
  `go test` 72 项通过、zip 的安装/重装/卸载、交给 Chrome 的 argv 保真、`.desktop`
  类检查都过了；原生 Windows 至今没跑过（CI 也未在原生 runner 上推过）。因此不作
  「可用」结论：交接时序、`UserChoice` 解析、Windows 11 材质、任务栏与焦点行为都待原生验证。
- **Wine 下的渲染：先前「文字全空」是测试环境问题，不是产品缺陷**。那张截图里的地址、
  路径与数字徽标空着，原因是测试机的 Wine 用 `--no-install-recommends` 安装、缺了
  `fonts-wine`（前缀 `drive_c/windows/Fonts` 为空）。补装字体并重启前缀后，同一份 exe
  正常显示域名、路径、两个数字徽标与两张卡的邮箱。
- **这个 Wine 前缀的中文仍有缺字/错字**：产品里部分汉字丢失，最小 MyGo 原生 UI 基线
  （三行文字、无动效无变换无材质）在同一前缀里同样出现，所以归因于该环境的 CJK 字体回退，
  而不是本应用。原生 Windows 上未验证。
- **Wine 下第二个实例的转发没通过**：后启动的实例转发命令行时挂起约 25 秒仍未完成。
  Wine 的本地 IPC 与 Windows 不同，原生 Windows 上未验证；Linux 上同一条路径是通的。
- **Windows 上父进程退出后 Chrome 是否存活**未验证。当前不给子进程加
  `CREATE_NEW_PROCESS_GROUP` / `DETACHED_PROCESS`；需要原生证据再决定是否补。
- **Chrome 在 `--enable-automation` 下拒收进程交接**：这是 Chrome 自身的行为
  （`ProcessSingletonNotificationCallback` 的 automation 检查），不是本工具的缺陷。
  用调试端口做自动化验证时不要带这个开关，否则第二次启动会被 Chrome 以
  「Failed to create SingletonLock / Aborting now to avoid profile corruption」
  （退出码 21）拒绝。看起来像锁坏了，其实是交接被丢弃。
- **没有原生材质时界面用实色表面**：Linux 全部如此，Windows 10 与系统「降低透明度」开启时
  也一样。设置面板会说明「当前窗口不提供原生磨砂材质」，外观三项随之置灰；只有窗口本来
  有材质、但系统要求降低透明度或提高对比时，原因才会写成系统偏好。不提供「假装磨砂」的替代。
- **Linux 无系统级菜单栏快捷键**：`Ctrl+C` / `Ctrl+,` 由视图接管，与 macOS 的实现方式相同。
- **观察窗只报「没有观察到失败」**：窗口内没有退出不代表页面已导航完成；如果某次交接
  比观察窗更慢才失败，请求已经出队，晚到的失败不回滚。
