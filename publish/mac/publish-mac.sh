#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/publish/output"
STAGING_ROOT="$ROOT_DIR/publish/staging/mac"
ARCH=""
VERSION=""
SKIP_BUILD=0
SKIP_RUNTIME_VERIFY=0
KEEP_STAGING=0

usage() {
  cat <<'EOF'
Usage:
  publish/mac/publish-mac.sh --arch <arm64|amd64> [options]

Options:
  --arch <arm64|amd64>   Target architecture (required)
  --version <ver>        Package version (default: read from wails.json)
  --skip-build           Skip frontend and Wails build steps
  --skip-runtime-verify  Skip runtime hash verification
  --keep-staging         Keep assembled .app bundle in publish/staging/mac
  -h, --help             Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)
      ARCH="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --skip-runtime-verify)
      SKIP_RUNTIME_VERIFY=1
      shift
      ;;
    --keep-staging)
      KEEP_STAGING=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[ERROR] Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$ARCH" ]]; then
  echo "[ERROR] --arch is required" >&2
  usage
  exit 1
fi

if [[ "$ARCH" != "amd64" && "$ARCH" != "arm64" ]]; then
  echo "[ERROR] unsupported arch: $ARCH (expected amd64 or arm64)" >&2
  exit 1
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "[ERROR] this script must run on macOS host" >&2
  exit 1
fi

host_arch_raw="$(uname -m)"
case "$host_arch_raw" in
  x86_64) HOST_ARCH="amd64" ;;
  arm64) HOST_ARCH="arm64" ;;
  *)
    echo "[ERROR] unsupported host architecture: $host_arch_raw" >&2
    exit 1
    ;;
esac

if [[ "$HOST_ARCH" != "$ARCH" ]]; then
  echo "[ERROR] host arch is $HOST_ARCH but target arch is $ARCH." >&2
  echo "        Build the first macOS package on a native runner for the same architecture." >&2
  exit 1
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[ERROR] required command not found: $1" >&2
    exit 1
  fi
}

require_cmd python3
require_cmd ditto
require_cmd wails
require_cmd codesign

if [[ -z "$VERSION" ]]; then
  VERSION="$(python3 - "$ROOT_DIR/wails.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)
version = (((data or {}).get("info") or {}).get("productVersion") or "").strip()
if not version:
    raise SystemExit("productVersion missing in wails.json")
print(version)
PY
)"
fi

TARGET="darwin-$ARCH"
RUNTIME_DIR="$ROOT_DIR/bin/$TARGET"
XRAY_SRC="$RUNTIME_DIR/xray"
SINGBOX_SRC="$RUNTIME_DIR/sing-box"
APP_BIN_DIR="$ROOT_DIR/build/bin"
CHROME_README_SRC="$ROOT_DIR/chrome/README.md"
CONFIG_INIT_SRC="$ROOT_DIR/publish/config.init.mac.yaml"
ZIP_NAME="ProfilePool-${VERSION}-macos-${ARCH}.zip"
APP_EXPORT="$OUTPUT_DIR/ProfilePool-${VERSION}-macos-${ARCH}.app"
STAGE_DIR="$STAGING_ROOT/$TARGET"
APP_STAGE="$STAGE_DIR/ProfilePool.app"

find_built_app_bundle() {
  python3 - "$APP_BIN_DIR" <<'PY'
from pathlib import Path
import sys

root = Path(sys.argv[1])
if not root.is_dir():
    sys.exit(0)

candidates = [p for p in root.iterdir() if p.is_dir() and p.suffix == ".app"]
if not candidates:
    sys.exit(0)

candidates.sort(key=lambda p: p.stat().st_mtime, reverse=True)
print(candidates[0])
PY
}

manifest_has_target() {
  python3 - "$ROOT_DIR/publish/runtime-manifest.json" "$TARGET" <<'PY'
import json
import sys

manifest_path = sys.argv[1]
target = sys.argv[2]

with open(manifest_path, "r", encoding="utf-8") as f:
    data = json.load(f)

for item in data.get("files", []):
    if target in (item.get("targets") or []):
        print("yes")
        raise SystemExit(0)

raise SystemExit(1)
PY
}

