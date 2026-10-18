package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrRecipientInvalid = dto.NewErrorWithCode(http.StatusBadRequest, "NOTIFICATION_RECIPIENT_INVALID", "notification recipient is not an active member of this tenant", nil)

// RecipientAccess reads live IAM state, including immediately before WS writes.
type RecipientAccess struct{ db *gorm.DB }

func NewRecipientAccess(db *gorm.DB) *RecipientAccess { return &RecipientAccess{db: db} }

func (a *RecipientAccess) Require(ctx context.Context, tenant, member string) error {
	if a == nil || a.db == nil {
		return ErrRecipientInvalid
	}
	id, err := uuid.Parse(member)
	if err != nil || id == uuid.Nil {
		return ErrRecipientInvalid
	}
	var row im.Member
	if err = a.db.WithContext(ctx).Where("tenant_uuid=? AND uuid=? AND status=?", tenant, id, 1).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRecipientInvalid
		}
		return err
	}
	return a.active(ctx, tenant, row.UserID)
}

func (a *RecipientAccess) active(ctx context.Context, tenant string, user uint64) error {
	if a == nil || a.db == nil || user == 0 {
		return ErrRecipientInvalid
	}
	var n int64
	if err := a.db.WithContext(ctx).Model(&tm.Tenant{}).Where("uuid=? AND status=?", tenant, 1).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return ErrRecipientInvalid
	}
	if err := a.db.WithContext(ctx).Model(&im.User{}).Where("id=? AND status=?", user, 1).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return ErrRecipientInvalid
	}
	return nil
}

// ConnectionMember resolves only the authenticated connection identity. Root
// without a tenant member can receive broadcasts, never another member's notice.
func (a *RecipientAccess) ConnectionMember(ctx context.Context, tenant string, memberID, userID uint64, root bool) (string, error) {
	if err := a.active(ctx, tenant, userID); err != nil {
		return "", err
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.TenantUUID != tenant || claims.UserID != userID || claims.MemberID != memberID || claims.IsRoot != root {
		return "", ErrRecipientInvalid
	}
	if claims.ExpiresAt != nil && !claims.ExpiresAt.Time.After(time.Now()) {
		return "", ErrRecipientInvalid
	}
	if root {
		var n int64
		if err := a.db.WithContext(ctx).Model(&im.User{}).Where("id=? AND is_root=?", userID, true).Count(&n).Error; err != nil {
			return "", err
		}
		if n != 1 {
			return "", ErrRecipientInvalid
		}
	}
	var row im.Member
	err := a.db.WithContext(ctx).Where("id=? AND user_id=? AND tenant_uuid=? AND status=?", memberID, userID, tenant, 1).Take(&row).Error
	if err == nil {
		return row.UUID.String(), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if root {
		return "", nil
	}
	return "", ErrRecipientInvalid
}

// Target accepts the existing notification view and the member_uuid input
// spelling. Conflicting, malformed or non-string targets never mean broadcast.
func Target(raw []byte) (string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return "", ErrRecipientInvalid
	}
	var target string
	for _, key := range []string{"userId", "member_uuid"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		var member string
		if json.Unmarshal(value, &member) != nil || string(value) == "null" {
			return "", ErrRecipientInvalid
		}
		member = strings.TrimSpace(member)
		if member == "" {
			continue
		}
		id, err := uuid.Parse(member)
		if err != nil || id == uuid.Nil {
			return "", ErrRecipientInvalid
		}
		if target != "" && target != id.String() {
			return "", ErrRecipientInvalid
		}
		target = id.String()
	}
	return target, nil
}
