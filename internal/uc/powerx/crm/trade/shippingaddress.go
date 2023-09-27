package trade

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ShippingAddressUseCase struct {
	db *gorm.DB
}

func NewShippingAddressUseCase(db *gorm.DB) *ShippingAddressUseCase {
	return &ShippingAddressUseCase{
		db: db,
	}
}

type FindManyShippingAddressesOption struct {
	CustomerId        int64
	LikeName          string
	ShippingAddressBy string
	types.PageEmbedOption
}

func (uc *ShippingAddressUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyShippingAddressesOption) *gorm.DB {
	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "is_default desc, id desc"
	if opt.ShippingAddressBy != "" {
		orderBy = opt.ShippingAddressBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *ShippingAddressUseCase) FindAllShippingAddresses(ctx context.Context, opt *FindManyShippingAddressesOption) (addresses []*trade.ShippingAddress, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.ShippingAddress{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		//Debug().
		Find(&addresses).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return addresses, err
}

func (uc *ShippingAddressUseCase) FindManyShippingAddresses(ctx context.Context, opt *FindManyShippingAddressesOption) (pageList types.Page[*trade.ShippingAddress], err error) {
	opt.DefaultPageIfNotSet()
	var addresses []*trade.ShippingAddress
	db := uc.db.WithContext(ctx).Model(&trade.ShippingAddress{})

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
		Find(&addresses).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.ShippingAddress]{
		List:      addresses,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *ShippingAddressUseCase) CreateShippingAddress(ctx context.Context, address *trade.ShippingAddress) error {

	db := uc.db.WithContext(ctx).Model(trade.ShippingAddress{})

	err := db.Transaction(func(tx *gorm.DB) error {
		var err error

		// 如果新建地址是需要默认地址，需要先
		if address.IsDefault {
			err = tx.
				Where("customer_id", address.CustomerId).
				Update("is_default", false).Error
			if err != nil {
				return err
			}
		}
		err = tx.
			//Debug().
			Create(&address).Error

		return err
	})

	return err
}

func (uc *ShippingAddressUseCase) UpsertShippingAddress(ctx context.Context, address *trade.ShippingAddress, fieldsToUpdate []string) (*trade.ShippingAddress, error) {

	addresses := []*trade.ShippingAddress{address}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除地址的相关联对象
		_, err := uc.ClearAssociations(tx, address)
		if err != nil {
			return err
		}

		// 更新地址对象主体
		_, err = uc.UpsertShippingAddresses(ctx, addresses, fieldsToUpdate)
		if err != nil {
			return errors.Wrap(err, "upsert address failed")
		}

		return err
	})

	return address, err
}

func (uc *ShippingAddressUseCase) UpsertShippingAddresses(ctx context.Context, addresses []*trade.ShippingAddress, fieldsToUpdate []string) ([]*trade.ShippingAddress, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.ShippingAddress{}, trade.ShippingAddressUniqueId, addresses, fieldsToUpdate, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert addresses failed"))
	}

	return addresses, err
}

func (uc *ShippingAddressUseCase) PatchShippingAddress(ctx context.Context, id int64, address *trade.ShippingAddress) {
	db := uc.db.WithContext(ctx).Model(trade.ShippingAddress{})
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error

		// 如果新建地址是需要默认地址，需要先
		if address.IsDefault {
			err = tx.
				//Debug().
				Where("customer_id", address.CustomerId).
				Where("id != ?", address.Id).
				Update("is_default", false).Error
			if err != nil {
				return err
			}

		}

		err = tx.
			//Debug().
			Where(id).Updates(address).Error

		return err
	})

	if err != nil {
		panic(err)
	}

}

func (uc *ShippingAddressUseCase) GetShippingAddress(ctx context.Context, id int64) (*trade.ShippingAddress, error) {
	var address = &trade.ShippingAddress{}
	if err := uc.db.WithContext(ctx).First(address, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到地址")
		}
		panic(err)
	}

	return address, nil
}

func (uc *ShippingAddressUseCase) DeleteShippingAddress(ctx context.Context, id int64) error {

	// 获取地址相关项
	address, err := uc.GetShippingAddress(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除地址相关项
		_, err = uc.ClearAssociations(tx, address)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.ShippingAddress{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到地址")
		}
		return err
	})

	return err
}

func (uc *ShippingAddressUseCase) ClearAssociations(db *gorm.DB, address *trade.ShippingAddress) (*trade.ShippingAddress, error) {
	var err error
	// 清除地址的关联
	//err = address.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return address, err
}
