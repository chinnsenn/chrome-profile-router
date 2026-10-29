# =============================================================================
#  Chrome Profile Router — Windows 包的 CI 冒烟（仅 CI 使用，终端用户不需要 PowerShell）
# =============================================================================
#  [INPUT]: build/windows-*/ 下由 packaging/package.sh 产出的 zip（优先本机架构）。
#  [OUTPUT]: 退出码 0 表示通过（含「卸载返回 0 时 uninstall.cmd 已同步自删」）；任何断言失败以 1 退出并打印原因。
#  [POS]:    CI 专用：在隔离的 CI 用户里实跑 install.cmd / uninstall.cmd，
#            验证候选键正确、用户默认关联未被改动、卸载只移除自己且脚本自身已消失。
#  [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
# =============================================================================
$ErrorActionPreference = 'Stop'

function Fail([string]$Message) {
    Write-Host "FAIL: $Message"
    exit 1
}

function Assert-Equal($Actual, $Expected, [string]$What) {
    if ("$Actual" -ne "$Expected") { Fail "$What = '$Actual'，期望 '$Expected'" }
}

function Assert-Key([string]$Path, [string]$What) {
    if (-not (Test-Path $Path)) { Fail "缺少注册表项: $What ($Path)" }
}

function Assert-NoKey([string]$Path, [string]$What) {
    if (Test-Path $Path) { Fail "注册表项仍存在: $What ($Path)" }
}

# 只读探针：用户默认关联的真实位置（install/uninstall 都不允许改动它们）
$probes = @(
    @{ Name = 'UrlAssociations http UserChoice'; Path = 'HKCU:\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice'; Values = @('ProgId', 'Hash') },
    @{ Name = 'UrlAssociations https UserChoice'; Path = 'HKCU:\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice'; Values = @('ProgId', 'Hash') },
    @{ Name = 'Explorer FileExts .html UserChoice'; Path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.html\UserChoice'; Values = @('ProgId', 'Hash') },
    @{ Name = 'Classes http command'; Path = 'HKCU:\Software\Classes\http\shell\open\command'; Values = @('(default)') },
    @{ Name = 'Classes https command'; Path = 'HKCU:\Software\Classes\https\shell\open\command'; Values = @('(default)') },
    @{ Name = 'Classes .html default'; Path = 'HKCU:\Software\Classes\.html'; Values = @('(default)') }
)

function Get-Snapshot {
    $snap = @{}
    foreach ($p in $probes) {
        $props = Get-ItemProperty -Path $p.Path -ErrorAction SilentlyContinue
        foreach ($v in $p.Values) {
            $snap["$($p.Name)/$v"] = if ($props) { $props.$v } else { '<absent>' }
        }
    }
    return $snap
}

function Assert-SnapshotUnchanged($Before, $After, [string]$When) {
    foreach ($k in $Before.Keys) {
        if ("$($Before[$k])" -ne "$($After[$k])") {
            Fail "$When 改动了用户默认关联: $k ('$($Before[$k])' -> '$($After[$k])')"
        }
    }
}

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$buildDir = Join-Path $repoRoot 'build'
$archSuffix = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { '-arm64.zip' } else { '-amd64.zip' }
$zips = Get-ChildItem -Path $buildDir -Recurse -Filter '*.zip' -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -match 'windows-' }
$zip = $zips | Where-Object { $_.Name.EndsWith($archSuffix) } | Select-Object -First 1
if (-not $zip) { $zip = $zips | Select-Object -First 1 }
if (-not $zip) { Fail "build/ 下没有 Windows zip（先跑 packaging/package.sh windows/amd64）" }

