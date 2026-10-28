# .github/workflows/
> L2 | 父级: ../../AGENTS.md

成员清单
ci.yml: 一个版本闸门 + 三个原生打包 job + 一个发布 job。触发只有 push `main`、`pull_request`、push `v*` 标签（同一个 ref 的 PR 运行取消旧运行，标签运行排队不取消）。`version`（ubuntu）：跑 `packaging/release_test.sh`，再按 `github.ref_type` 调 `packaging/release.sh tag-check`，输出 `version`/`tag`/`prerelease`/`publish`。`macos`：`go vet ./...`、`go test ./...`、`bash -n build.sh`；PR/main 用 mygo `-skip-dmg` 打 darwin/arm64+amd64，tag 时不带 `-skip-dmg` 生成双架构 DMG（ad-hoc 签名，不公证），复制 2 个 DMG 进 `$RUNNER_TEMP/release-assets` 后才上传。`linux`（ubuntu）：`go vet ./...`、`go test ./...`、`bash -n packaging/package.sh`，跑 `packaging/package.sh linux/amd64,linux/arm64`，把 2 个 .deb + 2 个 .tar.gz 复制进同一个扁平 staging 再上传。`windows`（windows-latest，bash 步骤 + `shell: pwsh` 的冒烟）：`go vet ./...`、`go test ./...`，跑 `packaging/package.sh windows/amd64,windows/arm64`，用 `packaging/windows/smoke.ps1` 在隔离用户里实跑安装/卸载与注册表键面，再把 2 个 zip 复制进 staging 上传。staging 步骤核对数目、包名里的版本与「没有 0 字节文件」。`publish`：条件写死 `push` + `ref_type == tag` + `publish == 'true'`，并且三个平台 job 全成功；权限单独开 `contents: write`，下载三份 artifact 后在 `dist/` 校验 8 个资产、写 SHA256SUMS，再用 `packaging/release.sh publish`（`gh` + `GITHUB_TOKEN`）以 draft 起手创建/续传并公开。

约束
- 三个平台各跑**原生** `go test`（不是只有交叉编译）：平台 build-tag 的测试文件只有在对应系统上才会真正执行。
- 版本只在闸门里解析（`release.sh` 用 jq 读 `mygo.json` 顶层 `name`/`version` 并断言是字符串）：标签格式非法或与 manifest 不一致时，任何打包 job 都不开始；普通分支/PR 不要求标签，也不创建 Release。
- artifact 必须从**扁平 staging** 上传，不直接传 `build/<goos>-<arch>/` 目录：upload-artifact 会保留共同根下的架构子目录，`download-artifact` 的 `merge-multiple` 不会递归摊平，`dist/` 顶层就拿不到 8 个文件。
- `permissions` 默认 `contents: read`，`contents: write` 只出现在 `publish` job；`publish` 的 `if` 不只看 job output，还重复检查事件类型与 ref 类型（PR 能改脚本，不能让写权限 job 在 PR 上跑）。不引入个人 token 或额外 secret，不用 MyGo 的 `build -upload`。
- 上传 artifact 一律 `if-no-files-found: error`；`publish` 先逐字校验 8 个资产名（含版本、含架构），多一个少一个都失败。
- 只有一个 Release 正文来源：`release.sh` 的固定前言（Windows 实验支持、macOS 未公证，文档链接固定到本次标签）+ `--generate-notes`。
- 发布写操作只发生在 `publish` job：不存在时 draft 起手、传完核对再公开；已有 draft 允许幂等续传；**已公开的 Release 拒绝覆盖**（打新标签）。
- CI 不写用户的默认浏览器、不碰 MIME 关联；Windows 冒烟只用隔离的 CI 用户与自建临时目录。
- macOS job 保持既有校验与双架构打包；跨平台产物一律经 `packaging/package.sh`（走 mygo，带 scheme 元数据）。
- 上传的产物只含分发需要的文件，Windows 不得包含 MyGo 的 NSIS Setup。

验证
`bash packaging/release_test.sh`（闸门 job 里也跑）；`actionlint .github/workflows/ci.yml`；推一次 PR 看四个 job 全绿且 `publish` 被跳过（上传的 artifact 应只有扁平文件名）。打标签前用 `bash packaging/release.sh tag-check v<版本>` 预检。Windows 冒烟失败时 `smoke.ps1` 会打印具体缺失的键或多余的值。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
