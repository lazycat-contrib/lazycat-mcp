package main

import (
	"lzcycat-mcp/internal/tools"

	"github.com/mark3labs/mcp-go/server"
)

const (
	version = "1.0.0"
)

func main() {
	svr := server.NewMCPServer(
		"LazyCat Mcp 🚀",
		version,
	)

	svr.AddTools(tools.PowerKit)
	sseServer := server.NewSSEServer(svr)
	err := sseServer.Start(":3000")
	if err != nil {
		panic(err)
	}
}
