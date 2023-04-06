package powerx

import (
	"PowerX/internal/model"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type CustomerUseCase struct {
	db *gorm.DB
}

func NewCustomerUseCase(db *gorm.DB) *CustomerUseCase {
	return &CustomerUseCase{
		db: db,
	}
}

type ExternalId struct {
	OpenIDInMiniProgram           string
	OpenIDInWeChatOfficialAccount string
	OpenIDInWeCom                 string
}

type Customer struct {
	model.Model
	Name        string
	Mobile      string `gorm:"unique"`
	Email       string `gorm:"unique"`
	InviterID   int64
	Inviter     *Customer
	Source      string
	Type        string
	IsActivated bool
	ExternalId
}

type Lead struct {
	model.Model
	Name        string
	Mobile      string `gorm:"unique"`
	Email       string `gorm:"unique"`
	InviterID   int64
	Inviter     *Customer
	Source      string
	Status      string
	IsActivated bool
	ExternalId
}

func (uc *CustomerUseCase) CreateLead(ctx context.Context, lead *Lead) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) PatchLead(ctx context.Context, id int64, lead *Lead) {
	if err := uc.db.WithContext(ctx).Model(&Lead{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) GetLead(ctx context.Context, id int64) (*Lead, error) {
	var lead Lead
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *CustomerUseCase) DeleteLead(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&Lead{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return nil
}

func (uc *CustomerUseCase) CreateCustomer(ctx context.Context, customer *Customer) {
	if err := uc.db.WithContext(ctx).Create(&customer).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) PatchCustomer(ctx context.Context, id int64, customer *Customer) {
	if err := uc.db.WithContext(ctx).Model(&Customer{}).Where(id).Updates(&customer).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) GetCustomer(ctx context.Context, id int64) (*Customer, error) {
	var customer Customer
	if err := uc.db.WithContext(ctx).First(&customer, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
		}
		panic(err)
	}
	return &customer, nil
}

func (uc *CustomerUseCase) DeleteCustomer(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&Customer{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
	}
	return nil
}