echo "========================================"
echo "  ProfilePool macOS Publish"
echo "========================================"
echo "Target : $TARGET"
echo "Version: $VERSION"
echo "Root   : $ROOT_DIR"
echo

if [[ ! -f "$XRAY_SRC" || ! -f "$SINGBOX_SRC" ]]; then
  echo "[ERROR] runtime files missing for $TARGET" >&2
  echo "        expected: $XRAY_SRC and $SINGBOX_SRC" >&2
  exit 1
fi

if [[ ! -f "$CONFIG_INIT_SRC" ]]; then
  echo "[ERROR] mac config template missing: $CONFIG_INIT_SRC" >&2
  exit 1
fi

if [[ "$SKIP_RUNTIME_VERIFY" -ne 1 ]]; then
  if manifest_has_target >/dev/null 2>&1; then
    bash "$ROOT_DIR/tools/runtime/verify-runtime.sh" "$TARGET"
  else
    echo "[WARN] runtime manifest does not yet define $TARGET, skipping hash verification"
  fi
else
  echo "[WARN] runtime verification skipped"
fi

if [[ "$SKIP_BUILD" -ne 1 ]]; then
  echo "[1/4] Installing frontend dependencies..."
  (cd "$ROOT_DIR/frontend" && npm ci --prefer-offline --no-audit --no-fund)

  echo "[2/4] Building frontend assets..."
  (cd "$ROOT_DIR/frontend" && npm run build:clean)

  echo "[3/4] Building macOS app bundle with Wails..."
  (
    cd "$ROOT_DIR"
    wails build -s -platform "darwin/$ARCH" -o profilepool
  )
else
  echo "[WARN] skipping build step"
fi

APP_SOURCE="$(find_built_app_bundle)"
if [[ -z "$APP_SOURCE" || ! -d "$APP_SOURCE" ]]; then
  echo "[ERROR] failed to locate built .app bundle under $APP_BIN_DIR" >&2
  exit 1
fi

echo "[4/4] Assembling macOS app bundle..."
rm -rf "$APP_STAGE" "$APP_EXPORT"
mkdir -p "$STAGE_DIR" "$OUTPUT_DIR"
ditto "$APP_SOURCE" "$APP_STAGE"

APP_MACOS_DIR="$APP_STAGE/Contents/MacOS"
if [[ ! -d "$APP_MACOS_DIR" ]]; then
  echo "[ERROR] invalid app bundle layout, missing: $APP_MACOS_DIR" >&2
  exit 1
fi

mkdir -p "$APP_MACOS_DIR/bin"
cp "$XRAY_SRC" "$APP_MACOS_DIR/bin/xray"
cp "$SINGBOX_SRC" "$APP_MACOS_DIR/bin/sing-box"
chmod +x "$APP_MACOS_DIR/bin/xray" "$APP_MACOS_DIR/bin/sing-box"

# 种子文件(config.yaml / chrome/README.md)放进 Contents/Resources，而非 Contents/MacOS。
# 原因:macOS 的 codesign 会把 MacOS/ 下的所有文件当作嵌套代码对象逐个签名,生成基于 xattr 的
# 逐文件签名;而 zip/unzip 会剥离这些 xattr,导致解压后 seal 校验失败(a sealed resource is missing)。
# 把纯数据种子放到 Resources/(Apple 以资源方式哈希进顶层 seal,不产生逐文件代码签名)即可让
# 压缩包在 unzip 解压后仍通过 codesign --deep --strict 校验。运行时 EnsureWritableLayout 会
# 优先从 Resources 读取这些种子并拷贝到用户可写状态目录(见 apppath.go)。
APP_RESOURCES_DIR="$APP_STAGE/Contents/Resources"
mkdir -p "$APP_RESOURCES_DIR"
cp "$CONFIG_INIT_SRC" "$APP_RESOURCES_DIR/config.yaml"

