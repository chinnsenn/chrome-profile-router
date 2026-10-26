#!/bin/bash
# ==============================================================================
# Chrome Profile Router — 构建与本地安装
# ==============================================================================
# [INPUT]: 读取 mygo.json、resources/ 与 Go 源码，无参数或交互。
# [OUTPUT]: /Applications/Chrome Profile Router.app 与对应架构的 .app / DMG 构建产物。
# [POS]: 本地构建安装入口，在目标卷暂存和校验新包，替换失败时恢复旧版本。
# [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
# ==============================================================================

set -euo pipefail

APP_NAME="Chrome Profile Router"
APP_TARGET="/Applications/${APP_NAME}.app"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

cd "$(dirname "${BASH_SOURCE[0]}")"

fail() { printf '错误: %s\n' "$*" >&2; exit 1; }
note() { printf '%s\n' "$*"; }

# ------------------------------------------------------------------------------
# 架构只推导一次：后续构建与产物路径都用同一个 GOARCH
# ------------------------------------------------------------------------------
[ "$(uname -s)" = "Darwin" ] || fail "仅支持 macOS 主机"

case "$(uname -m)" in
    arm64) GOARCH="arm64" ;;
    # Rosetta 下 uname -m 会报 x86_64，hw.optional.arm64 才反映真实硬件
    x86_64)
        if [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = "1" ]; then
            GOARCH="arm64"
        else
            GOARCH="amd64"
        fi
        ;;
    *) fail "不支持的 CPU 架构: $(uname -m)（仅支持 arm64 / amd64）" ;;
esac
readonly GOARCH
# -platform 用 GOOS/GOARCH，而 mygo 的产物目录用 GOOS-GOARCH
readonly PLATFORM="darwin/${GOARCH}"
readonly SOURCE_DIR="build/darwin-${GOARCH}"
readonly SOURCE_APP="${SOURCE_DIR}/${APP_NAME}.app"

[ -d /Applications ] || fail "安装目录不存在: /Applications"
[ -w /Applications ] || fail "没有写入 /Applications 的权限"

# ------------------------------------------------------------------------------
# 安装事务：旧版本仅在暂存版本完全就绪后才会被移开，任何失败都回滚
#   init → 未创建暂存目录；staged → 新版本已就绪；backedup → 旧版本已移入暂存；
#   installed → 新版本已就位、尚未注册；done → 已注册，提交
# ------------------------------------------------------------------------------
STAGE=""
BACKUP=""
STATE="init"

cleanup() {
    status=$?
    trap - EXIT INT TERM HUP
    set +e
    if [ "$status" -ne 0 ]; then
        if [ "$STATE" = "installed" ] && [ -e "$APP_TARGET" ]; then
            rm -rf "$APP_TARGET"
        fi
        if [ -d "$BACKUP" ] && [ ! -e "$APP_TARGET" ]; then
            mv "$BACKUP" "$APP_TARGET"
            if [ -e "$APP_TARGET" ]; then
                "$LSREGISTER" -f "$APP_TARGET" >/dev/null 2>&1
                printf '已恢复原版本: %s\n' "$APP_TARGET" >&2
            fi
        fi
        if [ -d "$BACKUP" ] && [ -e "$BACKUP" ]; then
            # 回滚未完成：暂存目录里是唯一一份旧版本，绝不删除
            printf '回滚未完成，旧版本保留在: %s\n' "$BACKUP" >&2
            exit "$status"
        fi
    fi
    if [ -n "$STAGE" ] && [ -d "$STAGE" ]; then
        rm -rf "$STAGE"
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

# ------------------------------------------------------------------------------
# 1. 构建：显式指定平台，避免默认跟随宿主工具链架构
# ------------------------------------------------------------------------------
note "构建 ${PLATFORM} ..."
go run github.com/egoist/mygo/cmd/mygo build -platform "$PLATFORM"

# ------------------------------------------------------------------------------
# 2. 先校验构建产物，任何问题都不触碰已安装版本
# ------------------------------------------------------------------------------
PLIST="$SOURCE_APP/Contents/Info.plist"
[ -f "$PLIST" ] || fail "构建产物缺少 Info.plist: $SOURCE_APP"
EXECUTABLE="$(plutil -extract CFBundleExecutable raw -o - "$PLIST" 2>/dev/null || true)"
[ -n "$EXECUTABLE" ] || fail "Info.plist 缺少 CFBundleExecutable: $PLIST"
[ -x "$SOURCE_APP/Contents/MacOS/$EXECUTABLE" ] ||
    fail "构建产物缺少可执行文件: $SOURCE_APP/Contents/MacOS/$EXECUTABLE"

# ------------------------------------------------------------------------------
# 3. 在目标文件系统上暂存（mktemp 保证同一卷，rename 才是原子的）
# ------------------------------------------------------------------------------
STAGE="$(mktemp -d "/Applications/.${APP_NAME}.install.XXXXXX")" ||
    fail "无法创建暂存目录"
STAGED_APP="$STAGE/${APP_NAME}.app"
BACKUP="$STAGE/${APP_NAME}.app.previous"

ditto "$SOURCE_APP" "$STAGED_APP" || fail "复制构建产物到暂存目录失败"
codesign --verify --deep --strict "$STAGED_APP" >/dev/null 2>&1 ||
    fail "暂存版本的代码签名校验失败: $STAGED_APP"
STATE="staged"

# ------------------------------------------------------------------------------
# 4. 原子替换：旧版本先移入暂存目录，新版本再就位
# ------------------------------------------------------------------------------
STATE="backedup"
if [ -e "$APP_TARGET" ] || [ -L "$APP_TARGET" ]; then
    mv "$APP_TARGET" "$BACKUP" || fail "无法移开旧版本: $APP_TARGET"
fi
STATE="installed"
mv "$STAGED_APP" "$APP_TARGET" || fail "无法把新版本移动到 $APP_TARGET"

# ------------------------------------------------------------------------------
# 5. 注册（最后一步，失败即回滚）
# ------------------------------------------------------------------------------
"$LSREGISTER" -f "$APP_TARGET" || fail "Launch Services 注册失败: $APP_TARGET"
STATE="done"

note ""
note "部署完成: $APP_TARGET"
note "架构: $PLATFORM"
note "构建产物: $SOURCE_DIR"
note "请前往 macOS「系统设置」→「桌面与程序坞」→「默认网页浏览器」中选择「${APP_NAME}」。"
