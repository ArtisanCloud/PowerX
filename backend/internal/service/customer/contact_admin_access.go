package customer

import "context"

func (s *ContactService) AuthorizeAdmin(ctx context.Context, resource, action string) (string, uint64, error) {
	return authorizeCustomerAdmin(ctx, s.db, resource, action)
}
