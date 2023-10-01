package menu

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/ArtisanCloud/PowerLibs/v3/object"

	"github.com/zeromicro/go-zero/core/logx"
)

type QueryMenusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewQueryMenusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QueryMenusLogic {
	return &QueryMenusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *QueryMenusLogic) QueryMenus() (resp *types.QueryMenusReply, err error) {

	res, err := l.svcCtx.PowerX.WechatOA.App.Menu.Get(l.ctx)
	if err != nil {
		return nil, err
	}
	if res.ErrCode != 0 {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, res.ErrMsg)
	}

	return &types.QueryMenusReply{
		Button:    res.Menus.Buttons,
		MatchRule: object.HashMap{},
	}, nil
}
