package permission

import (
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListRecoursesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListRecoursesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRecoursesLogic {
	return &ListRecoursesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListRecoursesLogic) ListRecourses() (resp *types.ListRecoursesReply, err error) {
	resSlice := l.svcCtx.UC.Auth.FindManyAuthResWithActs(l.ctx)
	fmt.Println(resSlice)
	var list []types.AuthRes
	for _, res := range resSlice {
		var acts []types.AuthResAct
		for _, act := range res.Acts {
			acts = append(acts, types.AuthResAct{
				ResCode: act.ResCode,
				Action:  act.Action,
				Desc:    act.Desc,
			})
		}
		list = append(list, types.AuthRes{
			ResCode: res.ResCode,
			ResName: res.ResName,
			Type:    res.Type,
			Desc:    res.Desc,
			Acts:    acts,
		})
	}

	resp = &types.ListRecoursesReply{
		List: list,
	}
	return
}
