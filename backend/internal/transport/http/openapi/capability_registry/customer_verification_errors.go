package capability_registry

import (
	"errors"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	capdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
)

func customerVerificationErrorTemplate(err error) (capdto.ErrorTemplate, bool) {
	for _, mapping := range []struct {
		err    error
		status int
		code   string
	}{
		{customersvc.ErrVerificationInvalidArgument, 400, "CUSTOMER_VERIFICATION_INVALID_ARGUMENT"},
		{customersvc.ErrVerificationUnauthorized, 401, "CUSTOMER_VERIFICATION_UNAUTHORIZED"},
		{customersvc.ErrVerificationForbidden, 403, "CUSTOMER_VERIFICATION_FORBIDDEN"},
		{customersvc.ErrVerificationNotFound, 404, "CUSTOMER_MEMBERSHIP_NOT_FOUND"},
		{customersvc.ErrVerificationCustomerInactive, 409, "CUSTOMER_ACCOUNT_INACTIVE"},
		{customersvc.ErrVerificationMembershipInactive, 409, "CUSTOMER_MEMBERSHIP_INACTIVE"},
		{customersvc.ErrVerificationMembershipExpired, 409, "CUSTOMER_MEMBERSHIP_EXPIRED"},
		{customersvc.ErrVerificationPrimaryContactInvalid, 409, "CUSTOMER_PRIMARY_CONTACT_INVALID"},
		{customersvc.ErrVerificationUnavailable, 503, "CUSTOMER_VERIFICATION_UNAVAILABLE"},
	} {
		if errors.Is(err, mapping.err) {
			base := capdto.ErrInvalidRequest
			switch mapping.status {
			case 401:
				base = capdto.ErrUnauthorized
			case 403:
				base = capdto.ErrCapabilityForbidden
			case 404:
				base = capdto.ErrNotFound
			case 503:
				base = capdto.ErrUnavailable
			}
			base.HTTPStatus = mapping.status
			return base.WithDetails(map[string]interface{}{"reason_code": mapping.code}), true
		}
	}
	return capdto.ErrorTemplate{}, false
}
