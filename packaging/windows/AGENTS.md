# packaging/windows/
> L2 | 父级: ../AGENTS.md

成员清单
install.cmd: 用户级安装（cmd + reg，无 PowerShell，双击即可）。把 exe 复制到 `%LOCALAPPDATA%\Programs\Chrome Profile Router`（先复制成同卷 `.new` 再改名，失败保留旧版本），然后只写本产品自有命名空间的候选键：`HKCU\Software\Clients\StartMenuInternet\ChromeProfileRouter`（含 `Capabilities` 的 ApplicationName/ApplicationDescription/ApplicationIcon、`URLAssociations\http|https`、`FileAssociations\.html|.htm|.xhtml`、`Startmenu\StartMenuInternet`）、`HKCU\Software\RegisteredApplications` 的 ChromeProfileRouter 值、ProgID `ChromeProfileRouterHTML`（DefaultIcon + `shell\open\command` 带 `"%1"`）、`.html|.htm|.xhtml\OpenWithProgids` 的对应值。提示一律 ASCII 英文，避免 cmd 代码页乱码。
uninstall.cmd: 只删上面的键/值与安装目录里的 exe、安装目录里的本脚本；值或键本就不存在算成功（可重复执行），目录里用户自己放的文件与应用设置一律保留。自删除与退出码是同步的：最后一条命令行先用 `(goto) 2>nul` 让 cmd 停止读取本脚本（行为等同 `exit /b`，但本行后续命令仍在调用者上下文执行），再 `del` 安装目录里的 uninstall.cmd、`rd` 安装目录，最后用存在性判定钉住退出码（安装目录里的 uninstall.cmd 真没了才 0）；不先停读就会出现 `The batch file cannot be found.` 并以 1 退出。从包目录运行时不删包内副本。
smoke.ps1: 仅 CI 用。在隔离的 CI 用户里实跑 `install.cmd` → 逐条核对新增的候选键，并与安装前的快照比对共享键（`HKCU\Software\Classes\http|https` 等既有共享键、`.html` 的默认值、真实 `UserChoice` 位置）保持**逐字节不变** → 跑 `uninstall.cmd` → 核对只有本产品命名空间被新增/移除、共享键与 `UserChoice` 仍与快照一致；卸载返回 0 后立刻断言安装目录里的 `uninstall.cmd` 已消失（同步完成，不用固定 sleep 掩盖时序）。

约束
- 只写 HKCU、只写自己的命名空间，安装不需要 UAC。绝不写 `HKCU\Software\Classes\http|https`、不改 `.html` 的默认值、不碰任何 `UserChoice`、不调用「设为默认」的接口：装完只是候选，默认浏览器由用户在系统设置里选。
- `<exe>` 一律用字面路径（不依赖 REG_EXPAND_SZ 展开）；命令里引号与 `%1` 的转义必须保持（`"\"%EXE%\" \"%%1\""`）。
- 卸载不得删除整个 `.html` 键，只删自己那一个值；保留 `settings.json` 与安装目录中的其他文件。
- 卸载的自删除必须同步完成：脚本返回 0 时安装目录里的 `uninstall.cmd` 已删除（从包目录运行时包内副本按契约保留）。不得用 `start /b` 之类的后台删除、固定 sleep 或「先返回再删」掩盖时序；`smoke.ps1` 在退出码断言之后立刻检查 `%LOCALAPPDATA%\Programs\Chrome Profile Router\uninstall.cmd` 已消失。
- 脚本不依赖 PowerShell（Wine 没有），只用 cmd 内建与 `reg`。

验证
`packaging/windows/smoke.ps1`（CI，隔离用户）；Wine 下等价核对用 `wine reg query` 逐条比对键面，并确认共享键（`HKCU\Software\Classes\http|https` 等）与 `UserChoice` 在安装/卸载前后与快照一致——这些键可能本来就存在（由系统或其他浏览器写入），本产品只保证不新增、不修改、卸载不动它们。

[PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
