#!/bin/bash
set -e

GO_BIN=${GO_BIN:-go}
TARGET_OS=${LAZYCAT_TARGET_OS:-linux}
TARGET_ARCH=${LAZYCAT_TARGET_ARCH:-amd64}

VERSION=${VERSION:-$(awk '/^version:/ {print $2; exit}' package.yml | tr -d '"')}
VERSION=${VERSION:-dev}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || true)}
COMMIT=${COMMIT:-unknown}
BUILD_TIME=${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}
LDFLAGS="-s -w -X lazycat-mcp/internal/buildinfo.Version=${VERSION} -X lazycat-mcp/internal/buildinfo.Commit=${COMMIT} -X lazycat-mcp/internal/buildinfo.BuildTime=${BUILD_TIME}"

echo "install frontend deps"
npm ci --include=dev
echo "build frontend"
npm run build --workspace frontend
echo "prepare go mod"
"$GO_BIN" mod tidy && "$GO_BIN" mod download
echo "generating ent"
"$GO_BIN" generate ./ent
echo "building dist ${VERSION} (${COMMIT})"
GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" "$GO_BIN" build -ldflags "$LDFLAGS" -o dist/lazycat-mcp ./cmd/mcp
echo "copy resources"
rm -rf dist/resources
mkdir -p dist/resources
cp -R resources/skills dist/resources/
echo "ensure permission"
chmod +x dist/lazycat-mcp
