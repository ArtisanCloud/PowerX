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

type PaymentUseCase struct {
	db *gorm.DB
}

func NewPaymentUseCase(db *gorm.DB) *PaymentUseCase {
	return &PaymentUseCase{
		db: db,
	}
}

type FindManyPaymentsOption struct {
	LikeName  string
	PaymentBy string
	types.PageEmbedOption
}

func (uc *PaymentUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyPaymentsOption) *gorm.DB {

	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	orderBy := "id desc"
	if opt.PaymentBy != "" {
		orderBy = opt.PaymentBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *PaymentUseCase) FindAllPayments(ctx context.Context, opt *FindManyPaymentsOption) (dictionaryItems []*trade.Payment, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.Payment{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Debug().
		Preload("Artisans").
		Find(&dictionaryItems).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return dictionaryItems, err
}

func (uc *PaymentUseCase) FindManyPayments(ctx context.Context, opt *FindManyPaymentsOption) (pageList types.Page[*trade.Payment], err error) {
	opt.DefaultPageIfNotSet()
	var payments []*trade.Payment
	db := uc.db.WithContext(ctx).Model(&trade.Payment{})

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

	return types.Page[*trade.Payment]{
		List:      payments,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *PaymentUseCase) CreatePayment(ctx context.Context, payment *trade.Payment) error {

	if err := uc.db.WithContext(ctx).
		Debug().
		Create(&payment).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *PaymentUseCase) UpsertPayment(ctx context.Context, payment *trade.Payment) (*trade.Payment, error) {

	payments := []*trade.Payment{payment}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertPayments(ctx, payments)
		if err != nil {
			return errors.Wrap(err, "upsert payment failed")
		}

		return err
	})

	return payment, err
}

func (uc *PaymentUseCase) UpsertPayments(ctx context.Context, payments []*trade.Payment) ([]*trade.Payment, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Payment{}, powermodel.UniqueId, payments, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert payments failed"))
	}

	return payments, err
}

func (uc *PaymentUseCase) PatchPayment(ctx context.Context, id int64, payment *trade.Payment) {
	if err := uc.db.WithContext(ctx).Model(&trade.Payment{}).
		Where(id).Updates(&payment).Error; err != nil {
		panic(err)
	}
}

func (uc *PaymentUseCase) GetPayment(ctx context.Context, id int64) (*trade.Payment, error) {
	var payment = &trade.Payment{}
	if err := uc.db.WithContext(ctx).First(payment, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}

	return payment, nil
}

func (uc *PaymentUseCase) DeletePayment(ctx context.Context, id int64) error {

	// 获取产品相关项
	payment, err := uc.GetPayment(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品相关项
		_, err = uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.Payment{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		return err
	})

	return err
}

func (uc *PaymentUseCase) ClearAssociations(db *gorm.DB, payment *trade.Payment) (*trade.Payment, error) {
	var err error
	// 清除支付单的关联
	//err = payment.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return payment, err
}
