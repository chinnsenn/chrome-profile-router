# 发布流程

版本号只有 `mygo.json` 的 `version` 一处来源：不引入 `VERSION` 文件，也不再写 Go
常量。GitHub 上的发布标签是 `v<version>`，与 `mygo.json` **逐字一致**；标签由人
手动创建，工具链不会自动打标签、改版本或提交。

## 版本规则

- 接受 SemVer 2.0.0 的 core 加可选 prerelease：`0.1.0`、`0.1.0-rc.1`。
- **build metadata 不在发布格式里**：标签与 manifest 只写 `v<core>[-prerelease]`。
  同一个版本串要同时当标签名、资产文件名和 macOS 的 `CFBundleShortVersionString`，
  多一种写法就多一处对不齐的东西，所以直接拒绝（`v0.1.0+build.1` 会在闸门失败），
  而不是声称 SemVer 本身不允许它。
- 数字段与数字 prerelease 标识符不允许前导零：`v01.0.0`、`v0.1.0-rc.01` 都被拒绝。
- `settingsVersion`（`settings.go` 的配置 schema）与版本号无关，发布不改它。

## 发布一条版本

前提：先把这次要发布的代码、工作流和文档提交到 `main`——标签指向的提交里必须已经有
`.github/workflows/ci.yml` 与 `packaging/`，否则标签会触发旧工作流。下面的示例是
「代码已在 `main`、只改版本」的常规流程；首次 0.1.0 已经写在 `mygo.json` 里，完成本次
提交后直接打 annotated tag，不必再改版本。

```bash
# 常规版本变更：只提交版本这一处（不要 git add .，别把无关 WIP 带进发布）
$EDITOR mygo.json
git add mygo.json
git commit -m "chore(release): 0.1.0"

# 打 annotated tag，名字必须与 mygo.json 的 version 逐字一致
git tag -a v0.1.0 -m "Chrome Profile Router 0.1.0"

# 推送分支与标签（标签触发发布）
git push origin main
git push origin v0.1.0
```

推送标签前可以先本地核对，不联网、不打包：

```bash
bash packaging/release.sh tag-check v0.1.0   # 打印 version/tag/prerelease/publish
bash packaging/release_test.sh               # 标签、资产名与重复发布的策略回归
```

`release.sh` 用 `jq` 真的解析 `mygo.json`（runner 预装；字段缺失、类型不对或 JSON
损坏都会失败），不靠正则猜版本。

## 触发规则

`.github/workflows/ci.yml` 只在三种情况运行：

| 事件 | 做什么 |
| --- | --- |
| push 到 `main` | 三平台原生 vet/test，macOS 用 `-skip-dmg` 快速打包；不发布 |
| pull_request | 同上；不发布 |
| push `v*` 标签 | 先过版本闸门，三平台完整打包，再发布 Release |

`version` job 是闸门：它读 `mygo.json`，校验标签格式与一致性，并输出
`version` / `tag` / `prerelease` / `publish`。三个平台的打包与 `publish` job 都依赖它，
所以标签非法或与 `mygo.json` 不一致时，**打包与发布都不会开始**，日志里直接给出
「标签 v0.1.1 与 mygo.json 的 version 0.1.0 不一致」这样的错误。普通分支与 PR 只
校验 `mygo.json` 本身，不会被标签规则误伤。

`publish` job 的条件写死了事件类型：只有 `push` + `ref_type == tag` + 闸门的
`publish == 'true'` 同时成立才运行，而且要求三个平台 job 全部成功。`if` 里重复检查
事件类型是有意的——PR 能改工作流里的脚本，但不能让一个带写权限的 job 在 PR 上跑。
`contents: write` 只开在这一个 job 上，用 `GITHUB_TOKEN`，不需要个人 token 或额外的
secret。同一条标签的重跑是排队而不是取消，避免打断正在上传的 Release。

## 发布资产

一条版本发布共上传 9 个文件：8 个安装包加 `SHA256SUMS`。

| 平台 | 资产 | 命名来源 |
| --- | --- | --- |
| macOS arm64 / amd64 | `Chrome Profile Router 0.1.0 arm64.dmg`、`… amd64.dmg` | MyGo：`<name> <version> <arch>.dmg`，**文件名含空格** |
| Linux amd64 / arm64 | `chrome-profile-router_0.1.0_amd64.deb`、`…_arm64.deb` | MyGo：`<slug>_<deb 版本>_<arch>.deb` |
| Linux amd64 / arm64 | `chrome-profile-router-0.1.0-linux-amd64.tar.gz`、`…arm64…` | 归档里含原版 `install.sh` |
| Windows amd64 / arm64 | `chrome-profile-router-0.1.0-windows-amd64.zip`、`…arm64…` | 只含 exe + `install.cmd` + `uninstall.cmd` |

