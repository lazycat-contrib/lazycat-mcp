package kit

import (
	"context"
	"gitee.com/linakesi/lzc-sdk/lang/go/common"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	online    = "online"
	offline   = "offline"
	allDevice = "all-device"
)

func (m *Manager) DeviceKits() []server.ServerTool {
	domainCheck := server.ServerTool{
		Tool: mcp.NewTool("lazycat_device_query",
			mcp.WithDescription("query lazy cat device list 查询懒猫设备列表"),
			mcp.WithString("status_kind",
				mcp.Required(),
				mcp.Enum(online, offline, allDevice),
				mcp.Description("the device status kind to filter 查询的设备类型"),
			),
		),
		Handler: m.deviceListHandler,
	}
	return []server.ServerTool{domainCheck}
}

func (m *Manager) deviceListHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	uIDs, err := m.gw.Users.ListUIDs(ctx, &common.ListUIDsRequest{})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	statusKind, err := request.RequireString("status_kind")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	userIDs := uIDs.GetUids()
	if len(userIDs) == 0 {
		return mcp.NewToolResultError("no user id found"), nil
	}
	devices, err := m.gw.Devices.ListEndDevices(ctx, &common.ListEndDeviceRequest{Uid: userIDs[0]})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	endDevices := devices.GetDevices()
	if endDevices == nil || len(endDevices) == 0 {
		return mcp.NewToolResultError("failed to find any device"), nil
	}
	switch statusKind {
	case online:
		onlineDevices := make([]*common.EndDevice, 0, 3)
		for _, device := range endDevices {
			if device.IsOnline {
				onlineDevices = append(onlineDevices, device)
			}
		}
		return mcp.NewToolResultText(j(onlineDevices)), nil
	case offline:
		offlineDevices := make([]*common.EndDevice, 0, 3)
		for _, device := range endDevices {
			if !device.IsOnline {
				offlineDevices = append(offlineDevices, device)
			}
		}
		return mcp.NewToolResultText(j(offlineDevices)), nil

	default:
		return mcp.NewToolResultText(j(endDevices)), nil
	}
}
