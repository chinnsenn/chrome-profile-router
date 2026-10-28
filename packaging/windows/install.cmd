@echo off
rem 提示一律用 ASCII 英文：cmd 的代码页在各环境不同，中文 echo 会乱码；
rem 中文只留在不执行的注释里。
rem ============================================================================
rem  Chrome Profile Router - Windows 用户级安装（cmd + reg，无 PowerShell）
rem ============================================================================
rem  [INPUT]  与脚本同目录的 "Chrome Profile Router.exe"（MyGo 产物，图标已内嵌）
rem           与 uninstall.cmd。
rem  [OUTPUT] %LOCALAPPDATA%\Programs\Chrome Profile Router 下的应用文件，
rem           以及 HKCU 中本产品自有命名空间的注册（仅候选，不抢默认）。
rem  [POS]    解压后双击安装；重复安装覆盖自身文件；exe 被占用时非 0 失败。
rem  [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
rem ============================================================================
rem
rem  只写自己的命名空间：
rem    HKCU\Software\Clients\StartMenuInternet\ChromeProfileRouter
rem    HKCU\Software\RegisteredApplications 值 ChromeProfileRouter
rem    HKCU\Software\Classes\ChromeProfileRouterHTML
rem    HKCU\Software\Classes\.html|.htm|.xhtml\OpenWithProgids 值 ChromeProfileRouterHTML
rem  绝不写 Classes 的 http/https、扩展的 (default)、任何 UserChoice，
rem  也不调用任何"设为默认"的接口。

setlocal EnableExtensions DisableDelayedExpansion
set "APPDIR=%LOCALAPPDATA%\Programs\Chrome Profile Router"
set "EXENAME=Chrome Profile Router.exe"
set "PROGID=ChromeProfileRouterHTML"
set "BROWSERKEY=Software\Clients\StartMenuInternet\ChromeProfileRouter"
set "CAPKEY=%BROWSERKEY%\Capabilities"

rem ---------------------------------------------------------------------------
rem 1. 安装包内容检查
rem ---------------------------------------------------------------------------
if not exist "%~dp0%EXENAME%" (
    echo [error] missing application executable: %~dp0%EXENAME%
    echo         Put this script next to the package contents.
    exit /b 1
)
if not exist "%~dp0uninstall.cmd" (
    echo [error] missing uninstall.cmd: %~dp0uninstall.cmd
    exit /b 1
)

rem ---------------------------------------------------------------------------
rem 2. 复制应用文件（只有 exe 与卸载脚本；运行时不依赖外部资源）
rem ---------------------------------------------------------------------------
if not exist "%APPDIR%" mkdir "%APPDIR%"
if errorlevel 1 goto :fail_mkdir

rem 目标路径若是目录（异常残留），先明确失败：绝不能把 .new 移进那个目录
if exist "%APPDIR%\%EXENAME%\" goto :fail_busy
rem 先复制成同卷 .new 再改名：新文件没准备好之前，已装的旧 exe 一直完好
copy /y "%~dp0%EXENAME%" "%APPDIR%\%EXENAME%.new" >nul
if errorlevel 1 goto :fail_new
move /y "%APPDIR%\%EXENAME%.new" "%APPDIR%\%EXENAME%" >nul
if errorlevel 1 goto :fail_busy
if exist "%APPDIR%\%EXENAME%.new" goto :fail_busy

rem 卸载脚本可以直接覆盖（它不是正在运行的那个文件，覆盖失败同样非 0）
copy /y "%~dp0uninstall.cmd" "%APPDIR%\uninstall.cmd" >nul
if errorlevel 1 goto :fail_script

set "EXE=%APPDIR%\%EXENAME%"

rem ---------------------------------------------------------------------------
rem 3. StartMenuInternet 候选（子键 DefaultIcon 用 (default)，命令只有 exe）
rem ---------------------------------------------------------------------------
reg add "HKCU\%BROWSERKEY%" /ve /t REG_SZ /d "Chrome Profile Router" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%BROWSERKEY%\DefaultIcon" /ve /t REG_SZ /d "\"%EXE%\",0" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%BROWSERKEY%\shell\open\command" /ve /t REG_SZ /d "\"%EXE%\"" /f >nul
if errorlevel 1 goto :fail_reg

rem ---------------------------------------------------------------------------
rem 4. Capabilities（ApplicationName/ApplicationDescription/ApplicationIcon）
rem ---------------------------------------------------------------------------
reg add "HKCU\%CAPKEY%" /v ApplicationName /t REG_SZ /d "Chrome Profile Router" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%" /v ApplicationDescription /t REG_SZ /d "Open links in a chosen Chrome profile" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%" /v ApplicationIcon /t REG_SZ /d "\"%EXE%\",0" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\URLAssociations" /v http /t REG_SZ /d "%PROGID%" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\URLAssociations" /v https /t REG_SZ /d "%PROGID%" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\FileAssociations" /v .html /t REG_SZ /d "%PROGID%" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\FileAssociations" /v .htm /t REG_SZ /d "%PROGID%" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\FileAssociations" /v .xhtml /t REG_SZ /d "%PROGID%" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\%CAPKEY%\Startmenu" /v StartMenuInternet /t REG_SZ /d "ChromeProfileRouter" /f >nul
if errorlevel 1 goto :fail_reg

reg add "HKCU\Software\RegisteredApplications" /v ChromeProfileRouter /t REG_SZ /d "%CAPKEY%" /f >nul
if errorlevel 1 goto :fail_reg

rem ---------------------------------------------------------------------------
rem 5. ProgID 与 OpenWithProgids（只加自己的值）
rem ---------------------------------------------------------------------------
reg add "HKCU\Software\Classes\%PROGID%" /ve /t REG_SZ /d "Chrome Profile Router HTML Document" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\Software\Classes\%PROGID%\DefaultIcon" /ve /t REG_SZ /d "\"%EXE%\",0" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\Software\Classes\%PROGID%\shell\open\command" /ve /t REG_SZ /d "\"%EXE%\" \"%%1\"" /f >nul
if errorlevel 1 goto :fail_reg

reg add "HKCU\Software\Classes\.html\OpenWithProgids" /v %PROGID% /t REG_SZ /d "" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\Software\Classes\.htm\OpenWithProgids" /v %PROGID% /t REG_SZ /d "" /f >nul
if errorlevel 1 goto :fail_reg
reg add "HKCU\Software\Classes\.xhtml\OpenWithProgids" /v %PROGID% /t REG_SZ /d "" /f >nul
if errorlevel 1 goto :fail_reg

echo.
echo Installed to: %APPDIR%
echo Registered as a Windows default-app candidate; no existing default was changed.
echo Uninstall: "%APPDIR%\uninstall.cmd" (keeps app settings).
exit /b 0

:fail_mkdir
echo [error] cannot create install directory: %APPDIR%
exit /b 1

:fail_new
rem 新文件都没复制成功：清掉自己的 .new，旧安装原样保留
del /f /q "%APPDIR%\%EXENAME%.new" >nul 2>&1
echo [error] copying the application files failed: %APPDIR%
echo         The previously installed version is untouched.
exit /b 1

:fail_busy
rem 改名失败 = 目标被占用或异常：清掉自己的 .new，旧安装原样保留
del /f /q "%APPDIR%\%EXENAME%.new" >nul 2>&1
echo [error] application file is in use (is it running?): %APPDIR%\%EXENAME%
echo         Quit Chrome Profile Router and retry; the old version is untouched.
exit /b 1

:fail_script
echo [error] copying uninstall.cmd failed: %APPDIR%
exit /b 1

:fail_reg
echo [error] writing the registry keys failed: HKCU\%BROWSERKEY%
exit /b 1
