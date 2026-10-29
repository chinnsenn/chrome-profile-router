#!/bin/bash
# ==============================================================================
# Chrome Profile Router — 发布元数据与资产校验
# ==============================================================================
# [INPUT]: mygo.json（版本的唯一来源，可用 RELEASE_MANIFEST 指向副本做本地验证）、
#          git 标签名（v<SemVer>）、已下载好的产物目录。
# [OUTPUT]: tag-check 打印 GitHub Actions 的 key=value（version/tag/prerelease/
#          publish）；assets 校验并列出 tag 发布应有的 8 个资产名；sums 写
#          SHA256SUMS（只含 basename）并自校验；publish 用 gh 以 draft 起手，
#          传齐 9 个文件并核对远端名字/大小后才公开。
# [POS]: 只被 .github/workflows/ci.yml 调用；除 publish 外都能离线跑。
# [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
# ==============================================================================
#
# 版本只有 mygo.json 一个来源：标签是 v<version>，逐字比较；格式不对或不一致时
# 打包与发布都不开始。JSON 必须真的能被解析（jq，runner 预装），字段类型错了也失败。
#
# 发布只接受 core 加可选 prerelease（0.1.0、0.1.0-rc.1）。build metadata 属于
# SemVer 但不在本项目的发布格式里：所有资产名与 macOS 的 CFBundleShortVersionString
# 都用同一个字面版本串，标签、manifest、文件名保持一一对应，不引入第四种写法。
# 数字段与数字 prerelease 标识符不允许前导零。
#
# 资产名来自 MyGo 与 packaging/package.sh 的实际命名（见 docs/releasing.md）：
# macOS 的 DMG 在本地是 "<name> <version> <arch>.dmg"（含空格），进 staging 时把
# basename 的空格换成 '-'——GitHub 上传 asset 时会把空格规范化成 '.'，带空格的
# 名字在远端会变成 Chrome.Profile.Router…，与清单逐字比对就对不上；.deb 把
# prerelease 的 '-' 换成 '~'（0.1.0-rc.1 → 0.1.0~rc.1），tar.gz 与 zip 用连字符 slug。

set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

fail() { printf '错误: %s\n' "$*" >&2; exit 1; }
note() { printf '%s\n' "$*"; }

MANIFEST="${RELEASE_MANIFEST:-mygo.json}"
[ -f "$MANIFEST" ] || fail "缺少 $MANIFEST"
command -v jq >/dev/null 2>&1 || fail "缺少 jq：版本闸门用它解析 $MANIFEST"

# 顶层 name/version 必须存在且是字符串；坏 JSON、嵌套字段、类型不对都在这里失败。
manifest_field() {
    jq -er --arg key "$1" '
        if (.[$key] | type) == "string" and (.[$key] | length) > 0
        then .[$key] else error("\($key) 必须是字符串") end
    ' "$MANIFEST" 2>/dev/null || fail "$MANIFEST 不是合法 JSON 对象，或 $1 缺失/不是字符串"
}
APP_NAME="$(manifest_field name)"
APP_VERSION="$(manifest_field version)"
SLUG="$(printf '%s' "$APP_NAME" | tr '[:upper:]' '[:lower:]' | tr ' ' '-')"

# semver_ok：SemVer 2.0.0 的 core 与 prerelease；build metadata 不在发布格式里。
semver_ok() {
    local v="$1" pre id
    case "$v" in
        *+*) return 1 ;;
    esac
    local re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$'
    [[ "$v" =~ $re ]] || return 1
    pre="${BASH_REMATCH[5]:-}"
    [ -n "$pre" ] || return 0
    local IFS='.'
    for id in $pre; do
        if [[ "$id" != *[!0-9]* ]]; then
            if [ "${#id}" -gt 1 ] && [ "${id:0:1}" = "0" ]; then
                return 1
            fi
        fi
    done
    return 0
}

check_manifest() {
    semver_ok "$APP_VERSION" ||
        fail "$MANIFEST 的 version 不是可发布的 SemVer（v<major>.<minor>.<patch>[-prerelease]）: $APP_VERSION"
}

