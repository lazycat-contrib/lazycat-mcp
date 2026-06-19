#!/bin/bash
set -e

VERSION=${VERSION:-$(awk '/^version:/ {print $2; exit}' package.yml | tr -d '"')}
VERSION=${VERSION:-dev}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || true)}
COMMIT=${COMMIT:-unknown}
BUILD_TIME=${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}
LDFLAGS="-s -w -X lazycat-mcp/internal/buildinfo.Version=${VERSION} -X lazycat-mcp/internal/buildinfo.Commit=${COMMIT} -X lazycat-mcp/internal/buildinfo.BuildTime=${BUILD_TIME}"

echo "prepare go mod"
go mod tidy && go mod download
echo "generating ent"
go generate ./ent
echo "building dist ${VERSION} (${COMMIT})"
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/lazycat-mcp ./cmd/mcp
echo "copy resources"
rm -rf dist/resources
mkdir -p dist/resources
cp -R resources/skills dist/resources/
echo "ensure permission"
chmod +x dist/lazycat-mcp
