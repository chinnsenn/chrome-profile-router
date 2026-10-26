#!/bin/bash
# ==============================================================================
# Chrome Profile Router 构建、打包与本地部署脚本
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME="Chrome Profile Router"
APP_TARGET="/Applications/${APP_NAME}.app"

echo "🚀 开始基于 MyGo (https://mygo.egoist.dev/) 构建原生应用..."

cd "$SCRIPT_DIR"
go run github.com/egoist/mygo/cmd/mygo build

echo "📦 部署至 /Applications..."
rm -rf "$APP_TARGET"
cp -R "build/darwin-arm64/${APP_NAME}.app" "$APP_TARGET"

# 注入专属定制的 Chrome 极光分流微标 AppIcon.icns
if [ -f "$SCRIPT_DIR/resources/AppIcon.icns" ]; then
    cp "$SCRIPT_DIR/resources/AppIcon.icns" "$APP_TARGET/Contents/Resources/AppIcon.icns"
fi

# 校验 public.html 与 public.xhtml 声明：已由 mygo.json 的 macos.infoPlist
# 生成，此处仅对缺失的 key 做幂等兜底，避免重复追加造成数组项重复。
PLIST="$APP_TARGET/Contents/Info.plist"
if ! /usr/libexec/PlistBuddy -c 'Print :CFBundleDocumentTypes:0:LSItemContentTypes' "$PLIST" >/dev/null 2>&1; then
    /usr/libexec/PlistBuddy -c 'Add :CFBundleDocumentTypes:0:LSItemContentTypes array' "$PLIST"
    /usr/libexec/PlistBuddy -c 'Add :CFBundleDocumentTypes:0:LSItemContentTypes:0 string "public.html"' "$PLIST"
    /usr/libexec/PlistBuddy -c 'Add :CFBundleDocumentTypes:0:LSItemContentTypes:1 string "public.xhtml"' "$PLIST"
fi

# 重新代码签名并向 Launch Services 注册
codesign --force --deep --sign - "$APP_TARGET"
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$APP_TARGET"

echo ""
echo "=================================================================="
echo "🎉 部署完成: $APP_TARGET"
echo "=================================================================="
echo "请前往 macOS 「系统设置」->「桌面与程序坞」->「默认网页浏览器」中选择「${APP_NAME}」即可！"
echo "=================================================================="
