# Chrome Profile Router - 选择 Chrome Profile 再打开链接的菜单
Go 1.27.3 + MyGo v0.6.4；macOS 用原生 Vibrancy，Linux/Windows 走 MyGo 的实色/系统材质

<directory>
docs/ - 开发资料：视觉来源与出处说明、平台支持与验证记录、发布流程（见 docs/platforms.md 与 docs/releasing.md）
resources/ - 应用图标源与打包资产
packaging/ - 跨平台打包与安装：package.sh 统一走 mygo；Linux 复用官方产物并把构建目录里的 install.sh 补进 tar.gz；Windows 只分发 exe + install.cmd + uninstall.cmd 的 zip（见 packaging/AGENTS.md）
.github/workflows/ - 版本闸门、三平台原生校验与打包、`v*` 标签发布（见 .github/workflows/AGENTS.md）
</directory>

成员清单
main.go: 请求队列、单实例入口与应用生命周期；按屏幕工作区调整窗口，成功启动后才将对应请求 ID 出队；在创建窗口前读入设置文件，用其中的材质档位与主题初始化外观；单实例锁只在 Linux/Windows 启用（macOS 由 LaunchServices 保证），后实例只把面板拉回前台，命令行由 MyGo 转发到 OnOpenURL/OnOpenFile。
chrome.go: Profile 发现的解析层——读取 Chrome 的 `Local State`，按 `info_cache` 解出名称/邮箱/头像，跳过磁盘上已不存在的目录，`Default` 排在最前；如实区分文件缺失、JSON 损坏与无有效条目，绝不虚构占位身份；平台根目录由 chrome_<goos>.go 提供。
chrome_darwin.go: macOS 的 Chrome 用户数据根目录（`~/Library/Application Support/Google/Chrome`），逐字保持已发布行为。
chrome_linux.go: Linux 的根目录与可执行文件事实：`$XDG_CONFIG_HOME/google-chrome`（为空或相对则 `~/.config/google-chrome`）、PATH 上的 `google-chrome` → `google-chrome-stable`；不解析 Chromium/Flatpak/Snap。
chrome_windows.go: Windows 的根目录与可执行文件事实：`%LOCALAPPDATA%\Google\Chrome\User Data`（取不到即报错，不回退 USERPROFILE）、`chrome.exe` 按 LOCALAPPDATA → ProgramFiles → ProgramFiles(x86) 顺序查找，缺失时列出已查找路径；不读注册表。
launch.go: 交付层——`openWith` 在后台启动 Chrome、成功才按请求 ID 出队、失败保留并可重试；`chromeArgs` 只产出两个独立参数（Profile 目录与目标 URL，中文/空格/查询串原样交给 exec 加引号）；`runDetached` 用 `launchGrace`(700ms) 观察窗给出「这次交付是否失败」，窗口内未退出只表示没有观察到失败，不证明页面已导航完成，也不等待浏览器生命周期。
launch_darwin.go: macOS 的 launchChrome：`/usr/bin/open -n -b com.google.Chrome --args --profile-directory=… URL`，8s 超时，参数与文案保持原样。
launch_linux.go: Linux 的 launchChrome：直启 PATH 上的 Chrome，`setsid` 进入独立会话（应用交付完退出时不带走浏览器），再交给 runDetached。
launch_windows.go: Windows 的 launchChrome：直启 chrome.exe 再交给 runDetached；不加进程创建标志（父进程退出后浏览器是否存活尚未验证，见 docs/platforms.md）。
destination.go: 整幅宽度的地址卡：域名主行、路径/启动中/失败原因次行、队列剩余徽标、复制按钮；复制走 `mygo.Clipboard` 的跨平台实现（不再调用 pbcopy），失败可回显；设置面板打开时快捷键不复制。
settings_panel.go: 设置面板的控件与绑定（两个浓度滑块、四档材质、三个动效开关、两档减少动态效果、恢复默认）以及面板模式位与 Esc 分层所需的开关函数；替换身份列表区、高度固定，关闭时冲刷设置。
shortcut_darwin.go: macOS 的快捷键文案（`⌘`+键）。
shortcut_other.go: 非 macOS 的快捷键文案（`Ctrl+`+键）；绑定本身来自 ui.Cmd，文案只影响提示。
fileurl_other.go: 非 Windows 的 file URL 组成：POSIX 路径没有 host，path 原样交给 url.URL 转义。
fileurl_windows.go: Windows 的 file URL 组成：盘符路径补前导 `/` 得到 `file:///C:/…`，UNC 的 server 作为 host 得到 `file://server/share/…`，反斜线一次性换成 `/`，绝不让 `%5C` 或「C: 被当成 scheme」进入 URL。
settings.go: `<PathUserData>/settings.json` 的读写层（版本号 + 外观分组 theme/material/backgroundTint/cardTint + 动效分组 motion.spin/hover/tilt/reduceMotion）；首次运行原子写入默认值文件，缺失/损坏/未知键/越界/更新版本各自按默认值、夹取或只读运行，不覆盖用户文件；写盘前先读回旧文件按层合并，保留用户手改的未知键（顶层、appearance 与 motion 内），已知字段以内存中夹取后的值为准；材质名只收 mygo 的 macOS 材质常量，未知回落 popover；动效分组整组缺失时回落全开 + system，未知 reduceMotion 回落 system（没有 off）；面板改动走内存写入 + 标脏，离散项与关闭面板/退出前冲刷，滑块拖动期间不逐帧写盘；不碰原生窗口。
motion.go: 内容层的入场与可中断出场生命周期（180ms 淡入 / 120ms 出场、缩放与 Y 位移、仅内容层降透明度）；以动画通道 Settled 判定完成并经 Window.Update 投递后 Quit，新请求在同一把锁下取消自然出场，显式关闭不被取消。
layout.go: 单列与三列画廊共享的列数、卡片高度、头像/徽标尺寸和窗口尺寸预算；固定控件高度推导窗口尺寸，不预留未显示状态的空白；另提供设置面板的固定高度预算 panelSettingsHeight 与 settingsPanelSize（宽度沿用列表预算、高度固定，不参与 panelChromeBase/panelChrome 的推导）。
hover_layout.go: 悬停放大与邻居让位的纯几何（网格同排沿 x 放大到 1.06、单列沿 y 增高到 1.08，邻居按 1/(1+d) 距离权重缩窄并移位，带首尾外边界与卡片间距保持不变，交叉轴只吃列表内缩/行间距余量）；零逐帧分配、不读指针与应用状态，静止时返回单位变换。
view.go: 身份画廊与用户输入，指针/键盘意图转成选择、启动与退出；手动主题同步原生磨砂外观；header 右侧的设置图标（三条滑杆，13pt 自绘，不用齿轮以免被误读成太阳/亮度）与视图内快捷键切换设置面板（文案按平台来自 shortcut_<goos>.go），Esc 分层与面板期间的键盘屏蔽也在这里；地址卡在 destination.go，面板控件在 settings_panel.go。
profile_view.go: 单列与三列共用身份卡片（身份色细条、32pt 真实头像、名称/邮箱两行、数字键徽标），拆成不随悬停变化的布局槽位、承载放大/让位并接收指针的可交互层、以及只做倾斜的内层表面三层，槽位盒子不随悬停移动；三个动效开关在这里落到绘制上（关掉流彩只保留静态基础边框与柔光且不调 Keyframes，关掉悬停放大走与 ReduceMotion 相同的单位变换，关掉倾斜不旋转不视差）；槽位 PassThrough 退出命中登记，命中与悬停只由已变换的交互层决定，可见边缘与可点边缘一致；悬停归属由窗口坐标与上一帧几何（可见矩形→静止槽位）判定，动画不能自己切走悬停；提供无障碍描述、悬停 Tooltip 与空格详情浮层。
card_motion.go: 身份卡悬停 3D 倾斜与内层视差（5.5° 上限、透视距离为卡宽 2.5 倍、内容 1.5pt / 徽标 2pt、可打断弹簧跟随、离开归中）；倾斜开关与减少动态效果都由调用方传入，关掉时走与 ReduceMotion 相同的归零通路，不另写几何；只输出旋转与视差变换，不含缩放——放大与让位统一由 hover_layout.go 分配给整个画廊，这里再叠缩放会双重放大；不写选择、不改布局。
view_style.go: 原生磨砂上的地址卡与卡片薄层、细边界、卡片投影与轻量反馈，保持文字对比；高对比或降低透明度时改用不透明表面并去掉投影；根背景与地址卡的覆盖 alpha 由入参决定（缺省值来自 settings.go 的默认外观）、常态/选中卡的次行文字色都在这里定义。
card_glow.go: 选中身份卡的流彩描边与外辉光的参数与预合成——Stripe 的 5 stop 位置、BorderBeam 的 6s 周期与 1pt 线宽及暖/紫两色（首尾同色以保证转圈接缝连续）、Pow 的内两级单色阴影公式（base radius 19 与 gain 0.62 均为按真机像素调出的适配值，非 Pow 默认 16 / 1；派生半径 3.325 / 6.650pt，依据是选中卡底边外 2–5px 与未选中同像素相差浅色 27/20/13/9、深色 20/14/10/5 级，邻卡 0–1 级）。stops 与基础选中边框按 source-over 预合成，缓存经 `ui.Local` 归属当前卡片、以 accent 为键，因此有状态但不跨卡片/窗口共享；逐帧只改渐变起始角，由共享静态 Keyframes 调度，无逐帧分配、无 shader、无贴图。
ui_test.go: 混合输入、九身份同屏、末行边界和窄屏导航回归，防止静止悬停覆盖键盘选择；设置面板的入口（header 设置图标 / ⌘,）、Esc 分层、面板期间数字键与回车不启动身份、Tab + 方向键调值、恢复默认、固定高度、外观三项在无材质/降低透明度下置灰、流彩关闭后动画帧收敛与静态边框仍在、倾斜与悬停关闭后几何停在静止槽位、强制减少动态效果与系统一致。
card_motion_test.go: 倾斜方向与钳制、ReduceMotion 归零、槽位几何不变、真实 3D 变换、网格顺序与列数、内容不越界、跨卡片宽度强度一致。
hover_layout_test.go: 网格同排与单列整列两种带状、悬停卡放大到 1.06/1.08、邻居按距离权重缩窄、带首尾外边界与间距守恒、卡片互不重叠、边缘卡单侧锚定、交叉轴受间距/内缩约束、ReduceMotion 与未就绪布局返回单位变换。
hover_ui_test.go: 放大后命中区域跟随可见卡（首列 grownRight−2 必须落在邻居静止槽位内仍归本卡）、点击放大边缘只启动该卡一次、静止指针不伪造 hover 切换、离开后几何复原、悬停不改变窗口尺寸/列数/身份顺序。
motion_test.go: 出场等待、显式关闭优先级、重置入场、立即中断、新请求取消与 ReduceMotion 的行为回归。
routing_test.go: 请求队列与失败重试的行为边界（文件 URL 编码已按平台拆到 fileurl_*_test.go）。
fileurl_other_test.go: POSIX 的 file URL 组成——host 为空、路径逐段保真（空格/#/?/% 都只是路径字符）、不伪造查询串与片段。
fileurl_windows_test.go: Windows 的 file URL 组成——`C:\…` 得到 `file:///C:/…`、`\\server\share\…` 得到 `file://server/share/…`，用 url.Parse 独立解码并断言无反斜线/无 %5C。
chrome_test.go: `loadProfilesFrom` 的解析契约：缺文件/坏 JSON/空条目必须报错、目录已不存在的条目被跳过、`Default` 优先、名称回落到目录名（用 t.TempDir 造 Local State，不依赖本机 Chrome）。
chrome_darwin_test.go: macOS 根目录仍是 `~/Library/Application Support/Google/Chrome`。
chrome_linux_test.go: Linux 根目录的 XDG 优先级（含相对路径被忽略）与 PATH 上 Stable 命令的查找顺序、缺失时的可读错误。
chrome_windows_test.go: Windows 根目录只在 LOCALAPPDATA 下解析、缺失即报错，`chrome.exe` 的 per-user 优先顺序与缺失时列出已查找路径。
helper_test.go: 测试二进制作为假 Chrome 的 TestMain 分支（argv 落盘 / 非零退出 / 长驻等待释放文件）、newHelperCommand 与 waitFor（等状态而不是等固定时长）；进程边界断言不依赖本机 Chrome，长驻 helper 由 t.Cleanup 释放，不留后台进程。
launch_test.go: `runDetached` 的三类结局（立即非零失败、干净早退、长驻时按观察窗返回）；队列的交付状态由平台测试文件经真实 `openWith` 覆盖。
launch_linux_test.go: Linux 端到端——从 PATH 找到假 chrome，profile 目录与目标 URL（中文、空格、&、#、本地文件路径）原样到达子进程，缺 Chrome 时不启动；另经真实 `openWith` 覆盖「失败保留请求」「成功后出队且收起 loading」。
launch_windows_test.go: Windows 端到端——从 LOCALAPPDATA 找到假 chrome.exe，同样的参数保真、URL 用 url.Parse 独立校验、缺 Chrome 报错，以及同样的 `openWith` 交付状态。
settings_test.go: 设置文件的首次生成默认值、损坏回落不覆盖、越界夹取、未知键容错、`version:2` 不回写、未知材质回落 popover、主题往返、motion 缺键/部分缺键/未知 reduceMotion 的回落、motion 层未知键在写盘后仍保留、恢复默认立即落盘、浓度只写当前外观那一侧且量化到 0.02 的行为边界（只写临时目录）。
build.sh: 根据本机架构构建，在目标卷暂存验签后替换应用，失败时恢复旧包。
README.md: 面向新访客的简明说明：定位、支持情况、安装、使用与验证命令；细节与视觉参数不在 README 展开。
docs/visual-design.md: 选中卡流彩与柔光的来源、固定 commit 链接与结构差异；视觉数值仍以源码常量为准。
docs/platforms.md: 平台支持矩阵与验证记录：各平台的 Chrome 路径与启动方式、默认浏览器只注册为候选的边界、启动观察窗的语义，以及 Linux/Windows 已验证与未验证的清单。
go.mod / go.sum: Go 工具链及固定依赖版本。
mygo.json: Bundle 标识、URL scheme、文档类型、Linux 打包意图和版本的唯一声明来源（当前 `0.1.0`；发布标签是 `v` + 这个版本串，逐字一致，规则见 docs/releasing.md）。`linux.webview: false` 声明这是原生 UI（不加载网页），因此 `.deb` 只依赖 GTK3、安装脚本也不检查 WebKitGTK；删掉它会让 mygo 默认按 WebView 应用打包，凭空加上 libwebkit2gtk 依赖。

