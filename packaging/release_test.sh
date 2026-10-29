#!/bin/bash
# ==============================================================================
# Chrome Profile Router — 发布策略的离线回归
# ==============================================================================
# [INPUT]: mygo.json（复制成临时副本并改 version 来造标签场景）、假的 gh（PATH
#          前置的 stub，只记录调用，不联网）。
# [OUTPUT]: 每条断言的 ok / FAIL；任何一条失败退出码非 0。不打包、不联网。
# [POS]: CI 的 version job 与本地都跑它，锁住标签规则、资产名与重复发布行为。
# [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
# ==============================================================================
#
# 断言的是策略边界，不是脚本内部实现：标签 v<SemVer> 与 mygo.json 逐字一致、
# 不支持 build metadata、分支/PR 不发布、资产正好 8 个且名字含版本、SHA256SUMS
# 只写 basename；Release 不存在时 draft 起手，已有 draft 幂等续传，已公开的拒绝覆盖。

set -uo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

FAILS=0
ok() { printf 'ok   %s\n' "$*"; }
bad() {
    printf 'FAIL %s\n' "$*" >&2
    FAILS=$((FAILS + 1))
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/cpr-release-test.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

# ---- fixtures ---------------------------------------------------------------

manifest_with_version() {
    local version="$1"
    local dest="$WORK/mygo-$version.json"
    sed "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" mygo.json >"$dest"
    grep -qF "\"version\": \"$version\"" "$dest" ||
        bad "无法造出 version=$version 的 manifest（mygo.json 结构变了？）"
    printf '%s' "$dest"
}

# 期望资产名按版本写死：这是对外契约，不能由 release.sh 自己算出来再自己比对。
# DMG 是 staging 的公开名（空格换连字符），不是 MyGo 本地那个带空格的名字。
expected_names() {
    case "$1" in
        0.1.0)
            printf '%s\n' \
                "Chrome-Profile-Router-0.1.0-arm64.dmg" \
                "Chrome-Profile-Router-0.1.0-amd64.dmg" \
                "chrome-profile-router_0.1.0_amd64.deb" \
                "chrome-profile-router_0.1.0_arm64.deb" \
                "chrome-profile-router-0.1.0-linux-amd64.tar.gz" \
                "chrome-profile-router-0.1.0-linux-arm64.tar.gz" \
                "chrome-profile-router-0.1.0-windows-amd64.zip" \
                "chrome-profile-router-0.1.0-windows-arm64.zip"
            ;;
        0.1.0-rc.1)
            printf '%s\n' \
                "Chrome-Profile-Router-0.1.0-rc.1-arm64.dmg" \
                "Chrome-Profile-Router-0.1.0-rc.1-amd64.dmg" \
                "chrome-profile-router_0.1.0~rc.1_amd64.deb" \
                "chrome-profile-router_0.1.0~rc.1_arm64.deb" \
                "chrome-profile-router-0.1.0-rc.1-linux-amd64.tar.gz" \
                "chrome-profile-router-0.1.0-rc.1-linux-arm64.tar.gz" \
                "chrome-profile-router-0.1.0-rc.1-windows-amd64.zip" \
                "chrome-profile-router-0.1.0-rc.1-windows-arm64.zip"
            ;;
        *)
            bad "测试里没有 $1 的期望资产名"
            return 1
            ;;
    esac
}

make_assets() {
    local dir="$1" version="$2" name
    mkdir -p "$dir"
    while IFS= read -r name; do
        printf 'dummy payload for %s\n' "$name" >"$dir/$name"
    done < <(expected_names "$version")
}

release() { # manifest args...
    local manifest="$1"
    shift
    RELEASE_MANIFEST="$manifest" bash packaging/release.sh "$@"
}

# ---- 标签与版本 -------------------------------------------------------------

check_tag_ok() { # desc manifest tag expected-output
    local desc="$1" manifest="$2" tag="$3" want="$4" out err
    err="$WORK/err"
    if out="$(release "$manifest" tag-check "$tag" 2>"$err")"; then
        if [ "$out" = "$want" ]; then
            ok "$desc"
        else
            bad "${desc}：输出不符，得到 [$out]"
        fi
    else
        bad "${desc}：应成功却失败（$(head -1 "$err")）"
    fi
}

