package powerx

import (
	"PowerX/internal/types"
	"gorm.io/gorm"
)

type CustomerUseCase struct {
	db   *gorm.DB
	clue *ClueUseCase
}

func NewCustomerUseCase(db *gorm.DB, clueUseCase *ClueUseCase) *CustomerUseCase {
	return &CustomerUseCase{
		db:   db,
		clue: clueUseCase,
	}
}

type Customer struct {
	types.Model
	Id                 int64
	Name               string
	PhoneNumber        string
	OpenId             string
	InviteByCustomerId int64
	Source             string
	Type               string
	Status             string
}

func (c *CustomerUseCase) CreateCustomer(customer *Customer) {
	if err := c.db.Create(customer).Error; err != nil {
		panic(err)
	}
}

func (c *CustomerUseCase) GetCustomer(id int64) (customer *Customer, err error) {
	if err := c.db.First(&customer, id).Error; err != nil {
		return nil, err
	}
	return
}

func (c *CustomerUseCase) PatchCustomer(customer *Customer) {
	if err := c.db.Save(customer).Error; err != nil {
		panic(err)
	}
}

func (c *CustomerUseCase) DeleteCustomer(id int64) error {
	result := c.db.Delete(&Customer{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return nil
}