is_prerelease() {
    case "$1" in
        *-*) return 0 ;;
        *) return 1 ;;
    esac
}

# tag_version：校验标签并打印它对应的版本；格式错或与 mygo.json 不一致就失败。
tag_version() {
    local tag="$1" v
    [ "${tag#v}" != "$tag" ] || fail "标签必须以 v 开头: $tag"
    v="${tag#v}"
    semver_ok "$v" ||
        fail "标签不是可发布的 SemVer（v<major>.<minor>.<patch>[-prerelease]）: $tag"
    [ "$v" = "$APP_VERSION" ] ||
        fail "标签 $tag 与 $MANIFEST 的 version $APP_VERSION 不一致：先把 mygo.json 改成 $v 并提交"
    printf '%s' "$v"
}

# deb_version：与 MyGo 的 debVersion 一致——prerelease 的 '-' 换 '~'，其余 '-' 换 '.'。
deb_version() {
    local v="${1#v}" core rest
    case "$v" in
        *-*)
            core="${v%%-*}"
            rest="${v#*-}"
            v="${core}~${rest//-/.}"
            ;;
    esac
    printf '%s' "$v"
}

# expected_assets：tag 发布必须齐的 8 个资产（顺序固定，便于日志比对）。
# DMG 用 staging 的规范名：MyGo 本地名里的空格换成 '-'，版本串大小写照原样
# （prerelease 的 -rc.1 不被改写），与 CI「Stage DMGs」的改名规则是同一条。
expected_assets() {
    local version="$1" deb dmg
    deb="$(deb_version "$version")"
    dmg="${APP_NAME// /-}"
    printf '%s\n' \
        "${dmg}-${version}-arm64.dmg" \
        "${dmg}-${version}-amd64.dmg" \
        "${SLUG}_${deb}_amd64.deb" \
        "${SLUG}_${deb}_arm64.deb" \
        "${SLUG}-${version}-linux-amd64.tar.gz" \
        "${SLUG}-${version}-linux-arm64.tar.gz" \
        "${SLUG}-${version}-windows-amd64.zip" \
        "${SLUG}-${version}-windows-arm64.zip"
}

