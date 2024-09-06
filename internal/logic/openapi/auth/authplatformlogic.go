package auth

import (
	"PowerX/internal/types/errorx"
	customerdomain2 "PowerX/internal/uc/powerx/crm/customerdomain"
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AuthPlatformLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Auth by platform
func NewAuthPlatformLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthPlatformLogic {
	return &AuthPlatformLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AuthPlatformLogic) AuthPlatform(req *types.PlatformAuthRequest) (resp *types.PlatformAuthResponse, err error) {

	// 暂时只开放给BrainX
	if req.AccessKey != l.svcCtx.Config.OpenAPI.Platforms.BrainX.AccessKey {
		return nil, errorx.ErrOpenAPIPlatformUnAuthorization

	}
	if req.SecretKey != l.svcCtx.Config.OpenAPI.Platforms.BrainX.SecretKey {
		return nil, errorx.ErrOpenAPIPlatformInvalidSecret
	}

	token := l.svcCtx.OpenAPI.Auth.SignPlatformToken(req.AccessKey, l.svcCtx.Config.OpenAPI.Platforms.BrainX.SecretKey)

	return &types.PlatformAuthResponse{
		TokenType:    token.TokenType,
		ExpiresIn:    fmt.Sprintf("%d", customerdomain2.CustomerTokenExpiredDuration),
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
	}, err
}
