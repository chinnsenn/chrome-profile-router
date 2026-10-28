#!/bin/bash
# ==============================================================================
# Chrome Profile Router — 跨平台打包入口
# ==============================================================================
# [INPUT]: 逗号分隔的平台列表（linux/amd64,linux/arm64,windows/amd64,
#          windows/arm64；可只给一个），读取 mygo.json 与 packaging/windows/。
# [OUTPUT]: build/<goos>-<goarch>/ 下 MyGo 的产物，外加 Windows 的
#          <slug>-<version>-windows-<arch>.zip（exe + install.cmd + uninstall.cmd）；
#          Linux 复用 MyGo 的 .deb / .tar.gz / install.sh / .desktop。
# [POS]: CI 与本地共用的打包入口；macOS 由 build.sh 负责，这里不碰。
# [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
# ==============================================================================
#
# 只调用 MyGo 官方 build（scheme 与文件关联由 mygo.json 注入），不修改依赖源。
# Windows 绝不分发 MyGo 的 NSIS "Setup" 安装器：它会写 HKCU Classes 的
# http/https，抢默认浏览器；构建后丢弃，也从不进 zip。
# 需要 Go + bash + 压缩工具（zip，或 Windows 上的 PowerShell Compress-Archive）。

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

fail() { printf '错误: %s\n' "$*" >&2; exit 1; }
note() { printf '%s\n' "$*"; }

case "${1:-}" in
    -h | --help)
        cat <<'USAGE'
用法: packaging/package.sh [平台列表]

  平台列表  逗号分隔：linux/amd64、linux/arm64、windows/amd64、windows/arm64
            省略时打全部四个。
USAGE
        exit 0
        ;;
esac

PLATFORMS="${1:-linux/amd64,linux/arm64,windows/amd64,windows/arm64}"
[ $# -le 1 ] || fail "只接受一个平台列表参数（见 --help）"

# 平台白名单：只打能验证的组合，顺带判断是否需要压缩工具
NEED_ZIP=0
PLATFORM_LIST=()
OLD_IFS="$IFS"
IFS=','
for p in $PLATFORMS; do
    case "$p" in
        linux/amd64 | linux/arm64) ;;
        windows/amd64 | windows/arm64) NEED_ZIP=1 ;;
        *) fail "不支持的平台: ${p}（只支持 linux/amd64、linux/arm64、windows/amd64、windows/arm64）" ;;
    esac
    PLATFORM_LIST+=("$p")
done
IFS="$OLD_IFS"
[ ${#PLATFORM_LIST[@]} -gt 0 ] || fail "平台列表为空"

command -v go >/dev/null 2>&1 || fail "缺少 Go 工具链（go）：先安装 Go 1.27+"

ZIP_MODE=""
if [ "$NEED_ZIP" = "1" ]; then
    if command -v zip >/dev/null 2>&1; then
        ZIP_MODE="zip"
    elif command -v powershell.exe >/dev/null 2>&1; then
        ZIP_MODE="powershell.exe"
    elif command -v pwsh >/dev/null 2>&1; then
        ZIP_MODE="pwsh"
    else
        fail "打包 Windows 需要压缩工具：安装 zip，或在 Windows 上提供 PowerShell（Compress-Archive）"
    fi
fi

# mygo.json 是手写的固定格式，这里只取顶层 name/version/out 三个字符串字段
json_field() {
    sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" mygo.json | head -1
}
APP_NAME="$(json_field name)"
APP_VERSION="$(json_field version)"
OUT_DIR="$(json_field out)"
[ -n "$APP_NAME" ] || fail "mygo.json 缺少 name"
[ -n "$APP_VERSION" ] || fail "mygo.json 缺少 version"
[ -n "$OUT_DIR" ] || OUT_DIR="dist"
SLUG="$(printf '%s' "$APP_NAME" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')"

note "打包 ${APP_NAME} ${APP_VERSION}: ${PLATFORMS}"

# 1. MyGo 官方 build，一次性构建全部目标
go run github.com/egoist/mygo/cmd/mygo build -skip-dmg -platform "$PLATFORMS" ||
    fail "mygo build 失败（平台: ${PLATFORMS}）"

ARTIFACTS=()

# 2. Windows：丢掉 NSIS Setup，只打 exe + install.cmd + uninstall.cmd
package_windows() {
    local arch="$1"
    local dir="${OUT_DIR}/windows-${arch}"
    local exe="${dir}/${APP_NAME}.exe"
    [ -d "$dir" ] || fail "缺少构建目录: $dir"
    [ -f "$exe" ] || fail "缺少 Windows 可执行文件: $exe"

    rm -f "$dir/${APP_NAME} Setup "*.exe

    local stage target
    stage="$(mktemp -d "${TMPDIR:-/tmp}/cpr-winzip.XXXXXX")"
    local target="${dir}/${SLUG}-${APP_VERSION}-windows-${arch}.zip"
    # zip 在 stage 里运行，传给压缩工具的必须是绝对路径
    local target_abs
    target_abs="$(cd "$dir" && pwd)/$(basename "$target")"
    cp "$exe" "${stage}/${APP_NAME}.exe" || fail "复制 exe 失败"
    cp packaging/windows/install.cmd "${stage}/install.cmd" || fail "复制 install.cmd 失败"
    cp packaging/windows/uninstall.cmd "${stage}/uninstall.cmd" || fail "复制 uninstall.cmd 失败"

    rm -f "$target"
    case "$ZIP_MODE" in
        zip)
            ( cd "$stage" && zip -q -r -X "$target_abs" . ) || fail "zip 压缩失败: $target"
            ;;
        powershell.exe | pwsh)
            # Windows 的 PowerShell 不认 MSYS 路径（/tmp/...）：先转成盘符路径。
            # 路径只经环境变量传递，不拼进 PowerShell 源码。
            local ps_stage="$stage" ps_target="$target_abs"
            case "$(uname -s)" in
                MINGW* | MSYS* | CYGWIN*)
                    command -v cygpath >/dev/null 2>&1 ||
                        fail "Windows 上需要 cygpath（Git Bash 自带）来转换暂存路径"
                    ps_stage="$(cygpath -w "$stage")" || fail "cygpath 转换暂存目录失败: $stage"
                    ps_target="$(cygpath -w "$target_abs")" || fail "cygpath 转换目标路径失败: $target_abs"
                    ;;
            esac
            CPR_STAGE="$ps_stage" CPR_TARGET="$ps_target" "$ZIP_MODE" -NoProfile -NonInteractive \
                -Command 'Compress-Archive -Path (Join-Path $env:CPR_STAGE "*") -DestinationPath $env:CPR_TARGET -Force' ||
                fail "PowerShell 压缩失败: $target"
            ;;
    esac
    rm -rf "$stage"
    [ -f "$target" ] || fail "压缩后没有生成: $target"
    ARTIFACTS+=("$target")
}

