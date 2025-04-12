package tools

import (
	"context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"log"
)

const (
	powerOff       = "power-off"
	reboot         = "reboot"
	queryLedStatus = "query-led-status"
	ledOn          = "led-on"
	ledOff         = "led-off"
)

var PowerKit server.ServerTool

func init() {
	PowerKit = server.ServerTool{
		Tool: mcp.NewTool("lazycat_power_op",
			mcp.WithDescription("let lazycat device to invoke a power operation 设置懒猫设备进行电源操作"),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Enum(powerOff, reboot, queryLedStatus, ledOff, ledOn),
				mcp.Description("operation to execute on device要在设备上执行的操作"),
			),
		),
		Handler: powerKitHandler,
	}
}

func powerKitHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	op := request.Params.Arguments["operation"]
	log.Println(op)
	return mcp.NewToolResultText("操作成功"), nil
}