两个与版本写法有关的细节：

- `.deb` 的版本字段不是原样的 SemVer：MyGo 把 prerelease 的 `-` 换成 `~`，所以
  `0.1.0-rc.1` 的 .deb 叫 `chrome-profile-router_0.1.0~rc.1_amd64.deb`。其余资产沿用
  原样的版本字符串。
- Windows 的 zip 来自 `packaging/package.sh`，MyGo 的 `Setup.exe`（NSIS）在打包时就被
  丢掉，不进 zip、也不进 Release。

每个平台 job 先把自己那 2 / 4 / 2 个文件复制进 `$RUNNER_TEMP/release-assets` 这个
**扁平** staging 目录，核对数目（macOS 2 个 DMG、Linux 2 个 .deb + 2 个 .tar.gz、
Windows 2 个 zip）、包名里的版本、以及没有 0 字节文件，然后才上传；不直接上传
`build/` 下的目录，否则 artifact 里会保留 `darwin-arm64/` 这类子目录，下载后
`dist/` 顶层就拿不到那 8 个文件。

`publish` job 逐字比对这 8 个文件名（含版本、含正确架构），多一个（例如旧的 1.0.0
包）少一个都直接失败，不会把半套或空目录发出去。上传动作带 `if-no-files-found: error`。

### SHA256SUMS

`SHA256SUMS` 只写 basename（没有目录前缀，也不包含它自己），这样下载到同一目录后
直接校验：

```bash
# Linux
sha256sum -c SHA256SUMS
# macOS
shasum -a 256 -c SHA256SUMS
```

含空格的 DMG 名在这个格式下同样能被解析；脚本写完会自己做一遍同样的读回校验，
上传前再重新生成一次。

## 签名与平台状态

- **macOS 未公证**：没有 Developer ID，MyGo 按默认的 ad-hoc（`-`）签名。包能装能用，
  但 Gatekeeper 会认为「无法验证开发者」，首次打开可能要在「系统设置 → 隐私与安全性」
  里手动允许。仓库不放签名 secret，也不在安装后改动已签名资源；绕过 Gatekeeper 的
  `spctl` / `xattr` 命令不写进文档也不进脚本。
- **Windows 为实验支持**：zip 只把自己登记成 http/https 候选，但原生 Windows 的默认
  应用注册、单实例与显示效果尚未验证，迄今只有 amd64 在 Wine 下跑过功能测试（中文
  缺字与第二实例转发仍不通）。Release 正文的固定前言会写明这一点，再接 GitHub 自动
  生成的 changes。
- **Linux**：amd64 在隔离环境用真实 Chrome 端到端验证过；arm64 只构建、没有实际运行
  记录。详见 [platforms.md](platforms.md)。

Release 正文的固定前言由 `packaging/release.sh` 生成，文档链接固定到本次标签
（`/blob/v0.1.0/docs/…`），不随 `main` 漂移。

## 重跑与失败处理

- **Release 不存在**：先建 draft（不带文件），再传 9 个文件，核对远端的名字与大小，
  最后才 `--draft=false` 公开。中途失败只留下一个 draft，不会露出半成品。
- **Release 是 draft**（例如上次传到一半失败）：允许幂等续传，同名资产 `--clobber`
  覆盖，核对完再公开；不删除 Release。
- **Release 已公开**：明确拒绝覆盖，报错退出，不做任何写请求。要发新内容就打新版本
  标签；不删除、不重建、也不给别的标签覆盖资产。
- **远端状态查询**：用 `gh release view`，它自己会按 tag 查已发布 Release，再对 draft
  走 GraphQL 兜底（`cli/cli` 的 `shared.FetchRelease`），所以 draft 也能查到；只把 gh
  明确的 `release not found` 当「还没有 Release」。网络、权限（如 401）等错误原样失败，
  不会接着盲发一次公开创建。
- **预发布状态只由版本决定**：`0.1.0-rc.1` 一定是 prerelease，并显式
  `--latest=false`，不冒充最新稳定版；稳定版本按 GitHub 默认成为 latest。环境变量
  `PRERELEASE` 只能与版本一致，说反了直接失败。
- 版本发错了：改 `mygo.json`、提交、打新标签，不要改已有标签指向。

## 本地验证边界

`packaging/release_test.sh` 用临时目录与假的 `gh` 锁定这些策略（标签正负例、资产
8 文件、0 字节拒绝、draft 起手、已发布拒绝覆盖、not-found 与 401 的区别、prerelease 标志），
不联网、不打包。真实 8 个资产由 CI 在 tag 上产出；本地只跑这套策略与
`packaging/release.sh assets/sums` 的组装命令，不把测试夹具当打包证据。
