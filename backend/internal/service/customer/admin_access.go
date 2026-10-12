package customer

import (
	"context"
	"strings"

	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

// 管理入口独立验证用户 JWT 的上下文；菜单可见、STS 和 API Key 均不授予此操作。
func authorizeCustomerAdmin(ctx context.Context, db *gorm.DB, resource, action string) (string, uint64, error) {
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil {
		return "", 0, dto.NewErrorWithCode(400, "CUSTOMER_TENANT_REQUIRED", "需要有效的当前租户", nil)
	}
	actor := reqctx.GetUserID(ctx)
	claims := reqctx.GetClaims(ctx)
	if actor == 0 || (claims != nil && claims.Issuer == "powerx-sts") {
		return "", 0, dto.NewErrorWithCode(403, "CUSTOMER_ADMIN_USER_REQUIRED", "客户管理需要管理员用户会话", nil)
	}
	if claims != nil {
		for _, plane := range claims.Platforms {
			if plane == "api_key" {
				return "", 0, dto.NewErrorWithCode(403, "CUSTOMER_ADMIN_USER_REQUIRED", "API Key 不能访问客户管理入口", nil)
			}
		}
	}
	if !reqctx.IsRoot(ctx) {
		allowed, err := iamrepo.NewPermissionRepository(db).MemberHasLivePlatformPermission(ctx, tenant, reqctx.GetMemberUUID(ctx), reqctx.GetMemberID(ctx), "customer", resource, action)
		if err != nil {
			return "", 0, dto.NewErrorWithCode(503, "CUSTOMER_ADMIN_UNAVAILABLE", "客户操作授权暂不可用", nil)
		}
		if !allowed {
			return "", 0, dto.NewErrorWithCode(403, "CUSTOMER_ADMIN_FORBIDDEN", "当前角色缺少客户操作权限", nil)
		}
	}
	return tenant, actor, nil
}

func (s *AccountService) CreateFromAdmin(ctx context.Context, in CreateAccountInput) (result customerrepo.AccountRow, err error) {
	tenant, _, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_accounts", "create")
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(in.TenantUUID) != tenant {
		return result, dto.NewErrorWithCode(403, "CUSTOMER_TENANT_MISMATCH", "租户不匹配", nil)
	}
	patch := AccountProfilePatch{PrimaryEmail: &in.PrimaryEmail, PrimaryPhone: &in.PrimaryPhone}
	if in.DisplayName != "" {
		patch.DisplayName = &in.DisplayName
	}
	if _, err := validateAccountPatch(patch); err != nil {
		return result, dto.NewErrorWithCode(400, "CUSTOMER_ACCOUNT_INVALID", "客户姓名、邮箱或电话格式无效", nil)
	}
	return s.Create(ctx, in)
}