if [[ -f "$CHROME_README_SRC" ]]; then
  mkdir -p "$APP_RESOURCES_DIR/chrome"
  cp "$CHROME_README_SRC" "$APP_RESOURCES_DIR/chrome/README.md"
fi

# 关键:wails build(第 [3/4] 步)先对 bundle 自签,随后本步才注入 bin/xray、bin/sing-box 与
# 种子文件 —— 这些新增文件不在原 seal 内,导致签名失效("a sealed resource is missing")。
# 故必须在所有文件就位后重新 ad-hoc 签名(与本应用一贯的分发签名方式一致,无开发者证书/公证,
# 首次启动需右键打开)。--deep 连同 bin/ 下的运行时可执行一起签。
#
# 注意:签名与打包必须在非 iCloud 的临时目录完成。若工程目录在 ~/Documents(iCloud Drive),
# 系统会在 .app 上反复重打 com.apple.FinderInfo 等扩展属性 —— codesign 视之为
# "resource fork / detritus not allowed" 拒绝 seal,且签完后又被重新打上导致 seal 立刻失效。
# 因此把组装好的 bundle ditto 到 TMPDIR,清 xattr、签名、打包,再只把产物(.app/.zip)拷回 output。
SIGN_WORK="$(mktemp -d "${TMPDIR:-/tmp}/profilepool-sign.XXXXXX")"
trap 'rm -rf "$SIGN_WORK"' EXIT
SIGN_APP="$SIGN_WORK/ProfilePool.app"
ditto "$APP_STAGE" "$SIGN_APP"
xattr -cr "$SIGN_APP" 2>/dev/null || true
codesign --force --deep --sign - "$SIGN_APP"
# 不吞掉 codesign 的诊断输出 —— 校验失败时它就是定位根因的唯一线索。
if ! codesign --verify --deep --strict "$SIGN_APP"; then
  echo "[ERROR] 重签名后 seal 校验失败(bundle 位于临时目录 $SIGN_WORK,脚本退出后会被清理)" >&2
  exit 1
fi

# 以干净已签 bundle 作为导出与打包源(--norsrc 防 iCloud 在拷回时附带的 xattr 进入 zip)。
ditto --norsrc "$SIGN_APP" "$APP_EXPORT"
rm -f "$OUTPUT_DIR/$ZIP_NAME"
# 注意两点:
#  1) 不要用 --sequesterRsrc —— 它会把资源 fork/xattr 抽离成 AppleDouble(._*)文件,破坏 seal。
#  2) 加 --norsrc 排除资源 fork 与 xattr,避免任何扩展属性在 zip 里变成 AppleDouble(._*)垃圾,
#     导致解压后被 codesign 判为 "file added"。
ditto -c -k --norsrc --keepParent "$SIGN_APP" "$OUTPUT_DIR/$ZIP_NAME"

# 对拷回的 .app 做一次尽力校验:zip 才是权威分发产物;但若输出目录在 iCloud(如 ~/Documents),
# 系统可能在拷回后给 .app 重打 xattr 使其 seal 失效 —— 此处如实提示,不让"坏 .app"无声通过。
if codesign --verify --deep --strict "$APP_EXPORT" >/dev/null 2>&1; then
  EXPORT_SEAL="valid"
else
  EXPORT_SEAL="invalid"
  echo "[WARN] 导出目录中的 .app 校验未通过(若输出目录在 iCloud 会重打 xattr 所致)。" >&2
  echo "       请使用 zip 解压后的 App 测试/分发;zip 内的 bundle 已通过 seal 校验。" >&2
fi

echo "Artifacts generated:"
echo "  - $OUTPUT_DIR/$ZIP_NAME   [distribution 权威产物, seal 已校验]"
echo "  - $APP_EXPORT   [seal: $EXPORT_SEAL]"

if [[ "$KEEP_STAGING" -ne 1 ]]; then
  rm -rf "$APP_STAGE"
fi

echo "Done."
