# docs/
> L2 | 父级: ../AGENTS.md

成员清单
visual-design.md: 开发资料，记录选中卡流彩与柔光参考的开源项目、固定 commit 链接与结构差异。
platforms.md: 平台支持矩阵与验证记录：各平台的 Chrome 路径与启动方式、默认浏览器只注册为候选的边界、启动观察窗的语义，以及 Linux/Windows 上已验证与未验证的清单。
releasing.md: 版本与发布的唯一说明：`mygo.json` 是版本的唯一来源（新内容用尚未占用的版本号，标签与该提交的 version 逐字一致）、`v<SemVer>` 标签规则（只收 core + prerelease，拒绝 build metadata 与前导零）、手动 commit → annotated tag → push tag 的命令、闸门与三平台 job 的触发条件（先让 `main` 的 `checks` 全绿）、9 个资产（8 个包 + SHA256SUMS）的命名与校验命令、ad-hoc 未公证与 Windows 实验支持的限制，以及 draft 起手、已发布拒绝覆盖、需改代码才修好时换新 patch 标签等重跑行为。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
