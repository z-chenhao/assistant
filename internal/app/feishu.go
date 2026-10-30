package app

import (
	"context"
	"github.com/z-chenhao/collie/internal/interaction/feishu"
)

func (a *App) FeishuStatus(ctx context.Context, check bool) feishu.ConnectionStatus {
	if a.feishu == nil {
		return feishu.ConnectionStatus{Channel: "feishu", Mode: "disabled", State: "not_configured", Action: "Configure feishu and bind the exact owner and chat through channel manage."}
	}
	if check {
		return a.feishu.Check(ctx)
	}
	return a.feishu.ConnectionStatus()
}
