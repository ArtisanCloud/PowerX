package app

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateWeWorkAppGroupLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateWeWorkAppGroupLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWeWorkAppGroupLogic {
	return &CreateWeWorkAppGroupLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateWeWorkAppGroup
//
//	@Description:   创建企业群
//	@receiver this
//	@param opt
//	@return resp
//	@return err
func (group *CreateWeWorkAppGroupLogic) CreateWeWorkAppGroup(opt *types.AppGroupCreateRequest) (resp *types.AppGroupCreateReply, err error) {

	reply, err := group.svcCtx.PowerX.SCRM.Wechat.CreateWeWorkAppGroupRequest(&request.RequestAppChatCreate{
		Name:     opt.Name,
		Owner:    opt.Owner,
		UserList: opt.UserList,
		ChatID:   opt.ChatId,
	})

	return &types.AppGroupCreateReply{
		ChatId: reply.ChatID,
	}, err
}
