package invokers

import (
	"context"
	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	"github.com/rs/zerolog"
)

type Ability struct {
	gw *gohelper.APIGateway
	lg *zerolog.Logger
}

func NewAbility(ctx context.Context, lg *zerolog.Logger) *Ability {
	gateway, err := gohelper.NewAPIGateway(ctx)
	if err != nil {
		return nil
	}
	return &Ability{
		gw: gateway,
		lg: lg,
	}
}
