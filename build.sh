#!/bin/bash
echo "prepare go mod"
go mod tidy && go mod download
echo "building dist"
GOOS=linux GOARCH=amd64 go build -o dist/lazycat-mcp ./cmd/mcp
echo "ensure permission"
chmod +x dist/lazycat-mcp