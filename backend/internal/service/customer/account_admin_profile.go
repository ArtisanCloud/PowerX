package customer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AdminAccountProfileInput 不接受状态、租户、登录凭据或联系人变更。
type AdminAccountProfileInput struct {
	CustomerUUID           string
	ExpectedProfileVersion string
	DisplayName            *string
	Nickname               *string
	PrimaryEmail           *string
	PrimaryPhone           *string
}

func (s *AccountService) UpdateProfileFromAdmin(ctx context.Context, in AdminAccountProfileInput) (result customerrepo.AccountRow, err error) {
	tenant, actor, err := authorizeCustomerAdmin(ctx, s.db, "admin_customers_accounts_customer_uuid", "update")
	if err != nil {
		return result, err
	}
	id, err := uuid.Parse(strings.TrimSpace(in.CustomerUUID))
	if err != nil || id == uuid.Nil {
		return result, dto.NewErrorWithCode(400, "CUSTOMER_ACCOUNT_ID_INVALID", "客户 UUID 无效", nil)
	}
	version, err := hex.DecodeString(in.ExpectedProfileVersion)
	if err != nil || len(version) != 32 {
		return result, dto.NewErrorWithCode(400, "CUSTOMER_PROFILE_VERSION_REQUIRED", "请读取客户资料后再保存", nil)
	}
	fields, err := validateAccountPatch(AccountProfilePatch{DisplayName: in.DisplayName, Nickname: in.Nickname, PrimaryEmail: in.PrimaryEmail, PrimaryPhone: in.PrimaryPhone})
	if err != nil {
		return result, dto.NewErrorWithCode(400, "CUSTOMER_ACCOUNT_INVALID", "客户姓名、昵称、邮箱或电话格式无效", nil)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, _, err := authorizeCustomerAdmin(ctx, tx, "admin_customers_accounts_customer_uuid", "update"); err != nil {
			return err
		}
		repo := customerrepo.NewAccountRepository(tx)
		if err := repo.UpdateAdminProfile(ctx, tenant, id.String(), strings.ToLower(in.ExpectedProfileVersion), fields); err != nil {
			return err
		}
		var err error
		result, err = repo.Get(ctx, tenant, id.String())
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		requestID, _ := ctx.Value("request_id").(string)
		changes, _ := json.Marshal(map[string]any{"fields": keys, "previous_profile_version": in.ExpectedProfileVersion, "profile_version": result.ProfileVersion, "request_id": requestID})
		actorID := int64(actor)
		return tx.Create(&modelaudit.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: tenant, ActorUserID: &actorID, Source: "customer.account.admin", Operation: "customer.account.profile_updated", ResourceType: "customer.account", ResourceID: id.String(), Outcome: "SUCCESS", Severity: "INFO", CorrelationID: reqctx.GetTraceID(ctx), ChangesAfter: datatypes.JSON(changes), ChangesRedacted: true}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return customerrepo.AccountRow{}, dto.NewErrorWithCode(404, "CUSTOMER_ACCOUNT_NOT_FOUND", "当前租户未找到该客户", nil)
	}
	if errors.Is(err, customerrepo.ErrAccountProfileChanged) {
		return customerrepo.AccountRow{}, dto.NewErrorWithCode(409, "CUSTOMER_PROFILE_CHANGED", "客户资料已被修改，请重新读取后再编辑", nil)
	}
	if err != nil && dto.CodeOf(err) == "" {
		return customerrepo.AccountRow{}, dto.NewErrorWithCode(503, "CUSTOMER_PROFILE_UPDATE_FAILED", "客户资料未保存，请稍后重试", nil)
	}
	return result, err
}
