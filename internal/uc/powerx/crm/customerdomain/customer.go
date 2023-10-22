package customerdomain

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/securityx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type CustomerUseCase struct {
	db *gorm.DB
}

func NewCustomerUseCase(db *gorm.DB) *CustomerUseCase {
	return &CustomerUseCase{
		db: db,
	}
}

type FindManyCustomersOption struct {
	LikeName   string
	LikeMobile string
	Mobile     string
	Statuses   []int
	Sources    []int
	OrderBy    string
	types.PageEmbedOption
}

func (uc *CustomerUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyCustomersOption) *gorm.DB {
	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}
	if opt.LikeMobile != "" {
		db = db.Where("mobile LIKE ?", "%"+opt.LikeMobile+"%")
	}
	if opt.Mobile != "" {
		db = db.Where("mobile = ?", opt.Mobile)
	}
	if len(opt.Statuses) > 0 {
		db = db.Where("status IN ?", opt.Statuses)
	}
	if len(opt.Sources) > 0 {
		db = db.Where("source IN ?", opt.Sources)
	}
	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *CustomerUseCase) FindManyCustomers(ctx context.Context, opt *FindManyCustomersOption) (pageList types.Page[*customerdomain.Customer], err error) {
	var customers []*customerdomain.Customer
	db := uc.db.WithContext(ctx).Model(&customerdomain.Customer{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.
		//Debug().
		Find(&customers).Error; err != nil {
		panic(err)
	}

	return types.Page[*customerdomain.Customer]{
		List:      customers,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *CustomerUseCase) CreateCustomer(ctx context.Context, customer *customerdomain.Customer) error {
	if err := uc.db.WithContext(ctx).Create(&customer).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *CustomerUseCase) UpsertCustomer(ctx context.Context, customer *customerdomain.Customer) (*customerdomain.Customer, error) {

	customers := []*customerdomain.Customer{customer}

	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := powermodel.UpsertModelsOnUniqueID(tx, &customerdomain.Customer{}, customerdomain.CustomerUniqueId, customers, nil, false)

		if err != nil {
			panic(errors.Wrap(err, "upsert customerdomain failed"))
		}
		// 如果是新增用户，那么需要给一个唯一识别号
		if customer.Uuid == "" {
			customer.Uuid = securityx.GenerateUUID()
			err = powermodel.UpsertModelsOnUniqueID(tx, &customerdomain.Customer{}, customerdomain.CustomerUniqueId, customer, []string{"uuid"}, false)
			if err != nil {
				return err
			}
		}
		return err
	})

	return customer, err
}

func (uc *CustomerUseCase) UpsertCustomers(ctx context.Context, customers []*customerdomain.Customer) ([]*customerdomain.Customer, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &customerdomain.Customer{}, customerdomain.CustomerUniqueId, customers, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert customers failed"))
	}

	return customers, err
}

func (uc *CustomerUseCase) UpdateCustomer(ctx context.Context, id int64, customer *customerdomain.Customer) error {
	//fmt.Dump(customer)
	err := uc.db.WithContext(ctx).Model(&customerdomain.Customer{}).
		Debug().
		Where(id).Updates(&customer).Error

	return err
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

func (uc *CustomerUseCase) GetCustomerByMobile(ctx context.Context, mobile string) (*customerdomain.Customer, error) {
	var customer customerdomain.Customer
	if err := uc.db.WithContext(ctx).
		Where("mobile", mobile).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
		}
		panic(err)
	}
	return &customer, nil
}

func (uc *CustomerUseCase) GetCustomerByUUID(ctx context.Context, uuid string) (*customerdomain.Customer, error) {
	var customer customerdomain.Customer
	if err := uc.db.WithContext(ctx).
		Where("uuid", uuid).
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到客户")
		}
		panic(err)
	}
	return &customer, nil
}
func (uc *CustomerUseCase) GetCustomerByInviteCode(ctx context.Context, inviteCode string) (*customerdomain.Customer, error) {
	var customer customerdomain.Customer
	if err := uc.db.WithContext(ctx).
		Where("invite_code", inviteCode).
		First(&customer).Error; err != nil {
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

func (uc *CustomerUseCase) CheckRegisterPhoneExist(ctx context.Context, mobile string) bool {

	customer := &customerdomain.Customer{}
	err := uc.db.WithContext(ctx).
		//Debug().
		Unscoped().
		Where("mobile", mobile).
		First(customer).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false
		}
		panic(err)
	}

	return customer.Id > 0

}
