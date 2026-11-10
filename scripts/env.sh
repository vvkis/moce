#!/usr/bin/env bash
# Point every cache and temp directory at the project tree.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export MOCE_ROOT="$ROOT"
mkdir -p \
  "$ROOT/.cache/tmp" \
  "$ROOT/.cache/go-build" \
  "$ROOT/.cache/gomod" \
  "$ROOT/.cache/gopath" \
  "$ROOT/.cache/npm" \
  "$ROOT/.cache/pnpm-home" \
  "$ROOT/.pnpm-store" \
  "$ROOT/dist"
touch "$ROOT/.cache/goenv"
export GOPATH="$ROOT/.cache/gopath"
export GOCACHE="$ROOT/.cache/go-build"
export GOMODCACHE="$ROOT/.cache/gomod"
export GOTMPDIR="$ROOT/.cache/tmp"
export GOENV="$ROOT/.cache/goenv"
export GOTELEMETRY=off
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"
export TMPDIR="$ROOT/.cache/tmp"
export TMP="$ROOT/.cache/tmp"
export TEMP="$ROOT/.cache/tmp"
export XDG_CACHE_HOME="$ROOT/.cache"
export npm_config_cache="$ROOT/.cache/npm"
export PNPM_HOME="$ROOT/.cache/pnpm-home"
export CGO_ENABLED=0
if [ -x "$ROOT/.toolchain/go/bin/go" ]; then
  export PATH="$ROOT/.toolchain/go/bin:$ROOT/.cache/gopath/bin:$PATH"
else
  export PATH="$ROOT/.cache/gopath/bin:$PATH"
fi