# cmd_assets：目录里必须正好是这 8 个非空文件（扁平），多一个少一个都失败。
# 打印每个资产名，供 sums / publish 复用。
cmd_assets() {
    local dir="$1" version="$2" tmp name
    check_manifest
    [ "$version" = "$APP_VERSION" ] ||
        fail "参数版本 $version 与 $MANIFEST 的 version $APP_VERSION 不一致"
    [ -d "$dir" ] || fail "资产目录不存在: $dir"

    tmp="$(mktemp -d "${TMPDIR:-/tmp}/cpr-assets.XXXXXX")"
    expected_assets "$version" | sort >"$tmp/expected"
    # SHA256SUMS 是派生文件（sums 写出），不算资产，也不参与 8 个文件的比对
    find "$dir" -mindepth 1 -maxdepth 1 -type f -print |
        while IFS= read -r p; do basename "$p"; done |
        while IFS= read -r n; do
            [ "$n" = "SHA256SUMS" ] || printf '%s\n' "$n"
        done | sort >"$tmp/actual"

    if ! diff -u "$tmp/expected" "$tmp/actual" >"$tmp/diff"; then
        sed -e '1,2d' "$tmp/diff" >&2 || true
        rm -rf "$tmp"
        fail "资产与版本 $version 期望的 8 个文件不一致（- 期望，+ 实际）"
    fi
    while IFS= read -r name; do
        [ -f "$dir/$name" ] || fail "资产不在目录顶层: $name"
        [ -s "$dir/$name" ] || fail "资产是 0 字节，不能发布: $name"
    done <"$tmp/expected"
    rm -rf "$tmp"
    expected_assets "$version"
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

# cmd_sums：SHA256SUMS 只写 basename（不含目录前缀），不包含它自己，
# 写完立刻用系统的校验命令读回一遍，确保清单里的每个名字都能被 -c 解析；
# 名字就是远端公开名（DMG 已按 staging 的规范名写出），下载后可直接校验。
cmd_sums() {
    local dir="$1" version="$2" name sum names
    names="$(cmd_assets "$dir" "$version")"

    : >"$dir/SHA256SUMS"
    while IFS= read -r name; do
        sum="$(sha256_of "$dir/$name")"
        [ -n "$sum" ] || fail "无法计算校验和: $name"
        printf '%s  %s\n' "$sum" "$name" >>"$dir/SHA256SUMS"
    done <<<"$names"

    (
        cd "$dir"
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum -c SHA256SUMS
        else
            shasum -a 256 -c SHA256SUMS
        fi
    ) >/dev/null || fail "SHA256SUMS 自校验失败"

    note "SHA256SUMS：$(wc -l <"$dir/SHA256SUMS" | tr -d ' ') 行，已自校验"
}

# 发布正文的固定前言：平台状态必须写在这里，不能被自动生成的 notes 盖掉。
# 文档链接固定到本次标签，避免 /main 随时间漂移。
release_notes() {
    local tag="$1" repo="https://github.com/chinnsenn/chrome-profile-router"
    cat <<EOF
> **Windows 为实验支持**：原生 Windows 上的默认应用注册、单实例与显示效果尚未验证，只有 amd64 在 Wine 下跑过功能测试。
> **macOS 包未公证**（ad-hoc 签名）：首次打开可能需要在「系统设置 → 隐私与安全性」里手动允许。
>
> 安装步骤与已知限制见 [平台说明]($repo/blob/$tag/docs/platforms.md)，资产与校验方式见 [发布说明]($repo/blob/$tag/docs/releasing.md)。
EOF
}

# 远端状态：用 gh release view，它先按 tag 查已发布 Release，再对 draft 走 GraphQL
# release(tagName:) 兜底（cli/cli pkg/cmd/release/shared.FetchRelease），所以 draft
# 也能查到；裸 REST /releases/tags/<tag> 只覆盖已发布的那一半。
# 只把 gh 明确的 "release not found" 当「还没有 Release」；网络、权限等错误原样失败，
# 不盲发第二次公开创建。
release_state() {
    local tag="$1" err="$2" out
    if out="$(gh release view "$tag" --json isDraft,assets --jq '[.isDraft, (.assets | length)] | @tsv' 2>"$err")"; then
        printf '%s' "$out"
        return 0
    fi
    if grep -q 'release not found' "$err"; then
        printf 'missing'
        return 0
    fi
    sed -e 's/^/  /' "$err" >&2
    return 1
}

# 上传后的硬核对：远端资产必须正好是本地的这 9 个名字，且大小一致。
verify_remote_assets() {
    local tag="$1" tmp="$2" file name
    shift 2
    : >"$tmp/remote"
    : >"$tmp/local"
    for file in "$@"; do
        name="$(basename "$file")"
        printf '%s\t%s\n' "$(wc -c <"$file" | tr -d ' ')" "$name"
    done | sort >"$tmp/local"
    gh release view "$tag" --json assets --jq '.assets[] | "\(.size)\t\(.name)"' |
        sort >"$tmp/remote"
    diff -u "$tmp/local" "$tmp/remote" >"$tmp/diff" ||
        {
            sed -e 's/^/  /' "$tmp/diff" >&2
            fail "Release $tag 的资产与上传清单（名字/大小）不一致"
        }
}

# cmd_publish：先校验标签/版本/资产，再动远端。
# - 新标签：建 draft（不带文件）→ 传 9 个文件 → 核对远端名字/大小 → 公开。半成品不露。
# - 已有 draft：允许幂等续传（--clobber），核对后公开。
# - 已有公开 Release：明确拒绝覆盖，绝不静默改已发布的包。
cmd_publish() {
    local dir="$1" version="$2"
    local tag="${TAG:-}" checked prerelease names name files=()
    [ -n "$tag" ] || fail "需要环境变量 TAG"
    checked="$(tag_version "$tag")" || exit 1
    [ "$checked" = "$version" ] || fail "标签 $tag 与发布版本 $version 不一致"

    # 预发布状态由版本本身决定；env 只能确认，不能把一个 rc 版本发成稳定版
    prerelease=false
    if is_prerelease "$version"; then prerelease=true; fi
    if [ -n "${PRERELEASE:-}" ] && [ "${PRERELEASE}" != "$prerelease" ]; then
        fail "PRERELEASE=${PRERELEASE} 与版本 $version 的预发布状态（${prerelease}）不一致"
    fi

    names="$(cmd_assets "$dir" "$version")"
    cmd_sums "$dir" "$version"
    while IFS= read -r name; do files+=("$dir/$name"); done <<<"$names"
    files+=("$dir/SHA256SUMS")

    command -v gh >/dev/null 2>&1 || fail "缺少 gh（GitHub CLI）"

    local tmp state draft args notes
    tmp="$(mktemp -d "${TMPDIR:-/tmp}/cpr-publish.XXXXXX")"
    if ! state="$(release_state "$tag" "$tmp/err")"; then
        rm -rf "$tmp"
        fail "查询 Release $tag 失败（见上），不创建新 Release"
    fi

    case "$state" in
        missing)
            notes="$(release_notes "$tag")"
            args=(release create "$tag" --draft --verify-tag --generate-notes --notes "$notes")
            if [ "$prerelease" = "true" ]; then
                args+=(--prerelease --latest=false)
            fi
            gh "${args[@]}"
            gh release upload "$tag" "${files[@]}" --clobber
            verify_remote_assets "$tag" "$tmp" "${files[@]}"
            ;;
        *)
            draft="${state%%$'\t'*}"
            if [ "$draft" != "true" ]; then
                rm -rf "$tmp"
                fail "Release $tag 已发布：拒绝覆盖已公开的资产；要换内容请打新版本标签"
            fi
            gh release upload "$tag" "${files[@]}" --clobber
            verify_remote_assets "$tag" "$tmp" "${files[@]}"
            ;;
    esac

    args=(release edit "$tag" --draft=false --prerelease="$prerelease")
    if [ "$prerelease" = "true" ]; then
        args+=(--latest=false)
    fi
    gh "${args[@]}"
    rm -rf "$tmp"
    note "已发布 ${tag}：9 个文件（8 个资产 + SHA256SUMS），prerelease=$prerelease"
}

