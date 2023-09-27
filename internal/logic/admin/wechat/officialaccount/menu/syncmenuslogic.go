package menu

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/officialAccount/menu/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncMenusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncMenusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncMenusLogic {
	return &SyncMenusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SyncMenusLogic) SyncMenus(req *types.SyncMenusRequest) (resp *types.SyncMenusReply, err error) {

	//resDelete, err := l.svcCtx.PowerX.WechatOA.App.Menu.Delete(l.ctx)
	//if err != nil {
	//	return nil, err
	//}
	//if resDelete.ErrCode != 0 {
	//	return nil, errorx.WithCause(errorx.ErrDeleteObject, resDelete.ErrMsg)
	//}

	buttons := TransformRequestToWechatOAMenu(&req.OAMenu)
	//fmt.Dump(buttons)
	_, err = l.svcCtx.PowerX.WechatOA.App.Menu.Create(l.ctx, buttons)
	if err != nil {
		return nil, err
	}

	return &types.SyncMenusReply{
		Success: true,
		Data:    buttons,
	}, nil
}

func TransformRequestToWechatOAMenu(req *types.OAMenu) []*request.Button {
	buttons := []*request.Button{}
	for _, oaButton := range req.OAButton {
		button := TransformRequestToWechatOAButton(oaButton)
		buttons = append(buttons, button)
	}
	return buttons
}

func TransformRequestToWechatOAButton(oaButton *types.OAButton) *request.Button {
	subButtons := TransformRequestToWechatOASubButtons(oaButton.OASubButton)
	return &request.Button{
		Type: oaButton.Type,
		Name: oaButton.Name,
		Key:  oaButton.Key,
		//MediaId:    oaButton.MediaId,
		URL:        oaButton.Url,
		AppID:      oaButton.AppID,
		PagePath:   oaButton.PagePath,
		SubButtons: subButtons,
	}
}

func TransformRequestToWechatOASubButtons(oaSubButtons []*types.OASubButton) []request.SubButton {
	suyButtons := []request.SubButton{}
	for _, oaSubButton := range oaSubButtons {
		subButton := TransformRequestToWechatOASubButton(oaSubButton)
		suyButtons = append(suyButtons, subButton)
	}
	return suyButtons
}

func TransformRequestToWechatOASubButton(oaSuhButton *types.OASubButton) request.SubButton {
	return request.SubButton{
		Type:     oaSuhButton.Type,
		Name:     oaSuhButton.Name,
		URL:      oaSuhButton.Url,
		AppID:    oaSuhButton.AppID,
		PagePath: oaSuhButton.PagePath,
		Key:      oaSuhButton.Key,
	}

}
