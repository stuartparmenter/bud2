#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
mkdir -p bin

# Suppress deprecated-declaration warnings from macOS SDK headers
case "$(uname -s)" in
  Darwin) export CGO_CFLAGS="-Wno-deprecated-declarations" ;;
esac

go build -o bin/bud ./cmd/bud
go build -o bin/efficient-notion-mcp ./cmd/efficient-notion-mcp

# Codesign on macOS if the signing identity exists
if [ "$(uname -s)" = "Darwin" ] && security find-identity -v -p codesigning 2>/dev/null | grep -q bud-dev; then
  codesign --sign "bud-dev" --force --deep bin/bud
fi