usage() {
    cat <<'EOF'
用法: bash packaging/release.sh <命令> [参数]

  tag-check [<tag>]        校验 mygo.json 版本与标签；打印 GitHub Actions 输出。
                           空标签（分支/PR）只校验 manifest，并给出 publish=false。
  assets <目录> <版本>      校验目录里正好是 8 个非空发布资产，逐个打印文件名。
  sums <目录> <版本>        写 SHA256SUMS（basename）并自校验。
  publish <目录> <版本>     draft 起手创建/续传 Release，核对后公开；需要 TAG 与 GH_TOKEN。
EOF
}

case "${1:-}" in
    tag-check)
        check_manifest
        tag="${2:-}"
        if [ -z "$tag" ]; then
            printf 'version=%s\n' "$APP_VERSION"
            printf 'tag=\n'
            printf 'prerelease=false\n'
            printf 'publish=false\n'
            exit 0
        fi
        version="$(tag_version "$tag")" || exit 1
        prerelease=false
        if is_prerelease "$version"; then prerelease=true; fi
        printf 'version=%s\ntag=%s\nprerelease=%s\npublish=true\n' "$version" "$tag" "$prerelease"
        ;;
    assets) cmd_assets "${2:?需要资产目录}" "${3:?需要版本}" ;;
    sums) cmd_sums "${2:?需要资产目录}" "${3:?需要版本}" ;;
    publish) cmd_publish "${2:?需要资产目录}" "${3:?需要版本}" ;;
    -h | --help | help | "") usage ;;
    *) usage >&2; fail "未知命令: $1" ;;
esac