check_tag_rejected() { # desc manifest tag
    local desc="$1" manifest="$2" tag="$3" out err
    err="$WORK/err"
    if out="$(release "$manifest" tag-check "$tag" 2>"$err")"; then
        bad "${desc}：应失败却成功，输出 [$out]"
    else
        ok "${desc}（$(head -1 "$err")）"
    fi
}

MANIFEST="$(manifest_with_version 0.1.0)"
RC_MANIFEST="$(manifest_with_version 0.1.0-rc.1)"

check_tag_ok "v0.1.0 与 manifest 一致 → 发布、非预发布" "$MANIFEST" v0.1.0 \
    "$(printf 'version=0.1.0\ntag=v0.1.0\nprerelease=false\npublish=true')"
check_tag_ok "分支 / PR（无标签）只校验、不发布" "$MANIFEST" "" \
    "$(printf 'version=0.1.0\ntag=\nprerelease=false\npublish=false')"
check_tag_ok "预发布标签 v0.1.0-rc.1 搭配相同 manifest" "$RC_MANIFEST" v0.1.0-rc.1 \
    "$(printf 'version=0.1.0-rc.1\ntag=v0.1.0-rc.1\nprerelease=true\npublish=true')"

check_tag_rejected "v0.1.1 与 manifest 0.1.0 不一致" "$MANIFEST" v0.1.1
check_tag_rejected "v1.0.0 与 manifest 不一致" "$MANIFEST" v1.0.0
check_tag_rejected "缺 v 前缀的 0.1.0" "$MANIFEST" 0.1.0
check_tag_rejected "不完整版本 v0.1" "$MANIFEST" v0.1
check_tag_rejected "前导零 v01.0.0" "$MANIFEST" v01.0.0
check_tag_rejected "前导零预发布 v0.1.0-rc.01" "$MANIFEST" v0.1.0-rc.01
check_tag_rejected "build metadata v0.1.0+build.1 不受支持" "$MANIFEST" v0.1.0+build.1
check_tag_rejected "空的预发布 v0.1.0-" "$MANIFEST" v0.1.0-
check_tag_rejected "空标识符 v0.1.0-rc..1" "$MANIFEST" v0.1.0-rc..1
check_tag_rejected "manifest 版本本身不合法（1.0）" "$(manifest_with_version 1.0)" v1.0

# ---- 资产清单 ---------------------------------------------------------------

DIST="$WORK/dist"
make_assets "$DIST" 0.1.0

check_assets_ok() { # desc dir version [manifest]
    local desc="$1" dir="$2" version="$3" manifest="${4:-$MANIFEST}" out err
    err="$WORK/err"
    if out="$(release "$manifest" assets "$dir" "$version" 2>"$err")"; then
        if [ "$out" = "$(expected_names "$version")" ]; then
            ok "$desc"
        else
            bad "${desc}：清单不符 [$out]"
        fi
    else
        bad "${desc}：应成功却失败（$(head -1 "$err")）"
    fi
}

check_assets_rejected() { # desc dir version
    local desc="$1" dir="$2" version="$3" out err
    err="$WORK/err"
    if out="$(release "$MANIFEST" assets "$dir" "$version" 2>"$err")"; then
        bad "${desc}：应失败却成功"
    else
        ok "${desc}（$(tail -1 "$err")）"
    fi
}

check_assets_ok "8 个资产齐 → 清单与期望一致" "$DIST" 0.1.0

cp -a "$DIST" "$WORK/dist-missing"
rm -f "$WORK/dist-missing/Chrome-Profile-Router-0.1.0-amd64.dmg"
check_assets_rejected "缺一个 DMG → 失败" "$WORK/dist-missing" 0.1.0

cp -a "$DIST" "$WORK/dist-empty"
: >"$WORK/dist-empty/chrome-profile-router_0.1.0_arm64.deb"
check_assets_rejected "0 字节资产 → 失败" "$WORK/dist-empty" 0.1.0

