package powerx

import (
	"context"
	"gorm.io/gorm"
)

type MembershipUseCase struct {
	db *gorm.DB
}

func NewMembershipUseCase(db *gorm.DB) *MembershipUseCase {
	return &MembershipUseCase{
		db: db,
	}
}

func (c *MembershipUseCase) CreateMemberships(ctx context.Context) {

}
