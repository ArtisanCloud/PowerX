package trade

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/operation"
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/model/powermodel"
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

// -----------------------------//

// Token Exchange Record

// -----------------------------//

func (uc *TokenUseCase) FindAllTokenExchangeRecords(ctx context.Context, opt *FindManyTokensOption) (records []*trade.TokenExchangeRecord, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.TokenExchangeRecord{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&records).Error; err != nil {
		panic(errors.Wrap(err, "find all tokenBalances failed"))
	}
	return records, err
}

func (uc *TokenUseCase) FindManyTokensExchangeRecords(ctx context.Context, opt *FindManyTokensOption) (pageList types.Page[*trade.TokenExchangeRecord], err error) {
	opt.DefaultPageIfNotSet()
	var tokens []*trade.TokenExchangeRecord
	db := uc.db.WithContext(ctx).Model(&trade.TokenExchangeRecord{})

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

	return types.Page[*trade.TokenExchangeRecord]{
		List:      tokens,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *TokenUseCase) CreateTokenExchangeRecord(ctx context.Context, record *trade.TokenExchangeRecord) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&record).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *TokenUseCase) UpsertTokenExchangeRecord(ctx context.Context, record *trade.TokenExchangeRecord) (*trade.TokenExchangeRecord, error) {

	records := []*trade.TokenExchangeRecord{record}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		//_, err := uc.ClearAssociations(tx, records)
		//if err != nil {
		//	return err
		//}

		// 更新产品对象主体
		_, err := uc.UpsertTokenExchangeRecords(ctx, records)
		if err != nil {
			return errors.Wrap(err, "upsert records failed")
		}

		return err
	})

	return record, err
}

func (uc *TokenUseCase) UpsertTokenExchangeRecords(ctx context.Context, records []*trade.TokenExchangeRecord) ([]*trade.TokenExchangeRecord, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.TokenExchangeRecord{}, trade.TokenExchangeRecordId, records, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert tokens failed"))
	}

	return records, err
}

func (uc *TokenUseCase) PatchTokenExchangeRecords(ctx context.Context, id int64, record *trade.TokenExchangeRecord) {
	if err := uc.db.WithContext(ctx).Model(&trade.TokenBalance{}).
		Where(id).Updates(&record).Error; err != nil {
		panic(err)
	}
}

func (uc *TokenUseCase) AddTokenByOrder(ctx context.Context, ticket *operation.TicketRecord, tokenTransactionTypeId int) (*trade.TokenTransaction, error) {
	var err error

	transaction := &trade.TokenTransaction{
		CustomerId: ticket.CustomerId,
		Amount:     ticket.DeductedTokenAmount,
		Type:       tokenTransactionTypeId,
		SourceType: (&operation.TicketRecord{}).GetTableName(false),
		SourceID:   ticket.Id,
	}

	err = uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// 更新Token预约单状态
		err = tx.Model(&trade.TokenReservation{}).
			//Debug().
			Where("source_type =? AND source_id = ?", ticket.GetTableName(false), ticket.Id).
			Update("is_confirmed", true).Error

		// 修改ticket状态为已经完成
		ticket.IsUsed = true
		err = tx.Save(ticket).Error
		if err != nil {
			return err
		}

		// 创建一条代币事务记录
		err = tx.
			//Debug().
			Create(transaction).Error
		if err != nil {
			return err
		}

		return err
	})

	return transaction, err

}

func (uc *TokenUseCase) DeductTokenByTicket(ctx context.Context, ticket *operation.TicketRecord, tokenTransactionTypeId int) (*trade.TokenTransaction, error) {
	var err error

	transaction := &trade.TokenTransaction{
		CustomerId: ticket.CustomerId,
		Amount:     ticket.DeductedTokenAmount,
		Type:       tokenTransactionTypeId,
		SourceType: (&operation.TicketRecord{}).GetTableName(false),
		SourceID:   ticket.Id,
	}

	err = uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		// 更新Token预约单状态
		err = tx.Model(&trade.TokenReservation{}).
			//Debug().
			Where("source_type =? AND source_id = ?", ticket.GetTableName(false), ticket.Id).
			Update("is_confirmed", true).Error

		// 修改ticket状态为已经完成
		ticket.IsUsed = true
		err = tx.Save(ticket).Error
		if err != nil {
			return err
		}

		// 创建一条代币事务记录
		err = tx.
			//Debug().
			Create(transaction).Error
		if err != nil {
			return err
		}

		return err
	})

	return transaction, err

}

// -----------------------------//

// Token Balance

// -----------------------------//

func (uc *TokenUseCase) CreateTokenBalance(ctx context.Context, balance *trade.TokenBalance) error {

	if err := uc.db.WithContext(ctx).
		//Debug().
		Create(&balance).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *TokenUseCase) GetTokenBalance(ctx context.Context, customerId int64) (*trade.TokenBalance, error) {
	var balance = &trade.TokenBalance{}
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		Where("customer_id = ?", customerId).
		First(balance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到产品")
		}
		return nil, err
	}

	return balance, nil
}

func (uc *TokenUseCase) UpsertTokenBalance(ctx context.Context, record *trade.TokenExchangeRecord) (*trade.TokenExchangeRecord, error) {

	records := []*trade.TokenExchangeRecord{record}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除产品的相关联对象
		//_, err := uc.ClearAssociations(tx, records)
		//if err != nil {
		//	return err
		//}

		// 更新产品对象主体
		_, err := uc.UpsertTokenExchangeRecords(ctx, records)
		if err != nil {
			return errors.Wrap(err, "upsert records failed")
		}

		return err
	})

	return record, err
}

