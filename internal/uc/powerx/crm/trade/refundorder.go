package trade

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type RefundOrderUseCase struct {
	db *gorm.DB
}

func NewRefundOrderUseCase(db *gorm.DB) *RefundOrderUseCase {
	return &RefundOrderUseCase{
		db: db,
	}
}

type FindManyRefundOrdersOption struct {
	LikeName      string
	RefundOrderBy string
	types.PageEmbedOption
}

func (uc *RefundOrderUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyRefundOrdersOption) *gorm.DB {

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "id desc"
	if opt.RefundOrderBy != "" {
		orderBy = opt.RefundOrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *RefundOrderUseCase) FindAllRefundOrders(ctx context.Context, opt *FindManyRefundOrdersOption) (orders []*trade.RefundOrder, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.RefundOrder{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		//Debug().
		Preload("Artisans").
		Find(&orders).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return orders, err
}

func (uc *RefundOrderUseCase) FindManyRefundOrders(ctx context.Context, opt *FindManyRefundOrdersOption) (pageList types.Page[*trade.RefundOrder], err error) {
	opt.DefaultPageIfNotSet()
	var payments []*trade.RefundOrder
	db := uc.db.WithContext(ctx).Model(&trade.RefundOrder{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.Find(&payments).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.RefundOrder]{
		List:      payments,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *RefundOrderUseCase) CreateRefundOrder(ctx context.Context, payment *trade.RefundOrder) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&payment).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *RefundOrderUseCase) UpsertRefundOrder(ctx context.Context, payment *trade.RefundOrder) (*trade.RefundOrder, error) {

	payments := []*trade.RefundOrder{payment}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除退款单的相关联对象
		_, err := uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		// 更新退款单对象主体
		_, err = uc.UpsertRefundOrders(ctx, payments)
		if err != nil {
			return errors.Wrap(err, "upsert payment failed")
		}

		return err
	})

	return payment, err
}

func (uc *RefundOrderUseCase) UpsertRefundOrders(ctx context.Context, payments []*trade.RefundOrder) ([]*trade.RefundOrder, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.RefundOrder{}, trade.RefundOrderUniqueId, payments, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert payments failed"))
	}

	return payments, err
}

func (uc *RefundOrderUseCase) PatchRefundOrder(ctx context.Context, id int64, payment *trade.RefundOrder) {
	if err := uc.db.WithContext(ctx).Model(&trade.RefundOrder{}).
		Where(id).Updates(&payment).Error; err != nil {
		panic(err)
	}
}

func (uc *RefundOrderUseCase) GetRefundOrder(ctx context.Context, id int64) (*trade.RefundOrder, error) {
	var payment = &trade.RefundOrder{}
	if err := uc.db.WithContext(ctx).First(payment, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到退款单")
		}
		panic(err)
	}

	return payment, nil
}

func (uc *RefundOrderUseCase) DeleteRefundOrder(ctx context.Context, id int64) error {

	// 获取退款单相关项
	payment, err := uc.GetRefundOrder(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除退款单相关项
		_, err = uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.RefundOrder{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到退款单")
		}
		return err
	})

	return err
}

func (uc *RefundOrderUseCase) ClearAssociations(db *gorm.DB, payment *trade.RefundOrder) (*trade.RefundOrder, error) {
	var err error
	// 清除退款单的关联
	//err = payment.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return payment, err
}
