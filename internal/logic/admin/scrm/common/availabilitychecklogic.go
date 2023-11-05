package common

import (
	"context"

	"PowerX/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type AvailabilityCheckLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAvailabilityCheckLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AvailabilityCheckLogic {
	return &AvailabilityCheckLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AvailabilityCheckLogic) AvailabilityCheck() (err error) {
	if err = l.svcCtx.PowerX.SCRM.AvailabilityCheck(); err != nil {
		return err
	}
	return nil
}
