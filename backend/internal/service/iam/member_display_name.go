package iam

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

const CodeMemberDisplayNameConflict = "IAM_MEMBER_DISPLAY_NAME_CONFLICT"

var ErrMemberDisplayNameConflict = errors.New("iam.member_display_name_conflict")

func memberDisplayNameConflictError(err error) error {
	return dto.NewErrorWithCode(http.StatusConflict, CodeMemberDisplayNameConflict, CodeMemberDisplayNameConflict, err)
}

func MemberDisplayNameConflictError(err error) error {
	return memberDisplayNameConflictError(err)
}

func IsMemberDisplayNameConflict(err error) bool {
	return isMemberDisplayNameConflict(err)
}

func normalizedMemberDisplayName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *MemberService) ensureActiveDisplayNameAvailable(ctx context.Context, tx *gorm.DB, tenantUUID, displayName string, excludeMemberID uint64) error {
	nameKey := normalizedMemberDisplayName(displayName)
	if nameKey == "" {
		return nil
	}
	query := tx.WithContext(ctx).Table(model.TableIAMMember).
		Where("tenant_uuid = ? AND status = 1 AND deleted_at IS NULL", tenantUUID).
		Where("lower(trim(display_name)) = ?", nameKey)
	if excludeMemberID != 0 {
		query = query.Where("id <> ?", excludeMemberID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return memberDisplayNameConflictError(ErrMemberDisplayNameConflict)
	}
	return nil
}

func isMemberDisplayNameConflict(err error) bool {
	return errors.Is(err, ErrMemberDisplayNameConflict) ||
		strings.Contains(strings.ToLower(err.Error()), "uk_iam_member_active_display_name")
}
