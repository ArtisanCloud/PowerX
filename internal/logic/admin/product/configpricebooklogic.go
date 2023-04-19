package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfigPriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewConfigPriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfigPriceBookLogic {
	return &ConfigPriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ConfigPriceBookLogic) ConfigPriceBook(req *types.ConfigPriceBookEntryRequest) (resp *types.ConfigPriceBookEntryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
