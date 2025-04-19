package kit

import (
	"context"
	"errors"
	"fmt"
	users "gitee.com/linakesi/lzc-sdk/lang/go/common"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"strings"
)

const (
	powerOff       = "power-off"
	reboot         = "reboot"
	queryLedStatus = "query-led-status"
	ledOn          = "led-on"
	ledOff         = "led-off"
)

var (
	unSupportOperation = errors.New("unsupport operation")
)

func (m *Manager) PowerKit() server.ServerTool {
	powerKit := server.ServerTool{
		Tool: mcp.NewTool("lazycat_power_op",
			mcp.WithDescription("let lazycat device to invoke a power operation 设置懒猫设备进行电源操作"),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Enum(powerOff, reboot, queryLedStatus, ledOff, ledOn),
				mcp.Description("operation to execute on device要在设备上执行的操作"),
			),
		),
		Handler: m.powerKitHandler,
	}
	return powerKit
}

func (m *Manager) powerKitHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if _op, ok := request.Params.Arguments["operation"].(string); ok {
		op := strings.ToLower(strings.Trim(_op, ""))
		switch op {
		case powerOff:
			err := m.powerOff(ctx)
			return checkMCPErr(err)
		case reboot:
			err := m.reboot(ctx)
			return checkMCPErr(err)
		case queryLedStatus:
			on, err := m.queryLedStatus(ctx)
			if err != nil {
				return checkMCPErr(err)
			}
			ledStatus := ""
			if on {
				ledStatus = "on"
			} else {
				ledStatus = "off"
			}
			return mcp.NewToolResultText(fmt.Sprintf("led staus:%s", ledStatus)), nil
		case ledOn:
			err := m.setLedStatus(ctx, true)
			return checkMCPErr(err)
		case ledOff:
			err := m.setLedStatus(ctx, false)
			return checkMCPErr(err)
		default:
			return mcp.NewToolResultText(operationSuccess), nil
		}
	}
	return mcp.NewToolResultText(operationFailed), unSupportOperation
}

func (m *Manager) powerOff(ctx context.Context) error {
	_, err := m.gw.Box.Shutdown(ctx, &users.ShutdownRequest{
		Action: users.ShutdownRequest_Poweroff,
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) reboot(ctx context.Context) error {
	_, err := m.gw.Box.Shutdown(ctx, &users.ShutdownRequest{
		Action: users.ShutdownRequest_Reboot,
	})
	if err != nil {
		return err
	}
	return nil
}

func (m *Manager) queryLedStatus(ctx context.Context) (bool, error) {
	boxInfo, err := m.gw.Box.QueryInfo(ctx, nil)
	if err != nil {
		return false, err
	}
	return boxInfo.PowerLed, nil
}

func (m *Manager) setLedStatus(ctx context.Context, onStaus bool) error {
	_, err := m.gw.Box.ChangePowerLed(ctx, &users.ChangePowerLedRequest{
		PowerLed: onStaus,
	})
	if err != nil {
		return err
	}
	return nil
}