cp -a "$DIST" "$WORK/dist-extra"
printf 'stray\n' >"$WORK/dist-extra/Chrome-Profile-Router-1.0.0-arm64.dmg"
check_assets_rejected "混入旧版本资产 → 失败" "$WORK/dist-extra" 0.1.0

RC_DIST="$WORK/dist-rc"
make_assets "$RC_DIST" 0.1.0-rc.1
check_assets_ok "预发布资产的 .deb 用 ~rc.1 命名" "$RC_DIST" 0.1.0-rc.1 "$RC_MANIFEST"  # manifest 见下

# ---- SHA256SUMS -------------------------------------------------------------

SUM_DIST="$WORK/dist-sums"
cp -a "$DIST" "$SUM_DIST"
if release "$MANIFEST" sums "$SUM_DIST" 0.1.0 >"$WORK/sums.log" 2>&1; then
    ok "sums 生成并自校验通过（DMG 用 staging 规范名）"
else
    bad "sums 失败：$(tail -1 "$WORK/sums.log")"
fi

SUMS="$SUM_DIST/SHA256SUMS"
[ -f "$SUMS" ] && [ "$(grep -c . "$SUMS")" = "8" ] &&
    ok "SHA256SUMS 是 8 行（不含自己）" || bad "SHA256SUMS 行数不是 8"
grep -qE '^[0-9a-f]{64}  Chrome-Profile-Router-0\.1\.0-amd64\.dmg$' "$SUMS" &&
    ok "SHA256SUMS 用 basename + 两个空格（DMG 用远端公开名，无空格）" ||
    bad "SHA256SUMS 的 DMG 行格式不对"
grep -q 'dist/' "$SUMS" && bad "SHA256SUMS 里出现了目录前缀" || ok "SHA256SUMS 没有目录前缀"

cp -a "$SUM_DIST" "$WORK/dist-tampered"
printf 'tampered\n' >>"$WORK/dist-tampered/chrome-profile-router-0.1.0-linux-amd64.tar.gz"
if (
    cd "$WORK/dist-tampered"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c SHA256SUMS
    else
        shasum -a 256 -c SHA256SUMS
    fi
) >"$WORK/tamper.log" 2>&1; then
    bad "被改动的资产没有让 SHA256SUMS 校验失败"
else
    ok "被改动的资产让 SHA256SUMS 校验失败"
fi

# ---- 发布（假 gh，验证 draft 起手与重复运行行为） -----------------------------

