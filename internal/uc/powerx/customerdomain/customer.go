package customerdomain

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
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

func (uc *CustomerUseCase) CreateCustomer(ctx context.Context, customer *customerdomain.Customer) {
	if err := uc.db.WithContext(ctx).Create(&customer).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) UpsertCustomer(ctx context.Context, customer *customerdomain.Customer) (*customerdomain.Customer, error) {

	customers := []*customerdomain.Customer{customer}

	_, err := uc.UpsertCustomers(ctx, customers)
	if err != nil {
		panic(errors.Wrap(err, "upsert customers failed"))
	}

	return customer, err
}

func (uc *CustomerUseCase) UpsertCustomers(ctx context.Context, customers []*customerdomain.Customer) ([]*customerdomain.Customer, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &customerdomain.Customer{}, customerdomain.CustomerUniqueId, customers, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert customers failed"))
	}

	return customers, err
}

func (uc *CustomerUseCase) PatchCustomer(ctx context.Context, id int64, customer *customerdomain.Customer) {
	if err := uc.db.WithContext(ctx).Model(&customerdomain.Customer{}).Where(id).Updates(&customer).Error; err != nil {
		panic(err)
	}
}

func (uc *CustomerUseCase) GetCustomer(ctx context.Context, id int64) (*customerdomain.Customer, error) {
	var customer customerdomain.Customer
	if err := uc.db.WithContext(ctx).First(&customer, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
		}
		panic(err)
	}
	return &customer, nil
}

func (uc *CustomerUseCase) DeleteCustomer(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&customerdomain.Customer{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
	}
	return nil
}
