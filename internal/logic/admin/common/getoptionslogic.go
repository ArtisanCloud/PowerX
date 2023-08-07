package common

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOptionsLogic {
	return &GetOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetOptions 获取选项 建议返回为 {label: title, value: key}
func (l *GetOptionsLogic) GetOptions(req *types.GetOptionsRequest) (resp *types.GetOptionsReply, err error) {
	var options []map[string]any
	switch req.Type {
	case "position":
		options, err = l.svcCtx.PowerX.Organization.GetPositionOptionMap(l.ctx, req.Search)
		break
	case "role":
		options, err = l.svcCtx.PowerX.AdminAuthorization.GetRoleOptionMap(l.ctx, req.Search)
		break
	default:
		return nil, errorx.WithCause(errorx.ErrBadRequest, "不支持的选项类型")
	}
	if err != nil {
		return nil, err
	}
	return &types.GetOptionsReply{
		Options: options,
	}, nil
}
