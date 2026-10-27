# Chrome Profile Router

一个用 Go 编写的轻量 macOS 默认浏览器分流工具，用于在外部打开链接时选择使用哪个 Google Chrome 账号 Profile。

<p align="center">
  <img src="resources/screenshots/preview-dark.png" alt="Chrome Profile Router 原生深色界面" width="680" />
  <br />
  <br />
  <img src="resources/screenshots/preview-light.png" alt="Chrome Profile Router 原生浅色界面" width="680" />
</p>

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
4. **自适应快捷菜单**：
   URL 是菜单的视觉重点：整幅宽度的地址卡占据顶部，域名以 24pt 粗体独占主行，路径独占次行；启动状态、失败原因和队列剩余条数都在这张卡内提示，不额外占据菜单高度。1–4 个身份使用单列卡片，5 个起使用最多三列的身份画廊；9 个身份在常规桌面上以 3×3 展示，窗口约 700×340，底部只保留 14pt 内边距。每张卡片由身份色细条、32pt 真实头像、名称与邮箱两行、右侧数字键徽标组成。窄屏自动减少列数，达到高度上限后在账号区内滚动。菜单不显示应用标题或快捷键说明；左上角关闭按钮、`Esc` 或 `⌘Q` 可随时退出。关闭按钮首次点击播放一段短出场动画后退出，动画期间再点一次立即退出；`Esc`、`⌘Q`、原生关闭与系统退出始终立即退出，不等待任何动效。最后一条链接成功交付给 Chrome 后菜单自行出场，期间若有新链接入队则取消这次出场并继续留在屏幕上；显式点击关闭不会被新请求取消，自然出场期间点关闭则升级为显式关闭。
   数字键 `1`–`9` 直接打开，回车确认；网格方向键按实际行列移动，末行不足时停在有效身份上；单列保留上下选择、左右循环。选中项自动滚入视野，静止鼠标不会覆盖键盘选择。悬停可查看完整名称和邮箱，键盘用户按空格展开或收起身份详情浮层（含完整名称、邮箱与 Chrome 身份目录）。状态提示不改变窗口尺寸、列数或身份顺序；悬停放大与邻居让位同样不改变窗口尺寸、列数、身份顺序或画廊外边界。
   地址卡右侧点击“复制”或按 `⌘C`，可将当前完整 URL（含查询参数和片段）复制到系统剪贴板；复制后显示“已复制”，不会打开 Chrome 或移除队列请求。
5. **外观与反馈**：
   支持系统、浅色、深色三种外观，手动切换会同步原生磨砂材质。窗口背景保留 `NSVisualEffectView`（Popover 材质、BehindWindow 混合、Active 状态）负责桌面模糊，其上只覆一层主题色薄层；这层的 alpha 与文字色都是为本机可读性选定的适配值，不是 Apple 公开的材质参数——原生材质本身不提供可调的模糊半径，能调的只有材质种类与这层覆盖的浓度。深色覆 `#16181D` @ 0.80、浅色覆 `#F6F6F7` @ 0.80；地址卡深色用中性 `#1E2229` @ 0.72、浅色用白 @ 0.72，让域名与路径压在桌面内容上仍然可读。文字始终不透明：常态卡次行深色 `#CDD3DE`、浅色 `#434D5A`，选中卡次行深色 `#CDD3DE`、浅色 `#374151`。高对比模式或系统降低透明度时改用不透明表面并去掉投影。
   身份卡常态是几乎不可见的薄底；悬停提亮并加一层极浅投影；选中换成主题色薄底、实心数字徽标，并额外得到一圈 1pt 流彩描边与两层同色柔光（仅在有材质且未降低透明度时）。按压只把卡片缩放到 0.98，不改变任何布局盒与列数。选中反馈即时生效，没有淡入延迟，也不做脉冲。
   入场与出场只作用于内容层：面板内容以 180ms 无回弹弹簧淡入（缩放 0.97→1，自下方 6pt 归位），出场用 120ms 弹簧回到缩放 0.98、下移 4pt 并淡出；原生窗口透明度与磨砂材质始终不变，文字对比度不随动效下降。出场以动画通道真正结束为准，不按时钟猜测，也不用固定等待。颜色过渡、按压弹簧与短促淡入都不阻塞操作，并跟随系统减少动态效果；键盘选择与数字键立即生效，没有过渡。
   悬停身份卡时卡片会真实放大，同排邻居缩窄让位：网格里悬停卡沿同一行横向放大到约 1.06 倍，单列里沿纵向增高到约 1.08 倍；交叉轴只在有真实余量时小幅外扩，受列表内缩与行间距约束，因此并非所有布局都均匀放大同样的比例。缺口由同排（单列则为整列）的邻居分担，近邻让得多、远邻让得少；带首尾的外边界与卡片之间的间距保持不变，所以悬停不会推动整个画廊，也不会改变列数、身份顺序或窗口尺寸。鼠标离开后所有卡片平滑回到静止尺寸。卡片分三层：不随悬停变化的布局槽位、承载放大与让位并接收指针的可交互层、以及做 3D 倾斜的内层表面；放大后的可交互区域跟随放大后的可见卡，指针落在放大出来的边缘上仍算悬停。
   倾斜只作用于内层表面，且只做旋转与视差、不再叠一层缩放（放大完全由上一条的让位几何负责）：最大 5.5°，透视距离取卡片宽度的 2.5 倍；卡片内的头像与文字列反向视差 1.5pt，数字键徽标 2pt，指针朝卡片哪一侧靠近，那一侧边缘就靠近观察者。跟随用可打断的弹簧（stiffness 260 / damping 30），指针离开后平滑归中。内容与表面一起转，文字、头像与徽标始终落在边框之内。系统开启减少动态效果时，放大、让位、倾斜、视差与入场位移全部归零，界面保持静止几何，退出即时完成。
   图标以蓝色分流路径与青绿色身份入口表示链接分流，`resources/icon.svg` 为源，`icon.png` 由 MyGo 自动转换为 ICNS。

