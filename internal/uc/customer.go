package uc

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
	Name   string
	Avatar string
}

func (c *CustomerUseCase) CreateCustomers(ctx context.Context) {

}