# 3. Linux：复用 MyGo 的 .deb / install.sh / .desktop，但归档要自己补 install.sh
package_linux() {
    local arch="$1"
    local dir="${OUT_DIR}/linux-${arch}"
    local archive="${dir}/${SLUG}-${APP_VERSION}-linux-${arch}.tar.gz"
    [ -d "$dir" ] || fail "缺少构建目录: $dir"
    [ -f "${dir}/${SLUG}" ] || fail "缺少 Linux 可执行文件: ${dir}/${SLUG}"
    [ -f "$archive" ] || fail "缺少 Linux 归档: $archive"
    [ -x "${dir}/install.sh" ] || fail "缺少可执行的 install.sh: ${dir}/install.sh"

    # MyGo 的归档只有应用文件，官方 install.sh 在构建目录里单独生成；发布的是
    # 「解包后 ./install.sh」这一条命令，所以把同一份脚本原样放进归档再重打。
    # 不改 .desktop / .deb / install.sh 的内容，也不自造第二套安装器。
    local stage tmp staged
    stage="$(mktemp -d "${TMPDIR:-/tmp}/cpr-linux-${arch}.XXXXXX")"
    tmp="$(dirname "$(cd "$dir" && pwd)")/.${SLUG}-linux-${arch}.tar.gz.new"
    tar -xzf "$archive" -C "$stage" || { rm -rf "$stage"; fail "解包归档失败: $archive"; }
    chmod 755 "$stage" || { rm -rf "$stage"; fail "无法设置暂存目录权限"; }
    cp -p "${dir}/install.sh" "${stage}/install.sh" || { rm -rf "$stage"; fail "复制 install.sh 失败"; }
    tar -czpf "$tmp" -C "$stage" . || { rm -rf "$stage"; fail "重打归档失败: $archive"; }
    rm -rf "$stage"
    mv "$tmp" "$archive" || fail "替换归档失败: $archive"
    staged="$(tar -tzf "$archive" | grep -cE '(^|/)install\.sh$')"
    [ "$staged" = "1" ] || fail "归档里没有 install.sh: $archive"

    ARTIFACTS+=("$archive" "${dir}/install.sh" "${dir}/${SLUG}.desktop")
    local deb
    for deb in "${dir}/${SLUG}"_*.deb; do
        [ -f "$deb" ] || fail "缺少 Linux 包: ${dir}/${SLUG}_*.deb"
        ARTIFACTS+=("$deb")
    done
}

for p in "${PLATFORM_LIST[@]}"; do
    case "$p" in
        windows/amd64) package_windows amd64 ;;
        windows/arm64) package_windows arm64 ;;
        linux/amd64) package_linux amd64 ;;
        linux/arm64) package_linux arm64 ;;
    esac
done

note ""
note "== 分发产物 =="
for a in "${ARTIFACTS[@]}"; do
    note "$a"
done
note ""
note "Windows：解压 zip，双击 install.cmd（用户级，无需 UAC，不抢默认浏览器）。"
note "Windows 卸载：运行安装目录里的 uninstall.cmd（保留应用设置数据）。"
note "Linux：.deb 或 install.sh 二选一；.tar.gz 供 install.sh 就地安装。"
