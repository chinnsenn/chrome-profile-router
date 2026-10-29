@echo off
rem 提示一律用 ASCII 英文：cmd 的代码页在各环境不同，中文 echo 会乱码；
rem 中文只留在不执行的注释里。
rem ============================================================================
rem  Chrome Profile Router - Windows 用户级卸载（cmd + reg，无 PowerShell）
rem ============================================================================
rem  [INPUT]  安装目录里的应用 exe 与本脚本自身。
rem  [OUTPUT] 移除本产品自有命名空间的注册与安装文件、以及安装目录里的本脚本；用户
rem           自己放进安装目录的额外文件、以及应用设置数据，一律保留。自删除与成功
rem           退出是同步的：返回 0 时安装目录里的 uninstall.cmd 已删除（从包目录运行
rem           时包内的副本按契约保留）。
rem  [POS]    与 install.cmd 成对使用；可重复执行。
rem  [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
rem ============================================================================

setlocal EnableExtensions DisableDelayedExpansion
set "APPDIR=%LOCALAPPDATA%\Programs\Chrome Profile Router"
set "EXENAME=Chrome Profile Router.exe"
set "PROGID=ChromeProfileRouterHTML"
set "BROWSERKEY=Software\Clients\StartMenuInternet\ChromeProfileRouter"
set "CAPKEY=%BROWSERKEY%\Capabilities"

rem ---------------------------------------------------------------------------
rem 1. 只删自己安装的文件（目录里用户额外放的东西不动）
rem ---------------------------------------------------------------------------
if exist "%APPDIR%\%EXENAME%" (
    del /f /q "%APPDIR%\%EXENAME%" >nul 2>&1
    if exist "%APPDIR%\%EXENAME%" goto :fail_locked
)

rem ---------------------------------------------------------------------------
rem 2. 只删自己的命名空间；值/键本就不存在算正常（可重复执行），真失败才报错
rem ---------------------------------------------------------------------------
call :delkey "HKCU\%BROWSERKEY%"
if errorlevel 1 goto :fail_reg
call :delkey "HKCU\Software\Classes\%PROGID%"
if errorlevel 1 goto :fail_reg
call :delval "HKCU\Software\RegisteredApplications" ChromeProfileRouter
if errorlevel 1 goto :fail_reg
call :delval "HKCU\Software\Classes\.html\OpenWithProgids" %PROGID%
if errorlevel 1 goto :fail_reg
call :delval "HKCU\Software\Classes\.htm\OpenWithProgids" %PROGID%
if errorlevel 1 goto :fail_reg
call :delval "HKCU\Software\Classes\.xhtml\OpenWithProgids" %PROGID%
if errorlevel 1 goto :fail_reg

goto :success

:fail_locked
echo [error] cannot delete the application file (is it running?): %APPDIR%\%EXENAME%
echo         Quit Chrome Profile Router and run this script again.
exit /b 1

:fail_reg
echo [error] deleting the registry keys failed: HKCU\%BROWSERKEY%
exit /b 1

rem ---------------------------------------------------------------------------
rem 子过程：删键 / 删值。不存在视为成功；仍然存在才是真失败。
rem ---------------------------------------------------------------------------
:delkey
reg delete "%~1" /f >nul 2>&1
if errorlevel 1 (
    reg query "%~1" >nul 2>&1
    if not errorlevel 1 exit /b 1
)
exit /b 0

:delval
reg delete "%~1" /v "%~2" /f >nul 2>&1
if errorlevel 1 (
    reg query "%~1" /v "%~2" >nul 2>&1
    if not errorlevel 1 exit /b 1
)
exit /b 0

:success
echo.
echo Removed Chrome Profile Router program files and registry candidates.
echo App settings and any other files in the install directory were left alone.
rem 最后一条命令行一次做完三件事，顺序不能改：
rem  1) (goto) 2>nul 让 cmd 立刻停止读取本脚本——行为等同 exit /b，但本行后面的
rem     命令仍在调用者上下文里执行（整行先解析后执行，所以 %APPDIR% 已展开）。
rem     不先停读，删掉自己之后 cmd 会打印 "The batch file cannot be found." 并以 1
rem     退出，`& exit /b 0` 也救不回来——这正是 CI 上见过的那次失败（括号块同样会
rem     打印这行错误，只是把块内命令从内存里执行完）。
rem  2) 删安装目录里的卸载脚本（即使当前跑的正是它），再移除（此时应为空的）
rem     安装目录；目录里还有用户文件时 rd 失败，目录保留。从包目录运行本脚本时，
rem     包里的副本按契约保留、不删。
rem  3) 用存在性判定钉住退出码：安装目录里的 uninstall.cmd 真没了才算成功，删不掉
rem     就是 1（不静默吞掉）；rd 的失败（目录非空）不算卸载失败。
(goto) 2>nul & del /f /q "%APPDIR%\uninstall.cmd" >nul 2>&1 & rd "%APPDIR%" 2>nul & if exist "%APPDIR%\uninstall.cmd" (cmd /c exit /b 1) else (cmd /c exit /b 0)
