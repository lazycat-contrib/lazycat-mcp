package main

import (
	"context"
	"github.com/mark3labs/mcp-go/server"
	"lzcycat-mcp/internal/pkg/kit"
	"lzcycat-mcp/internal/pkg/zlog"
)

const (
	version = "1.0.0"
)

func main() {
	logConfig := zlog.LogConfig{
		LogLevel:    "info",
		LogDir:      "/lzcapp/var/logs",
		LogFileName: "mcp-app.log",
		MaxSize:     10,
		MaxBackups:  5, // 保留5个备份文件
		MaxAge:      7, // 保留7天的日志文件
	}

	logger := zlog.NewLogger(logConfig)
	svr := server.NewMCPServer(
		"LazyCat Mcp 🚀",
		version,
	)
	kitManager := kit.NewManager(context.Background(), logger)
	defer kitManager.CleanUp()

	svr.AddTools(
		// 电源功能选项
		kitManager.PowerKit(),
	)
	sseServer := server.NewSSEServer(svr)
	err := sseServer.Start(":3000")
	if err != nil {
		panic(err)
	}
}
