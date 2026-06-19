#!/bin/bash
echo "prepare go mod"
go mod tidy && go mod download
echo "generating ent"
go generate ./ent
echo "building dist"
GOOS=linux GOARCH=amd64 go build -o dist/lazycat-mcp ./cmd/mcp
echo "copy resources"
rm -rf dist/resources
mkdir -p dist/resources
cp -R resources/skills dist/resources/
echo "ensure permission"
chmod +x dist/lazycat-mcp
