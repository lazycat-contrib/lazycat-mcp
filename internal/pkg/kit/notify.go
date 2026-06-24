package kit

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"gitee.com/linakesi/lzc-sdk/lang/go/common"
	"gitee.com/linakesi/lzc-sdk/lang/go/localdevice"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const deviceNotifyTimeout = 15 * time.Second

type notificationTarget struct {
	UID    string
	Device *common.EndDevice
}

type notificationDelivery struct {
	UID            string `json:"uid,omitempty"`
	UniqueDeviceID string `json:"unique_device_id,omitempty"`
	Name           string `json:"name,omitempty"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
}

type notificationResult struct {
	RequestedCount int                    `json:"requested_count"`
	SentCount      int                    `json:"sent_count"`
	FailedCount    int                    `json:"failed_count"`
	Deliveries     []notificationDelivery `json:"deliveries"`
}

func (m *Manager) deviceNotifyTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("lazycat_device_notify",
			mcp.WithDescription("send a LazyCat system notification to all online devices or selected devices 发送懒猫系统通知到全部在线设备或指定设备"),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("notification title 通知标题"),
			),
			mcp.WithString("body",
				mcp.Required(),
				mcp.Description("notification body 通知正文"),
			),
			mcp.WithString("deeplink_url",
				mcp.Description("optional deeplink URL opened when the notification is clicked 可选点击通知后打开的链接"),
			),
			mcp.WithString("device_id",
				mcp.Description("optional single target unique_device_id; empty means all online devices 可选单个目标设备ID，留空表示全部在线设备"),
			),
			mcp.WithArray("device_ids",
				mcp.WithStringItems(),
				mcp.UniqueItems(true),
				mcp.Description("optional target unique_device_id list; empty means all online devices 可选目标设备ID列表，留空表示全部在线设备"),
			),
		),
		Handler: m.deviceNotifyHandler,
	}
}

func (m *Manager) deviceNotifyHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	notifyReq, deviceIDs, err := notificationRequestFromTool(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if m == nil || m.gw == nil {
		return mcp.NewToolResultError("lazycat api gateway unavailable"), nil
	}

	available, err := m.listNotificationTargets(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	targets, err := resolveNotificationTargets(available, deviceIDs)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	out := notificationResult{
		RequestedCount: len(targets),
		Deliveries:     make([]notificationDelivery, 0, len(targets)),
	}
	for _, target := range targets {
		delivery := notificationDelivery{
			UID:            target.UID,
			UniqueDeviceID: target.Device.GetUniqueDeivceId(),
			Name:           target.Device.GetName(),
			Status:         "sent",
		}
		if err := m.notifyDevice(ctx, target.Device.GetDeviceApiUrl(), notifyReq); err != nil {
			delivery.Status = "failed"
			delivery.Error = err.Error()
			out.FailedCount++
		} else {
			out.SentCount++
		}
		out.Deliveries = append(out.Deliveries, delivery)
	}

	if out.SentCount == 0 && out.FailedCount > 0 {
		return mcp.NewToolResultError(j(out)), nil
	}
	return mcp.NewToolResultText(j(out)), nil
}

func notificationRequestFromTool(request mcp.CallToolRequest) (*localdevice.NotifyRequest, []string, error) {
	title, err := request.RequireString("title")
	if err != nil {
		return nil, nil, err
	}
	body, err := request.RequireString("body")
	if err != nil {
		return nil, nil, err
	}
	notifyReq, err := newNotifyRequest(title, body, request.GetString("deeplink_url", ""))
	if err != nil {
		return nil, nil, err
	}

	deviceIDs := request.GetStringSlice("device_ids", nil)
	if id := strings.TrimSpace(request.GetString("device_id", "")); id != "" {
		deviceIDs = append(deviceIDs, id)
	}
	return notifyReq, normalizeStringSet(deviceIDs), nil
}

func newNotifyRequest(title, body, deeplinkURL string) (*localdevice.NotifyRequest, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	deeplinkURL = strings.TrimSpace(deeplinkURL)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if body == "" {
		return nil, fmt.Errorf("body is required")
	}
	req := &localdevice.NotifyRequest{
		Title: title,
		Body:  body,
	}
	if deeplinkURL != "" {
		req.DeeplinkUrl = &deeplinkURL
	}
	return req, nil
}

func (m *Manager) listNotificationTargets(ctx context.Context) ([]notificationTarget, error) {
	uids, err := m.gw.Users.ListUIDs(ctx, &common.ListUIDsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if len(uids.GetUids()) == 0 {
		return nil, fmt.Errorf("no user id found")
	}

	var targets []notificationTarget
	seen := make(map[string]struct{})
	for _, uid := range uids.GetUids() {
		devices, err := m.gw.Devices.ListEndDevices(ctx, &common.ListEndDeviceRequest{Uid: uid})
		if err != nil {
			return nil, fmt.Errorf("list devices for user %q: %w", uid, err)
		}
		for _, device := range devices.GetDevices() {
			key := notificationTargetKey(uid, device)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, notificationTarget{UID: uid, Device: device})
		}
	}
	return targets, nil
}

func notificationTargetKey(uid string, device *common.EndDevice) string {
	if device == nil {
		return ""
	}
	if id := strings.TrimSpace(device.GetUniqueDeivceId()); id != "" {
		return id
	}
	apiURL := strings.TrimSpace(device.GetDeviceApiUrl())
	if apiURL == "" {
		return ""
	}
	return uid + "|" + apiURL
}

func resolveNotificationTargets(available []notificationTarget, deviceIDs []string) ([]notificationTarget, error) {
	deviceIDs = normalizeStringSet(deviceIDs)
	if len(deviceIDs) == 0 {
		out := make([]notificationTarget, 0, len(available))
		for _, target := range available {
			if target.Device != nil && target.Device.GetIsOnline() {
				out = append(out, target)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no online device found")
		}
		return out, nil
	}

	byID := make(map[string]notificationTarget, len(available))
	for _, target := range available {
		if target.Device == nil {
			continue
		}
		id := strings.TrimSpace(target.Device.GetUniqueDeivceId())
		if id == "" {
			continue
		}
		if _, exists := byID[id]; !exists {
			byID[id] = target
		}
	}

	out := make([]notificationTarget, 0, len(deviceIDs))
	for _, id := range deviceIDs {
		target, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("device %q not found", id)
		}
		if !target.Device.GetIsOnline() {
			return nil, fmt.Errorf("device %q is offline", id)
		}
		out = append(out, target)
	}
	return out, nil
}

func normalizeStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (m *Manager) notifyDevice(ctx context.Context, deviceAPIURL string, req *localdevice.NotifyRequest) error {
	deviceAPIURL = strings.TrimSpace(deviceAPIURL)
	if deviceAPIURL == "" {
		return fmt.Errorf("device api url is empty")
	}
	parsed, err := url.Parse(deviceAPIURL)
	if err != nil {
		return fmt.Errorf("parse device api url: %w", err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("device api url host is empty")
	}

	callCtx, cancel := context.WithTimeout(ctx, deviceNotifyTimeout)
	defer cancel()

	cred, err := gohelper.BuildClientCredOption(gohelper.CAPath, gohelper.APPKeyPath, gohelper.APPCertPath)
	if err != nil {
		return fmt.Errorf("build device credentials: %w", err)
	}
	conn, err := grpc.DialContext(callCtx, parsed.Host, grpc.WithBlock(), cred)
	if err != nil {
		return fmt.Errorf("connect device api: %w", err)
	}
	defer conn.Close()

	token, err := gohelper.RequestAuthToken(callCtx, conn)
	if err != nil {
		return fmt.Errorf("request device auth token: %w", err)
	}
	notifyCtx := metadata.AppendToOutgoingContext(callCtx, "lzc_dapi_auth_token", token.Token)
	_, err = localdevice.NewNotificationServiceClient(conn).Notify(notifyCtx, req)
	if err != nil {
		return fmt.Errorf("notify device: %w", err)
	}
	return nil
}
