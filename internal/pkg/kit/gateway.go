package kit

import (
	"context"
	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"lzcycat-mcp/internal/pkg/zlog"
)

type Manager struct {
	gw *gohelper.APIGateway
	lg *zlog.Logger
}

func NewManager(ctx context.Context, lg *zlog.Logger) *Manager {
	gateway, err := gohelper.NewAPIGateway(ctx)
	if err != nil {
		return nil
	}
	return &Manager{
		gw: gateway,
		lg: lg,
	}
}

func (m *Manager) CleanUp() error {
	return m.gw.Close()
}