约束
- 版本只有 `mygo.json` 的 `version` 一处来源：不新增 `VERSION` 文件或 Go 常量，资产命名也不另立表。发布用 `v<SemVer>` 标签（只收 core + prerelease，拒绝 build metadata 与前导零），标签与 manifest 不一致或格式非法时，任何打包与发布都不得开始；普通分支与 PR 只校验、构建，不创建 Release。
- `mygo.App.OnOpenURL` / `OnOpenFile` 统一入队；不再手动遍历启动参数，避免重复请求。
- `openWith` 持锁时只调用 `targetLocked` / `currentLocked`；Chrome 启动在后台运行，结果经 Window.Update 回到主线程。
- Esc 分层：设置面板打开时 Esc 只关闭面板并回到身份列表，面板已关时才退出应用；关闭按钮、⌘Q 与系统退出不受队列或动效拦截。除此之外 Esc 永远有效（启动中、错误、空状态下都能用），装饰性动效不能延迟键盘选择与链接交付。
- 设置入口就是 header 设置图标（滑杆）+ 视图内 ⌘,：不得新增独立设置窗口、不得加菜单栏常驻入口、不得改 `Panel` 语义。面板窗口不激活应用、没有菜单栏，标准 ⌘, 与 ⌘Q 都收不到，所以两者都必须在视图内接管。
- 设置面板替换身份列表区，地址卡保留（当前链接与剩余条数仍在，也是「阅读面浓度」的预览对象）；面板高度用 layout.go 的固定常量 `panelSettingsHeight`，不随身份数量变化，关闭后窗口回到原来的列表高度。面板打开时 `snap.chosen`、滚动位置、列数与身份顺序一律不变，关闭后选中卡照旧可见。
- 设置面板打开时数字键 1–9、回车、⌘C 与方向键都不得启动身份、复制或改写列表选择（避免误交付链接）；新链接照常入队并在地址卡显示剩余条数。
- 退出只在真实动画通道（Scale/Y/Opacity 三个 Settled）完成后才 Quit，用 Window.Update 投递并复核代次；不得用 Sleep 或固定时长猜测完成，也不得设置退出拦截器——Esc、⌘Q、原生关闭与系统退出始终走立即退出通路。
- 指针坐标取 `c.Root().PointerPosition()`（窗口坐标），是否在列表视口内取列表容器的第三返回值；不得改读面板或卡片的局部坐标，祖先变换会把它伪造成指针移动并覆盖键盘选择。
- 卡片分三层：布局槽位（静止几何，不随悬停变化）、可交互放大/让位层（承载缩放、位移与指针命中）、内层倾斜表面。悬停放大与邻居让位只改后两层的变换，槽位、列数、身份顺序、窗口尺寸与画廊带首尾外边界都不变。
- 悬停放大后，可交互层的命中区域必须跟随放大后的可见卡（指针落在放大出来的边缘上仍算悬停）；放大与让位不得借道重排布局或改写槽位。
- 悬停倾斜只作用于内层倾斜表面，不得加在可交互层或槽位上（否则命中区域、锚点与滚动位置随角度漂移）；放大与倾斜是两条独立通道，互不覆盖——倾斜只做旋转与视差、不含缩放，否则会与 hover_layout.go 的放大叠加成双重放大。
- 系统开启减少动态效果时，放大、让位、倾斜、视差与入场位移必须全部归零，界面保持静止几何。设置里的「减少动态效果：强制开启」与系统开启走同一条归零通路（取并集），**不得**提供「强制关闭」去覆盖系统设置。关掉「悬停放大与让位」或「3D 倾斜」时同样复用 ReduceMotion 的单位变换通路，不得另写一套几何。
- 原生材料负责背景模糊，文本保持不透明；不得通过全窗口 Opacity 降低可读性。系统开启「降低透明度」或高对比时，界面改用实色表面并去掉投影。
- 根背景只覆一层薄主题色（默认深 `#16181D` @ 0.30、浅 `#F6F6F7` @ 0.34），模糊由原生材质负责；文字对比主要由地址卡（默认深 `#1E2229` @ 0.62、浅白 @ 0.66）承担，浮层压在卡片上故更实一档（深 0.72、浅 0.82）。覆盖 alpha 不得加厚到压掉原生磨砂，也不得据此外宣称它是原生模糊半径——`NSVisualEffectView` 没有可调模糊半径，能调的只有材质种类与这层覆盖的浓度。根背景与地址卡的 alpha 是 settings.json 里 `backgroundTint` / `cardTint` 的默认值，夹取范围 0.00–0.90 与 0.30–0.95。
- 外观与动效设置只能来自 `<PathUserData>/settings.json`（settings.go）：入口只有 header 设置图标（滑杆）与视图内 ⌘,，不得新增独立设置窗口、菜单栏常驻入口，也不得在 header 之外另开常驻入口。`material` 与 tint 的默认值必须与已验证的视觉基线一致（popover / 0.30·0.34 / 0.62·0.66），动效默认必须等于加入开关之前的行为（spin/hover/tilt = true、reduceMotion = system），删除设置文件后的外观与动效必须与默认值等价。`material` 只是材质档位、tint 只是覆盖不透明度，二者都不能冒充模糊半径或模糊强度。滑块只写**当前外观那一侧**并量化到 0.02（越界夹到边界）；`motion.reduceMotion` 只接受 `system` / `on`，没有 `off`。读取失败、JSON 损坏、未知键、`version > 1` 时按默认值运行且不回写用户文件；系统「降低透明度 / 提高对比」仍优先强制不透明表面，此时面板里的两个浓度与材质置灰只读并给出原因，不得提供「忽略系统设置」的开关。面板改动即时生效、关闭面板与退出前冲刷落盘；手改文件仍需重启应用（没有热重载）。
- 选中卡的柔光只在 `isChosen && !pal.opaque` 时出现，流彩描边在此基础上还要求设置里 `motion.spin` 开着；高对比、降低透明度或窗口无材质时退回实色表面 + 清晰描边，不保留渐变与柔光。ReduceMotion 与关掉流彩都不退回实色：ReduceMotion 只让角度停止推进（Keyframes 完成到末帧 = 0°），关掉流彩只保留静态基础选中边框与柔光——两者都不消失，且都不得留下空转的动画帧（关掉流彩或强制减少动态效果时不得调用 Keyframes）。描边与柔光不得参与布局、不得新增布局元素、不得改变命中、选择或窗口尺寸。
- 流彩的 stops 必须与基础选中边框预合成，透明 stop 处要露出基础边；不得出现某个相位整圈描边消失。柔光替换选中原有的单层投影，不与它叠加，也不加淡入延迟或脉冲。
- 1–4 个身份使用单列卡片，更多身份按可用宽度排成最多三列画廊；列数、卡片高度与头像尺寸只来自 layout.go，状态提示与悬停都不改变窗口尺寸、列数或身份顺序。悬停只重新分配同一条带内的空间：网格是同排、单列是整列，邻居按距离权重缩窄并移位，带首尾外边界与卡片间距保持不变。
- 只有地址卡与详情浮层是始终带投影的表面，身份卡常态近透明；两处例外：悬停卡保留一层极浅投影（`Shadow(0,1,3,0,cardShadow)`），选中卡的两级柔光是描边的延伸而非体积投影。除此之外不得给身份卡加投影，也不得用投影改变布局尺寸。
- URL 域名独占地址卡主行、路径独占次行；卡片上的 1–9 数字键徽标是身份标识，不另设快捷键说明文字；错误或启动状态替换次行，不能预留底部空白。
- 图标修改同步 SVG 与 PNG，MyGo 自动生成 ICNS；不得在安装后另行修改已签名资源。
- 平台差异只允许出现在 build-tag 文件里：路径与可执行文件在 chrome_<goos>.go，启动在 launch_<goos>.go，快捷键文案在 shortcut_<goos>.go；不得为跨平台另造公共接口、配置项或运行时开关。
- 范围只有 Chrome Stable：darwin 用 LaunchServices 的 bundle id，linux 只认 PATH 上的 `google-chrome` / `google-chrome-stable`，windows 只认 LOCALAPPDATA / ProgramFiles / ProgramFiles(x86) 下的 `chrome.exe`。不解析 Chromium、Beta、Flatpak、Snap，也不读注册表或新增环境变量回退。
- 默认浏览器只注册为候选：应用运行期不写任何注册表、不调用 `RegisterURLScheme`、不碰 `UserChoice` 与任何「设为默认」的 API。Windows 的候选键由安装脚本在安装时写入，Linux 靠 `.desktop` 的 `MimeType`/`%U`，安装脚本不得调用 `xdg-mime default`。
- 启动握手：darwin 仍用 `/usr/bin/open` 加 8s 超时；linux/windows 直启 Chrome，用 `launchGrace` 观察窗判定——窗口内非零退出即失败并保留请求，窗口内 0 退出或窗口结束时仍在运行都只表示"没有观察到失败"，不证明页面已导航完成。绝不等待浏览器进程生命周期，绝不删除 `SingletonLock`、不加启动重试、不按错误码硬编码绕过。
- 单实例锁只在非 darwin 启用（macOS 由 LaunchServices 保证）；后实例只把面板拉回前台，命令行由 MyGo 转发的 `OnOpenURL`/`OnOpenFile` 入队，不得再遍历一遍参数。
- 复制走 `mygo.Clipboard`（`transfer.TextData`），不再调用 `pbcopy` 等平台命令；快捷键绑定来自 `ui.Cmd`，只有提示文案按平台区分。
- Linux 无原生材质时由 `c.Vibrancy()` 的 `ShowsMaterial()` 自动落实色表面，不新增「假磨砂」或材质配置字段。

验证
`go test ./...`、`go test -race ./...`、`go vet ./...`、`bash -n build.sh`；交叉编译检查 `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...` 与 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...`（平台测试文件同样要能编译：`GOOS=… go vet ./...`）。打包使用 `go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -platform darwin/arm64,darwin/amd64`，跨平台产物由 packaging/ 的脚本按平台列表构建（必须走 mygo，裸 `go build` 缺 scheme 元数据）。发布相关的离线校验是 `bash -n packaging/release.sh && bash packaging/release_test.sh` 与 `bash packaging/release.sh tag-check v<版本>`（发布用 `gh` 只发生在 CI 的 `publish` job，本地不得对远端写）。`./build.sh` 会替换 `/Applications` 中的应用，仅在需要本机安装时执行。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md（成员清单、约束与验证三节必须与实际实现一致）
