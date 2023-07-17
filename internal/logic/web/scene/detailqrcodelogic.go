package scene

import (
	"context"
	"fmt"
	"strings"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DetailQrcodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDetailQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DetailQrcodeLogic {
	return &DetailQrcodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// DetailQrcode
//  @Description:
//  @receiver qrcode
//  @param opt
//  @return resp
//  @return err
//
func (qrcode *DetailQrcodeLogic) DetailQrcode(opt *types.SceneRequest) (resp *types.SceneQrcodeActiveReply, err error) {
	if opt.Qid == `` {
		return nil, fmt.Errorf(`Qid error`)
	}

	detail := qrcode.svcCtx.PowerX.Scene.Scene.FindOneSceneQrcodeDetail(opt.Qid)
	go qrcode.svcCtx.PowerX.Scene.Scene.IncreaseSceneCpaNumber(opt.Qid)

	return &types.SceneQrcodeActiveReply{
		QId:                detail.QId,
		Name:               detail.Name,
		Desc:               detail.Desc,
		Owner:              strings.Split(detail.Owner, `,`),
		RealQrcodeLink:     detail.RealQrcodeLink,
		Platform:           detail.Platform,
		Classify:           detail.Classify,
		SceneLink:          detail.SceneLink,
		SafeThresholdValue: detail.SafeThresholdValue,
		ExpiryDate:         detail.ExpiryDate,
		State:              detail.State,
		ActiveQrcodeLink:   detail.ActiveQrcodeLink,
		CPA:                detail.Cpa,
	}, err
}
