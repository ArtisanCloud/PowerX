package customer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

var (
	ErrCustomerAccountInvalidArgument = errors.New("customer.account_invalid_argument")
	ErrCustomerAccountNotFound        = errors.New("customer.account_not_found")
)

// AccountProfilePatch 仅包含可更新的资料；nil 保持原值，空字符串清空可选资料。
type AccountProfilePatch struct {
	DisplayName  *string `json:"display_name,omitempty"`
	Nickname     *string `json:"nickname,omitempty"`
	GivenName    *string `json:"given_name,omitempty"`
	FamilyName   *string `json:"family_name,omitempty"`
	PrimaryEmail *string `json:"primary_email,omitempty"`
	PrimaryPhone *string `json:"primary_phone,omitempty"`
	AvatarURL    *string `json:"avatar_url,omitempty"`
	Locale       *string `json:"locale,omitempty"`
	Timezone     *string `json:"timezone,omitempty"`
	Status       *string `json:"status,omitempty"`
}

type UpdateAccountInput struct {
	TenantUUID, CustomerUUID string
	AccountProfilePatch
}

func (s *AccountService) Update(ctx context.Context, in UpdateAccountInput) (customerrepo.AccountRow, error) {
	tenantUUID, err := reqctx.CanonicalTenantUUID(in.TenantUUID)
	if err != nil {
		return customerrepo.AccountRow{}, fmt.Errorf("%w: tenant", ErrCustomerAccountInvalidArgument)
	}
	id, err := uuid.Parse(strings.TrimSpace(in.CustomerUUID))
	if err != nil || id == uuid.Nil {
		return customerrepo.AccountRow{}, fmt.Errorf("%w: customer_uuid", ErrCustomerAccountInvalidArgument)
	}
	fields, err := validateAccountPatch(in.AccountProfilePatch)
	if err != nil {
		return customerrepo.AccountRow{}, err
	}
	var result customerrepo.AccountRow
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountRepository(tx)
		if err := repo.UpdateProfile(ctx, tenantUUID, id.String(), fields); err != nil {
			return err
		}
		var err error
		result, err = repo.Get(ctx, tenantUUID, id.String())
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return customerrepo.AccountRow{}, ErrCustomerAccountNotFound
	}
	return result, err
}

func validateAccountPatch(p AccountProfilePatch) (map[string]any, error) {
	fields := map[string]any{}
	for _, field := range []struct {
		name  string
		value *string
		limit int
	}{
		{"display_name", p.DisplayName, 128}, {"nickname", p.Nickname, 128}, {"given_name", p.GivenName, 128}, {"family_name", p.FamilyName, 128},
		{"primary_email", p.PrimaryEmail, 255}, {"primary_phone", p.PrimaryPhone, 32}, {"avatar_url", p.AvatarURL, 2048}, {"locale", p.Locale, 32}, {"timezone", p.Timezone, 64}, {"status", p.Status, 32},
	} {
		if field.value == nil {
			continue
		}
		value := strings.TrimSpace(*field.value)
		invalid := !utf8.ValidString(value) || utf8.RuneCountInString(value) > field.limit || strings.IndexFunc(value, unicode.IsControl) >= 0
		switch field.name {
		case "display_name":
			invalid = invalid || value == ""
		case "status":
			invalid = invalid || !validCustomerStatus(value)
		case "primary_email":
			invalid = invalid || !validContactChannels(value, "")
		case "primary_phone":
			invalid = invalid || !validContactChannels("", value)
			if value != "" && strings.IndexFunc(value, func(r rune) bool { return r >= '0' && r <= '9' }) < 0 {
				invalid = true
			}
		case "avatar_url":
			if value != "" {
				u, err := url.Parse(value)
				invalid = invalid || err != nil || u == nil
				if u != nil {
					invalid = invalid || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil
				}
			}
		case "locale":
			if value != "" {
				_, err := language.Parse(value)
				invalid = invalid || err != nil
			}
		case "timezone":
			if value != "" {
				_, err := time.LoadLocation(value)
				invalid = invalid || err != nil || value == "Local"
			}
		}
		if invalid {
			return nil, fmt.Errorf("%w: %s", ErrCustomerAccountInvalidArgument, field.name)
		}
		fields[field.name] = value
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: empty update", ErrCustomerAccountInvalidArgument)
	}
	return fields, nil
}
