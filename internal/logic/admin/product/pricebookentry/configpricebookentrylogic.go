package pricebookentry

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigPriceBookEntryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigPriceBookEntryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigPriceBookEntryLogic {
	return &ConfigPriceBookEntryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigPriceBookEntryLogic) ConfigPriceBookEntry(req *types.ConfigPriceBookEntryEntryRequest) (resp *types.ConfigPriceBookEntryEntryReply, err error) {

	return
}
