#!/usr/bin/env bash
# One-command Windows build. Caches stay inside this repository.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck disable=SC1091
source "$ROOT/scripts/env.sh"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "需要 Go 1.26.10 或更新版本。可执行文件已经随发布提供，不必为了使用再安装 Go。"
  exit 1
fi
if ! command -v pnpm >/dev/null 2>&1; then
  echo "需要 pnpm。可用 corepack enable && corepack prepare pnpm@latest --activate"
  exit 1
fi

echo "运行测试"
go test ./pkg/... ./internal/...

if ! command -v wails >/dev/null 2>&1; then
  echo "安装 Wails CLI 到 .cache/gopath"
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
fi

echo "构建界面"
(
  cd "$ROOT/frontend"
  CI=true pnpm install --store-dir "$ROOT/.pnpm-store" --frozen-lockfile=false
  pnpm run build
)

echo "交叉编译 windows/amd64"
export GOOS=windows
export GOARCH=amd64
export CGO_ENABLED=0
wails build \
  -clean \
  -m \
  -s \
  -skipbindings \
  -trimpath \
  -nopackage \
  -platform windows/amd64 \
  -webview2 browser \
  -o moce.exe

mkdir -p "$ROOT/dist"
cp -f "$ROOT/build/bin/moce.exe" "$ROOT/dist/moce-windows-amd64.exe"
cp -f "$ROOT/build/bin/moce.exe" "$ROOT/dist/墨册.exe"
python3 "$ROOT/scripts/pack-portable.py" "$ROOT/build/bin/moce.exe" "$ROOT/dist/墨册-portable.zip"
SIZE="$(wc -c < "$ROOT/dist/墨册.exe" | tr -d ' ')"
ZIP="$(wc -c < "$ROOT/dist/墨册-portable.zip" | tr -d ' ')"
echo "输出：$ROOT/dist/墨册.exe （${SIZE} 字节）"
echo "输出：$ROOT/dist/墨册-portable.zip （${ZIP} 字节）"
