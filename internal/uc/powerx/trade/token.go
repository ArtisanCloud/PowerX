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

type TokenUseCase struct {
	db *gorm.DB
}

func NewTokenUseCase(db *gorm.DB) *TokenUseCase {
	return &TokenUseCase{
		db: db,
	}
}

type FindManyTokensOption struct {
	CustomerId int64

	OrderBy string
	types.PageEmbedOption
}

func (uc *TokenUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyTokensOption) *gorm.DB {

	if opt.CustomerId > 0 {
		db = db.Where("customer_id = ?", opt.CustomerId)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *TokenUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	//db = db.
	//Preload("Items.ProductBookEntry.SKU").
	return db
}

func (uc *TokenUseCase) FindAllTokenBalances(ctx context.Context, opt *FindManyTokensOption) (tokenBalances []*trade.TokenBalance, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.TokenBalance{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&tokenBalances).Error; err != nil {
		panic(errors.Wrap(err, "find all tokenBalances failed"))
	}
	return tokenBalances, err
}

func (uc *TokenUseCase) FindManyTokens(ctx context.Context, opt *FindManyTokensOption) (pageList types.Page[*trade.TokenBalance], err error) {
	opt.DefaultPageIfNotSet()
	var tokens []*trade.TokenBalance
	db := uc.db.WithContext(ctx).Model(&trade.TokenBalance{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		Find(&tokens).Error; err != nil {
		panic(err)
	}

	return types.Page[*trade.TokenBalance]{
		List:      tokens,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *TokenUseCase) CreateToken(ctx context.Context, order *trade.TokenBalance) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&order).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *TokenUseCase) UpsertToken(ctx context.Context, order *trade.TokenBalance) (*trade.TokenBalance, error) {

	tokens := []*trade.TokenBalance{order}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		_, err := uc.ClearAssociations(tx, order)
		if err != nil {
			return err
		}

		// 更新产品对象主体
		_, err = uc.UpsertTokens(ctx, tokens)
		if err != nil {
			return errors.Wrap(err, "upsert order failed")
		}

		return err
	})

	return order, err
}

func (uc *TokenUseCase) UpsertTokens(ctx context.Context, tokens []*trade.TokenBalance) ([]*trade.TokenBalance, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.TokenBalance{}, trade.TokenBalanceUniqueId, tokens, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert tokens failed"))
	}

	return tokens, err
}

func (uc *TokenUseCase) PatchToken(ctx context.Context, id int64, order *trade.TokenBalance) {
	if err := uc.db.WithContext(ctx).Model(&trade.TokenBalance{}).
		Where(id).Updates(&order).Error; err != nil {
		panic(err)
	}
}

func (uc *TokenUseCase) GetTokenBalance(ctx context.Context, id int64) (*trade.TokenBalance, error) {
	var order = &trade.TokenBalance{}
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		panic(err)
	}

	return order, nil
}

func (uc *TokenUseCase) ClearAssociations(db *gorm.DB, order *trade.TokenBalance) (*trade.TokenBalance, error) {
	var err error
	// 清除订单的关联
	//err = order.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return order, err
}