$work = Join-Path $env:RUNNER_TEMP ("cpr-smoke-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
Expand-Archive -Path $zip.FullName -DestinationPath $work

# 包内只应有 exe 与两个脚本（不含 Setup、不含裸目录）
$entries = Get-ChildItem -Path $work -Recurse -File | ForEach-Object { $_.Name } | Sort-Object
$expected = @('Chrome Profile Router.exe', 'install.cmd', 'uninstall.cmd') | Sort-Object
if (Compare-Object $entries $expected) {
    Fail ("zip 内容不符合预期: " + ($entries -join ', '))
}

$appDir = Join-Path $env:LOCALAPPDATA 'Programs\Chrome Profile Router'
$exe = Join-Path $appDir 'Chrome Profile Router.exe'
$browserKey = 'HKCU:\Software\Clients\StartMenuInternet\ChromeProfileRouter'
$capKey = "$browserKey\Capabilities"
$progId = 'ChromeProfileRouterHTML'

$before = Get-Snapshot

# 用户数据哨兵：卸载后必须还在
$settingsDir = Join-Path $env:APPDATA 'Chrome Profile Router'
New-Item -ItemType Directory -Path $settingsDir -Force | Out-Null
$settingsFile = Join-Path $settingsDir 'settings.json'
'{"version":2,"sentinel":"keep-me"}' | Set-Content -Path $settingsFile -Encoding utf8

# --- 安装 ---
& cmd /c (Join-Path $work 'install.cmd') | Out-Host
if ($LASTEXITCODE -ne 0) { Fail "install.cmd 退出码 $LASTEXITCODE" }
if (-not (Test-Path $exe)) { Fail "安装后没有 exe: $exe" }

# --- 候选键逐项核对 ---
Assert-Key $browserKey 'StartMenuInternet 候选'
Assert-Equal (Get-ItemProperty $browserKey).'(default)' 'Chrome Profile Router' 'StartMenuInternet (default)'
Assert-Equal (Get-ItemProperty "$browserKey\DefaultIcon").'(default)' "`"$exe`",0" 'StartMenuInternet DefaultIcon'
Assert-Equal (Get-ItemProperty "$browserKey\shell\open\command").'(default)' "`"$exe`"" 'StartMenuInternet 打开命令'
Assert-Equal (Get-ItemProperty $capKey).ApplicationName 'Chrome Profile Router' 'Capabilities.ApplicationName'
if (-not (Get-ItemProperty $capKey).ApplicationDescription) { Fail '缺少 Capabilities.ApplicationDescription' }
Assert-Equal (Get-ItemProperty $capKey).ApplicationIcon "`"$exe`",0" 'Capabilities.ApplicationIcon'
Assert-Equal (Get-ItemProperty "$capKey\URLAssociations").http $progId 'URLAssociations.http'
Assert-Equal (Get-ItemProperty "$capKey\URLAssociations").https $progId 'URLAssociations.https'
foreach ($ext in '.html', '.htm', '.xhtml') {
    Assert-Equal (Get-ItemProperty "$capKey\FileAssociations").$ext $progId "FileAssociations.$ext"
}
Assert-Equal (Get-ItemProperty "$capKey\Startmenu").StartMenuInternet 'ChromeProfileRouter' 'Capabilities.Startmenu'
Assert-Equal (Get-ItemProperty 'HKCU:\Software\RegisteredApplications').ChromeProfileRouter 'Software\Clients\StartMenuInternet\ChromeProfileRouter\Capabilities' 'RegisteredApplications'
Assert-Equal (Get-ItemProperty "HKCU:\Software\Classes\$progId").'(default)' 'Chrome Profile Router HTML Document' 'ProgID (default)'
Assert-Equal (Get-ItemProperty "HKCU:\Software\Classes\$progId\DefaultIcon").'(default)' "`"$exe`",0" 'ProgID DefaultIcon'
Assert-Equal (Get-ItemProperty "HKCU:\Software\Classes\$progId\shell\open\command").'(default)' "`"$exe`" `"%1`"" 'ProgID 打开命令'
foreach ($ext in '.html', '.htm', '.xhtml') {
    $props = Get-ItemProperty "HKCU:\Software\Classes\$ext\OpenWithProgids" -ErrorAction SilentlyContinue
    if (-not $props -or $null -eq $props.$progId) { Fail "OpenWithProgids.$ext 缺少 $progId" }
}

# --- 不抢默认：真实 UserChoice 位置与共享键必须原样（只读快照对比） ---
Assert-SnapshotUnchanged $before (Get-Snapshot) '安装'

# 我们从不写 Classes\http 下的任何东西
$httpTree = & reg query 'HKCU\Software\Classes\http' /s 2>$null
if ($httpTree -match 'ChromeProfileRouter') { Fail 'Classes\http 下出现了本产品的键或值' }

# --- 重复安装应成功 ---
& cmd /c (Join-Path $work 'install.cmd') | Out-Null
if ($LASTEXITCODE -ne 0) { Fail "重复安装退出码 $LASTEXITCODE" }

# --- 卸载：保留用户文件与设置 ---
'keep me' | Set-Content -Path (Join-Path $appDir 'user-notes.txt') -Encoding utf8
& cmd /c (Join-Path $appDir 'uninstall.cmd') | Out-Host
if ($LASTEXITCODE -ne 0) { Fail "uninstall.cmd 退出码 $LASTEXITCODE" }
# 自删除必须与退出码同步完成：返回 0 时脚本自己就该已经不在了，不靠等待掩盖时序
if (Test-Path (Join-Path $appDir 'uninstall.cmd')) { Fail "卸载后卸载脚本仍在: $appDir\uninstall.cmd" }

if (Test-Path $exe) { Fail "卸载后 exe 仍在: $exe" }
Assert-NoKey $browserKey 'StartMenuInternet 候选'
Assert-NoKey "HKCU:\Software\Classes\$progId" 'ProgID'
if ((Get-ItemProperty 'HKCU:\Software\RegisteredApplications' -ErrorAction SilentlyContinue).ChromeProfileRouter) {
    Fail 'RegisteredApplications 值仍在'
}
foreach ($ext in '.html', '.htm', '.xhtml') {
    $props = Get-ItemProperty "HKCU:\Software\Classes\$ext\OpenWithProgids" -ErrorAction SilentlyContinue
    if ($props -and $null -ne $props.$progId) { Fail "OpenWithProgids.$ext 里的 $progId 仍在" }
}
if (-not (Test-Path (Join-Path $appDir 'user-notes.txt'))) { Fail '卸载删掉了用户放在安装目录里的文件' }
if (-not (Test-Path $settingsFile)) { Fail '卸载删掉了应用设置数据' }

Assert-SnapshotUnchanged $before (Get-Snapshot) '卸载'

Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
Write-Host 'Windows 包冒烟通过：安装候选正确、未抢默认、卸载只移除自己。'