func (uc *TokenUseCase) UpsertTokenBalances(ctx context.Context, records []*trade.TokenExchangeRecord) ([]*trade.TokenExchangeRecord, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.TokenExchangeRecord{}, trade.TokenBalanceUniqueId, records, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert tokens failed"))
	}

	return records, err
}

// -----------------------------//

// Token Reservation

// -----------------------------//

func (uc *TokenUseCase) CheckTokenBalanceIsEnough(ctx context.Context, customer *customerdomain.Customer) (*trade.TokenBalance, int64, error) {
	// 当前余额
	balance, err := uc.GetTokenBalance(ctx, customer.Id)
	if err != nil {
		return nil, 0, err
	}
	// 使用代币状态
	usedToken, err := uc.GetUsedTokens(ctx, customer.Id)
	if err != nil {
		return nil, 0, err
	}
	balance.Usage = usedToken

	// 预扣款代币状态
	var unusedTicketsCount int64
	if err := uc.db.WithContext(ctx).Model(&operation.TicketRecord{}).
		//Debug().
		Select("count(ticket_records.id) as unused_tickets_count").
		Where("ticket_records.customer_id = ? AND ticket_records.is_used = ?", customer.Id, false).
		Joins("LEFT JOIN robot_tasks ON ticket_records.job_id = robot_tasks.job_id").
		Where("ticket_records.is_used = ?", false).
		Count(&unusedTicketsCount).
		Error; err != nil {
		return nil, 0, err
	}

	if balance.Balance < 1 && unusedTicketsCount < 1 {
		if balance.Balance < 1 {
			return balance, unusedTicketsCount, errorx.ErrNotEnoughBalance
		}
		if unusedTicketsCount < 1 {
			return balance, unusedTicketsCount, errorx.ErrNotEnoughTicket
		}
	}

	return balance, unusedTicketsCount, nil
}

func (uc *TokenUseCase) CreateReservedTokenByTicket(ctx context.Context,
	ticket *operation.TicketRecord, balance *trade.TokenBalance,
) error {
	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 创建一条Ticket
		err := tx.Create(ticket).Error
		if err != nil {
			return err
		}

		// 同时预扣款Token
		err = tx.Create(&trade.TokenReservation{
			CustomerId:  ticket.CustomerId,
			Amount:      ticket.DeductedTokenAmount,
			SourceType:  ticket.GetTableName(false),
			SourceID:    ticket.Id,
			IsConfirmed: false,
		}).Error
		if err != nil {
			return err
		}

		// 扣除余额，并且保存
		balance.Balance = balance.Balance - ticket.DeductedTokenAmount
		err = tx.Save(balance).Error

		return err
	})

	return err
}

func (uc *TokenUseCase) ReuseTicketForTask(ctx context.Context,
	customer *customerdomain.Customer,
	oldTicket *operation.TicketRecord, newTicket *operation.TicketRecord,
	reservedToken *trade.TokenReservation, balance *trade.TokenBalance,
) error {
	err := uc.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 之前对应的Ticket，需要被作废，delete
		err := tx.Delete(&operation.TicketRecord{}, oldTicket.Id).Error
		if err != nil {
			return err
		}

		// 创建一条Ticket
		err = tx.Create(newTicket).Error
		if err != nil {
			return err
		}

		// 同时Token预扣记录更新
		reservedToken.SourceType = newTicket.GetTableName(false)
		reservedToken.SourceID = newTicket.Id
		err = tx.Save(reservedToken).Error
		if err != nil {
			return err
		}

		return err
	})

	return err

}

func (uc *TokenUseCase) GetUsedTokens(ctx context.Context, customerId int64) (float64, error) {
	type result struct {
		Total float64 `gorm:"column:amount_used"`
	}
	var tokenReserved = &result{}
	db := uc.db.WithContext(ctx)
	if err := db.Model(&trade.TokenReservation{}).
		Select("customer_id, sum(amount) as amount_used").
		//Debug().
		Where("customer_id = ? AND is_confirmed = ?", customerId, true).
		Group("customer_id").
		Limit(1).
		Scan(tokenReserved).Error; err != nil {
		return 0, err
	}

	return tokenReserved.Total, nil
}

func (uc *TokenUseCase) GetReservedTokenByTicket(ctx context.Context, ticket *operation.TicketRecord) (*trade.TokenReservation, error) {

	reservation := &trade.TokenReservation{}
	err := uc.db.WithContext(ctx).Model(&trade.TokenReservation{}).
		Debug().
		Where("source_type =? AND source_id = ?", ticket.GetTableName(false), ticket.Id).
		First(reservation).Error
	if err != nil {
		return nil, err
	}
	return reservation, nil
}

func (uc *TokenUseCase) GetReservedTokens(ctx context.Context, customerId int64) (float64, error) {
	type result struct {
		Total float64 `gorm:"column:amount_reserved"`
	}
	var tokenReserved = &result{}
	db := uc.db.WithContext(ctx)
	if err := db.Model(&trade.TokenReservation{}).
		Select("customer_id, sum(amount) as amount_reserved").
		//Debug().
		Where("customer_id = ? AND is_confirmed = ?", customerId, false).
		Group("customer_id").
		Limit(1).
		Scan(tokenReserved).Error; err != nil {
		return 0, err
	}

	return tokenReserved.Total, nil
}

func (uc *TokenUseCase) ClearAssociations(db *gorm.DB, balance *trade.TokenBalance) (*trade.TokenBalance, error) {
	var err error
	// 清除订单的关联
	//err = order.ClearArtisans(db)
	//if err != nil {
	//	return nil, err
	//}

	return balance, err
}
