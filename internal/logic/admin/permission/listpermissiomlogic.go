package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPermissiomLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPermissiomLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPermissiomLogic {
	return &ListPermissiomLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPermissiomLogic) ListPermissiom() (resp *types.ListRecoursesReply, err error) {
	// todo: add your logic here and delete this line

	return
}
