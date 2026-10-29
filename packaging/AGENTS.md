# packaging/
> L2 | 父级: ../AGENTS.md

成员清单
package.sh: 跨平台打包入口（`bash packaging/package.sh linux/amd64,linux/arm64,windows/amd64,windows/arm64`，默认四个目标）。只调 `mygo build`（scheme 与文件关联由 mygo.json 注入，裸 `go build` 缺元数据），Windows 构建后**丢弃** MyGo 的 NSIS `Setup` 安装器（它写 HKCU Classes 的 http/https，会抢默认浏览器），只打 `exe + install.cmd + uninstall.cmd` 的 zip（无资源与 manifest）。Linux 复用 MyGo 的官方产物：`.deb`、`.desktop` 与构建目录里的 `install.sh` 原样保留，但 **tar.gz 必须在打包期重打**，把同目录生成的 `install.sh`（保留可执行位）补进归档顶层。MyGo 生成的 tar.gz 并不含它，直接分发会导致「解压后 `./install.sh` 找不到」。不另造 installer、不改 `.desktop` 与 `.deb`、不跑 `xdg-mime default`。压缩工具按平台选 `zip` 或 PowerShell `Compress-Archive`（MSYS 路径经 cygpath 转换）。
release.sh: 发布元数据与资产闸门（`tag-check` / `assets` / `sums` / `publish`）。版本只从 `mygo.json` 顶层 `version` 取（jq 真解析并断言字符串，坏 JSON/嵌套/类型错都失败），标签 `v<version>` 必须逐字一致，只接受 core + prerelease（build metadata 不在发布格式里，数字段不允前导零）。`assets` 要求目录里正好 8 个非空资产（macOS 2 个 staging 规范名 DMG `Chrome-Profile-Router-<version>-<arch>.dmg`——MyGo 本地名带空格，GitHub 上传 asset 会把空格规范化成 `.`，所以 staging 复制时把 basename 的空格换成 `-`，`expected_assets` 用同一条规则推导、版本串大小写照原样；Linux 2 个 .deb + 2 个 tar.gz、Windows 2 个 zip 仍按 MyGo 的实际命名，.deb 的 prerelease 用 `~`），多一个少一个都失败；`sums` 只写 basename 并用系统校验命令读回；`publish` 以 draft 起手建 Release、传 9 个文件、核对远端名字与大小后才 `--draft=false` 公开，已有 draft 可幂等续传，已公开的 Release 明确拒绝覆盖，远端状态用 `gh release view`（自带 draft 的 GraphQL 兜底），只把 `release not found` 当不存在。需要 `gh`、`GH_TOKEN` 与 jq；只有 `publish` 碰网络。
release_test.sh: 发布策略的离线回归（临时目录 + 假的 `gh`）：标签正负例、manifest 解析失败、8 资产清单（DMG 用公开规范名）、0 字节拒绝、SHA256SUMS 的 basename/规范名、draft 起手与公开顺序、已发布拒绝覆盖、not-found 与 401 的区别、prerelease 标志与 `--latest=false`。假 `gh` 如实模拟 GitHub 上传时把 asset 名里的空格规范化成 `.`，所以清单比对不能假设远端保留空格。不联网、不打包，CI 的 `version` job 会跑它。
windows/: Windows 的用户级安装、卸载与 CI 冒烟脚本，见 windows/AGENTS.md。

约束
- 打包只走 mygo（跨平台产物必须带 scheme 元数据）；不得改依赖源，也不得在产物里手改 MyGo 生成的 .desktop / .deb。
- Windows 分发物只有三个文件：`Chrome Profile Router.exe`、`install.cmd`、`uninstall.cmd`。不得夹带 MyGo 的 Setup.exe，也不新增其他脚本。
- 平台列表只接受 linux/amd64、linux/arm64、windows/amd64、windows/arm64；macOS 由根目录 build.sh 负责，这里不碰。
- 打包脚本不写用户环境：不设默认浏览器、不碰 MIME 关联、不写注册表（注册只在 Windows 的 install.cmd 里，且仅候选）。
- 脚本 shebang 是 `#!/bin/bash`，在 macOS 上就是 3.2：变量后面紧跟全角标点时必须写 `${var}`，否则 3.2 会把标点的字节并进变量名，`set -u` 下报 unbound variable。
- 版本只有一个来源（`mygo.json` 的 `version`）：`package.sh` 与 `release.sh` 都读它，不新增 `VERSION` 文件、Go 常量或第二份资产命名表；资产名只能跟着 MyGo 与 `package.sh` 的实际输出走。
- `release.sh publish` 只由 CI 的 `publish` job 调用；本地只跑 `tag-check` / `assets` / `sums` 与 `release_test.sh`，不得对真实远端创建、修改或删除 Release。
- `release_test.sh` 里的假 `gh` 与临时产物只证明策略，不得当成打包证据；真实 8 个资产由 tag 上的 CI 构建。

验证
`bash -n packaging/package.sh`；四个目标各跑一次 `bash packaging/package.sh <platform>`，确认产物齐全（Linux 的 exe/.deb/.tar.gz/install.sh/.desktop，Windows 一个 zip 且只含三个文件），并检查 Linux tar.gz 里含顶层 `install.sh` 且可执行位保留。`bash -n packaging/release.sh && bash packaging/release_test.sh`；发布前 `bash packaging/release.sh tag-check v<版本>`。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
