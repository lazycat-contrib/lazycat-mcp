package kit

import (
	"context"
	"fmt"

	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"lazycat-mcp/internal/pkg/zlog"
)

type Manager struct {
	gw *gohelper.APIGateway
	lg *zlog.Logger
}

func NewManager(ctx context.Context, lg *zlog.Logger) (*Manager, error) {
	gateway, err := gohelper.NewAPIGateway(ctx)
	if err != nil {
		return &Manager{lg: lg}, fmt.Errorf("create lazycat api gateway: %w", err)
	}
	return NewManagerWithGateway(gateway, lg), nil
}

func NewManagerWithGateway(gateway *gohelper.APIGateway, lg *zlog.Logger) *Manager {
	return &Manager{
		gw: gateway,
		lg: lg,
	}
}

func (m *Manager) CleanUp() error {
	if m == nil || m.gw == nil {
		return nil
	}
	return m.gw.Close()
}

func (m *Manager) Available() bool {
	return m != nil && m.gw != nil
}
