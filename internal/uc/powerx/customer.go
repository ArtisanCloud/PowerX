package powerx

import (
	"context"
	"gorm.io/gorm"
)

type CustomerUseCase struct {
	db *gorm.DB
}

func newCustomerUseCase(db *gorm.DB) *CustomerUseCase {
	return &CustomerUseCase{
		db: db,
	}
}

type Customer struct {
	Name                  string
	Avatar                string
	MiniProgramOpenId     string
	OfficialAccountOpenId string
	WeWorkOpenId          string
	PhoneNumber           string
	Email                 string
	InviterCustomerId     string
	Source                string
	Status                string
	Type                  string
	IsActivated           bool
}

func (c *CustomerUseCase) CreateCustomers(ctx context.Context) {

}
