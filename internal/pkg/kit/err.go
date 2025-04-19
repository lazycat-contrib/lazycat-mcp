package kit

import (
	"fmt"
	"github.com/mark3labs/mcp-go/mcp"
)

func checkMCPErr(err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("%s:%v", operationFailed, err)), err
	}
	return mcp.NewToolResultText(operationSuccess), nil
}
