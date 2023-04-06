package customer

import (
	"PowerX/internal/model"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.MPCustomerLoginRequest) (resp *types.MPCustomerLoginAuthReply, err error) {
	// 获取session数据
	rs, err := l.svcCtx.PowerX.WechatMP.App.Auth.Session(l.ctx, req.Code)
	if err != nil {
		panic(err)
		return
	}
	//rs := &response.ResponseCode2Session{
	//	OpenID:     "o1IFX5A8sfi5nbkXwOzNLLLiL0OA",
	//	SessionKey: "IHaqJoWvRRCRlfnrRntzcA==",
	//}

	mpCustomer, err := l.svcCtx.PowerX.WechatMP.FindOneMPCustomer(l.ctx, &model.FindMPCustomerOption{
		OpenIDs: []string{rs.OpenID},
	})

	if err != nil {
		if errors.Is(err, errorx.ErrRecordNotFound) {
			return nil, errorx.ErrPhoneUnAuthorization
		} else {
			return nil, err
		}
	}

	// 如果手机为空，则需要用户授权手机信息
	if mpCustomer.PhoneNumber == "" {
		return nil, errorx.ErrPhoneUnAuthorization
	}

	// 生成一个新的token
	token := l.svcCtx.PowerX.CustomerAuthorization.SignToken(mpCustomer, l.svcCtx.Config.JWTSecret)

	return &types.MPCustomerLoginAuthReply{
		OpenID:      mpCustomer.OpenID,
		UnionID:     mpCustomer.UnionID,
		PhoneNumber: mpCustomer.PhoneNumber,
		NickName:    mpCustomer.NickName,
		AvatarURL:   mpCustomer.AvatarURL,
		Gender:      mpCustomer.Gender,
		Token: types.Token{
			TokenType:    token.TokenType,
			ExpiresIn:    fmt.Sprintf("%d", customerdomain.CustomerTokenExpiredDuration),
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
		},
	}, nil

	return
}