### 选中卡流彩与柔光的来源

参数取自三个 MIT 项目，`card_glow.go` 的常量与下表一一对应。

| 来源（固定 commit） | 照搬的原值 | 本项目适配值 |
| --- | --- | --- |
| Stripe [`ba6f5ed`](https://github.com/stripe/stripe-ios/blob/ba6f5ed8b3011c3d8223fd9b6b0ee35d2e55e6af/StripeIdentity/StripeIdentity/Source/NativeComponents/Views/DocumentCapture/AnimatedBorderView.swift) `AnimatedBorderView` | stop 位置 `[0, 0.12, 0.55, 0.75, 1]`；亮弧 alpha 结构 `[c1, c2@0, c2@0, c1, c1]` | 透明 stop 与基础选中边框（accent α0.55）预合成，任何相位都不丢边框 |
| Magic UI [`efe76fc`](https://github.com/magicuidesign/magicui/blob/efe76fc5755eca1142dd4f74c1d6f0ace2887e3c/apps/www/registry/magicui/border-beam.tsx) `BorderBeam` | 周期 `6s` 线性无限；线宽 `1`；暖色 `#FFAA40`、紫 `#9C40FF` | 只改渐变起始角，不旋转矩形；首尾 stop 取同一暖色，转圈接缝连续 |
| Pow [`1b4b1dd`](https://github.com/EmergeTools/Pow/blob/1b4b1dda28c50b95f0872927ee2226fe8b58950e/Sources/Pow/Effects/GlowEffect.swift) `GlowEffect` | `amount=0.7`；alpha `sqrt(amount)/{1.2,4}`；半径 `amount*R/{4,2}`；偏移 `(0,0)` | `R=8`（Pow 默认 16，按本卡尺寸取一半），只取内两级 1.40 / 2.80pt，alpha 再乘 0.45；3.3333:1 的内外比例不变。R 是离屏观感调出的适配值，不是来源原值 |

本项目只借用参数与结构，Go 实现为独立重写。两处结构差异：Stripe 与 BorderBeam 都用图层 + mask 只留 1px 环，这里直接用 `BorderConicGradient` 画环；Stripe 靠旋转整张渐变层推进（因此渐变层要做成覆盖对角线的正方形），这里只改渐变起始角，矩形不旋转，不需要那条技巧。主题 accent 不由渐变 stop 承担，而由基础选中边框与两级柔光体现。

无障碍：系统开启减少动态效果时角度停止推进、停在 0°，静态描边与柔光照常保留，选中标识不消失；高对比、降低透明度或窗口无材质时才完全退回实色表面 + 清晰 1pt 描边，不出渐变、不出柔光。

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
