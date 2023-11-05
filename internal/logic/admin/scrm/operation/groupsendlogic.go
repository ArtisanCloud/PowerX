package operation

import (
	"context"
	"fmt"

	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GroupSendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGroupSendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GroupSendLogic {
	return &GroupSendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GroupSendLogic) GroupSend(req *types.GroupSendRequest) error {
	result, err := l.svcCtx.PowerX.SCRM.Wework.ExternalContactMessageTemplate.AddMsgTemplate(l.ctx, &request.RequestAddMsgTemplate{
		ChatType:       "single",
		ExternalUserID: req.ExternalUserIds,
		Text: &request.TextOfMessage{
			Content: req.Text.Content,
		},
	})
	if err != nil {
		return err
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("errcode: %d, errmsg: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}