mkdir -p "$WORK/bin"
cat >"$WORK/bin/gh" <<'STUB'
#!/bin/bash
printf 'CALL %s\n' "$*" >>"$GH_LOG"
for a in "$@"; do printf 'ARG %s\n' "$a" >>"$GH_LOG"; done
case "$1" in
    release)
        case "$2" in
            view)
                case "$*" in
                    *isDraft*)
                        # 状态查询：gh release view 自己带 draft 的 GraphQL 兜底
                        case "${GH_STATE:-missing}" in
                            missing) echo "release not found" >&2; exit 1 ;;
                            draft) printf 'true\t0\n' ;;
                            published) printf 'false\t9\n' ;;
                            auth) echo "gh: Bad credentials (HTTP 401)" >&2; exit 1 ;;
                        esac
                        ;;
                    *)
                        # 远端资产清单：默认与本地一致；GH_DROP 用来模拟少传了一个。
                        # GitHub 上传 release asset 时会把文件名里的空格规范化成 '.'（真实事故：
                        # "Chrome Profile Router 0.1.1 arm64.dmg" → "Chrome.Profile.Router.0.1.1.arm64.dmg"），
                        # 这里如实模拟，清单比对因此不能假设远端会原样保留空格。
                        for f in "$GH_DIST"/*; do
                            n="$(basename "$f")"
                            [ "$n" = "${GH_DROP:-}" ] && continue
                            n="${n// /.}"
                            printf '%s\t%s\n' "$(wc -c <"$f" | tr -d ' ')" "$n"
                        done
                        ;;
                esac
                ;;
        esac
        ;;
esac
exit 0
STUB
chmod +x "$WORK/bin/gh"

publish_run() { # manifest state tag dir version prerelease [drop]
    local manifest="$1" state="$2" tag="$3" dir="$4" version="$5" pre="$6" drop="${7:-}"
    GH_LOG="$WORK/gh.log"
    : >"$GH_LOG"
    PATH="$WORK/bin:$PATH" GH_LOG="$GH_LOG" GH_STATE="$state" GH_DIST="$dir" \
        GH_DROP="$drop" TAG="$tag" PRERELEASE="$pre" GH_TOKEN=fake \
        RELEASE_MANIFEST="$manifest" bash packaging/release.sh publish "$dir" "$version"
}

dump_on_fail() { sed -e 's/^/  gh> /' "$WORK/gh.log" >&2; }

PUBLISH_DIST="$WORK/dist-publish"
cp -a "$DIST" "$PUBLISH_DIST"

# 1. 新标签：draft 起手 → 上传 → 核对远端 → 公开
if publish_run "$MANIFEST" missing v0.1.0 "$PUBLISH_DIST" 0.1.0 "" >"$WORK/publish.log" 2>&1; then
    ok "新标签发布成功"
else
    bad "新标签发布失败：$(tail -1 "$WORK/publish.log")"
    dump_on_fail
fi
grep -q '^CALL release create v0.1.0 --draft ' "$WORK/gh.log" &&
    ok "先建 draft（--draft）" || bad "没有以 draft 起手"
grep -q -- '^ARG --verify-tag$' "$WORK/gh.log" &&
    ok "create 带 --verify-tag（不自动造标签）" || bad "create 缺少 --verify-tag"
grep -q -- '^ARG --generate-notes$' "$WORK/gh.log" &&
    ok "create 带 --generate-notes" || bad "create 缺少 --generate-notes"
grep -q '^CALL release upload v0.1.0 ' "$WORK/gh.log" &&
    [ "$(grep -c "^ARG $PUBLISH_DIST/" "$WORK/gh.log")" = "9" ] &&
    ok "draft 里传满 9 个文件（8 资产 + SHA256SUMS）" ||
    bad "上传文件数不是 9：$(grep -c "^ARG $PUBLISH_DIST/" "$WORK/gh.log")"
[ "$(grep -c '^CALL release view v0.1.0 ' "$WORK/gh.log")" = "2" ] &&
    ok "先查状态、公开前再核对远端资产清单（两次 view）" || bad "没有核对远端资产"
grep -q '^CALL release edit v0.1.0 --draft=false ' "$WORK/gh.log" &&
    ok "核对后才 edit --draft=false 公开" || bad "没有公开 draft"
grep -q -- '^ARG --latest=false$' "$WORK/gh.log" &&
    bad "稳定版不应带 --latest=false" || ok "稳定版不带 --latest=false（按 GitHub 默认成为 latest）"

# 2. 已有 draft：幂等续传后公开
if publish_run "$MANIFEST" draft v0.1.0 "$PUBLISH_DIST" 0.1.0 "" >"$WORK/publish.log" 2>&1; then
    ok "已有 draft 幂等续传"
else
    bad "已有 draft 续传失败：$(tail -1 "$WORK/publish.log")"
    dump_on_fail
fi
grep -q '^CALL release upload v0.1.0 ' "$WORK/gh.log" &&
    grep -q -- '^ARG --clobber$' "$WORK/gh.log" &&
    ok "draft 走 release upload --clobber" || bad "draft 没有 --clobber 上传"
grep -q '^CALL release create' "$WORK/gh.log" &&
    bad "已有 draft 被重建" || ok "已有 draft 不重建"
grep -q '^CALL release edit v0.1.0 --draft=false ' "$WORK/gh.log" &&
    ok "draft 续传后公开" || bad "draft 没有公开"

# 3. 已公开的 Release：明确拒绝覆盖，且不发送任何写请求
if publish_run "$MANIFEST" published v0.1.0 "$PUBLISH_DIST" 0.1.0 "" >"$WORK/publish.log" 2>&1; then
    bad "已公开的 Release 被覆盖成功"
else
    ok "已公开的 Release 拒绝覆盖（$(tail -1 "$WORK/publish.log")）"
fi
if grep -qE '^CALL release (create|upload|edit)' "$WORK/gh.log"; then
    bad "拒绝覆盖前仍发出了写请求"
else
    ok "拒绝覆盖时只有查询调用"
fi

# 4. 查询失败不是 not-found（auth/网络）：原样失败，不盲发创建
if publish_run "$MANIFEST" auth v0.1.0 "$PUBLISH_DIST" 0.1.0 "" >"$WORK/publish.log" 2>&1; then
    bad "HTTP 401 must not create a Release（$(tail -1 "$WORK/publish.log")）"
else
    ok "HTTP 401 fails without creating a Release（$(tail -1 "$WORK/publish.log")）"
fi
grep -q '^CALL release create' "$WORK/gh.log" &&
    bad "查询失败后仍执行了 release create" || ok "查询失败后没有 release create"

# 5. 远端少一个资产：不公开
if publish_run "$MANIFEST" draft v0.1.0 "$PUBLISH_DIST" 0.1.0 "" \
    "chrome-profile-router-0.1.0-windows-arm64.zip" >"$WORK/publish.log" 2>&1; then
    bad "远端缺资产仍然公开"
else
    ok "远端缺资产时拒绝公开"
fi
grep -q '^CALL release edit' "$WORK/gh.log" &&
    bad "核对失败后仍公开了 draft" || ok "核对失败后没有公开"

# 6. 预发布：--prerelease --latest=false；env 说 false 必须失败
RC_PUBLISH_DIST="$WORK/dist-rc-publish"
make_assets "$RC_PUBLISH_DIST" 0.1.0-rc.1
if publish_run "$RC_MANIFEST" missing v0.1.0-rc.1 "$RC_PUBLISH_DIST" 0.1.0-rc.1 "" \
    >"$WORK/publish.log" 2>&1; then
    ok "预发布发布成功（PRERELEASE 未设置时由版本决定）"
else
    bad "预发布发布失败：$(tail -1 "$WORK/publish.log")"
    dump_on_fail
fi
grep -q '^CALL release create v0.1.0-rc.1 --draft ' "$WORK/gh.log" &&
    grep -q -- '^ARG --prerelease$' "$WORK/gh.log" &&
    ok "预发布 create 带 --prerelease" || bad "预发布 create 缺少 --prerelease"
grep -q '^CALL release edit v0.1.0-rc.1 --draft=false --prerelease=true --latest=false' "$WORK/gh.log" &&
    ok "预发布公开时 --latest=false（不冒充最新稳定版）" || bad "预发布公开没有 --latest=false"

if publish_run "$RC_MANIFEST" missing v0.1.0-rc.1 "$RC_PUBLISH_DIST" 0.1.0-rc.1 false \
    >"$WORK/publish.log" 2>&1; then
    bad "PRERELEASE=false 把 rc 版本发成了稳定版"
else
    ok "Reject a prerelease flag that conflicts with the version"
fi
if [ -s "$WORK/gh.log" ]; then
    bad "预发布状态冲突时仍调用了 gh"
else
    ok "预发布状态冲突时没有调用 gh"
fi

# 7. 标签与版本不一致：不碰远端
if publish_run "$MANIFEST" missing v0.1.1 "$PUBLISH_DIST" 0.1.0 "" >"$WORK/publish.log" 2>&1; then
    bad "标签 v0.1.1 与版本 0.1.0 不一致却发布成功"
elif [ -s "$WORK/gh.log" ]; then
    bad "标签不一致仍调用了 gh"
else
    ok "标签不一致时没有调用 gh"
fi

# ---- 结果 -------------------------------------------------------------------

if [ "$FAILS" -ne 0 ]; then
    printf '\n%d 条失败\n' "$FAILS" >&2
    exit 1
fi
printf '\n全部通过\n'
