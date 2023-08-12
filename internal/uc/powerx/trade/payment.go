package trade

import (
	"PowerX/internal/config"
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx"
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/payment"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/payment/order/request"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/payment/order/response"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type PaymentUseCase struct {
	db        *gorm.DB
	WXPayment *payment.Payment
}

const WXCurrencyUnit = 100

func NewPaymentUseCase(db *gorm.DB, conf *config.Config) *PaymentUseCase {

	// 初始化微信公众号API SDK
	wxPayment, err := payment.NewPayment(&payment.UserConfig{
		AppID:            conf.WechatPay.AppId,
		MchID:            conf.WechatPay.MchId,
		MchApiV3Key:      conf.WechatPay.MchApiV3Key,
		Key:              conf.WechatPay.Key,
		CertPath:         conf.WechatPay.CertPath,
		KeyPath:          conf.WechatPay.KeyPath,
		RSAPublicKeyPath: conf.WechatPay.RSAPublicKeyPath,
		SerialNo:         conf.WechatPay.SerialNo,
		Http: payment.Http{
			Timeout: 30.0,
			BaseURI: "https://api.mch.weixin.qq.com",
		},
		NotifyURL: conf.WechatPay.NotifyUrl,
		HttpDebug: conf.WechatPay.HttpDebug,
		Debug:     false,
	})

	if err != nil {
		panic(errors.Wrap(err, "wechat payment init failed"))
	}

	return &PaymentUseCase{
		db:        db,
		WXPayment: wxPayment,
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

func (uc *PaymentUseCase) PreloadItems(db *gorm.DB) *gorm.DB {
	db = db.Preload("Items")
	return db
}

func (uc *PaymentUseCase) FindAllPayments(ctx context.Context, opt *FindManyPaymentsOption) (payments []*trade.Payment, err error) {
	query := uc.db.WithContext(ctx).Model(&trade.Payment{})

	query = uc.buildFindQueryNoPage(query, opt)
	query = uc.PreloadItems(query)
	if err := query.
		//Debug().
		Find(&payments).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return payments, err
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

	db = uc.PreloadItems(db)
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

func (uc *PaymentUseCase) CreatePaymentFromOrderByWechat(ctx context.Context,
	customer *customerdomain2.Customer, order *trade.Order,
	openId string, paymentType int,
) (payment *trade.Payment, data interface{}, err error) {

	db := uc.db.WithContext(ctx)

	paymentStatusId := uc.GetPaymentStatusId(ctx, trade.PaymentStatusPending)
	// 创建支付单
	payment = uc.MakePaymentFromOrder(customer, order, paymentType, paymentStatusId)
	err = db.Transaction(func(tx *gorm.DB) error {
		// 保存支付单
		err = tx.Model(trade.Payment{}).
			//Debug().
			Create(payment).Error
		if err != nil {
			return err
		}

		// 创建微信支付单
		rsOrder, err := uc.MakeWechatOrder(ctx, payment, openId)
		if err != nil {
			return err
		}

		if rsOrder.PrepayID == "" {
			err = errors.New("no Prepay Id generated")
			return err
		}

		// config wx Bridge for front end
		data, err = uc.WXPayment.JSSDK.BridgeConfig(rsOrder.PrepayID, false)

		return err
	})

	return payment, data, err
}

func (uc *PaymentUseCase) MakePaymentFromOrder(customer *customerdomain2.Customer, order *trade.Order, paymentType int, paymentStatus int) (payment *trade.Payment) {
	payment = &trade.Payment{
		OrderId:       order.Id,
		PaymentType:   paymentType,
		PaidAmount:    order.UnitPrice,
		PaymentNumber: trade.GeneratePaymentNumber(),
		Status:        paymentStatus,
	}

	quantityOfItems := len(order.Items)
	paymentItems := []*trade.PaymentItem{
		{
			Quantity:            quantityOfItems,
			UnitPrice:           order.UnitPrice,
			PaymentCustomerName: customer.Name,
		},
	}
	payment.Items = paymentItems

	return payment
}

func (uc *PaymentUseCase) MakeWechatOrder(ctx context.Context, payment *trade.Payment, openID string) (*response.ResponseUnitfy, error) {

	description := fmt.Sprintf("%f-%s", payment.PaidAmount, payment.PaymentNumber)
	if payment.Remark != "" {
		description = payment.Remark
	}

	mapObject := &request.RequestJSAPIPrepay{
		Amount: &request.JSAPIAmount{
			Total: int(payment.PaidAmount * WXCurrencyUnit),
			//Total: int(0.3 * WXCurrencyUnit),
			//Currency: "CNY",
		},
		Attach:      "订单支付",
		Description: description,
		OutTradeNo:  payment.PaymentNumber,
		Payer: &request.JSAPIPayer{
			OpenID: openID,
		},
	}
	mapObject.SetNotifyUrl(uc.WXPayment.Config.GetString("notify_url", ""))
	//mapObject.SetAppID(service.App.Config.GetString("app_id", ""))

	return uc.WXPayment.Order.JSAPITransaction(ctx, mapObject)

}

func (uc *PaymentUseCase) CreatePayment(ctx context.Context, payment *trade.Payment) error {

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

func (uc *PaymentUseCase) UpsertPayment(ctx context.Context, payment *trade.Payment) (*trade.Payment, error) {

	payments := []*trade.Payment{payment}

	err := uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除支付单的相关联对象
		_, err := uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		// 更新支付单对象主体
		_, err = uc.UpsertPayments(ctx, payments)
		if err != nil {
			return errors.Wrap(err, "upsert payment failed")
		}

		return err
	})

	return payment, err
}

func (uc *PaymentUseCase) UpsertPayments(ctx context.Context, payments []*trade.Payment) ([]*trade.Payment, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &trade.Payment{}, trade.OrderUniqueId, payments, nil, false)

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
	var p = &trade.Payment{}
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	if err := db.
		//Debug().
		First(p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到支付单")
		}
		panic(err)
	}

	return p, nil
}

func (uc *PaymentUseCase) GetPaymentByNumber(ctx context.Context, paymentNumber string) (*trade.Payment, error) {
	var p = &trade.Payment{}
	db := uc.db.WithContext(ctx)
	db = uc.PreloadItems(db)
	err := db.
		Preload("Order").
		Where("payment_number", paymentNumber).
		First(p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到支付单")
		}
		panic(err)
	}

	return p, nil
}

func (uc *PaymentUseCase) DeletePayment(ctx context.Context, id int64) error {

	// 获取支付单相关项
	payment, err := uc.GetPayment(ctx, id)
	if err != nil {
		return errorx.ErrNotFoundObject
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		// 删除支付单相关项
		_, err = uc.ClearAssociations(tx, payment)
		if err != nil {
			return err
		}

		result := tx.Delete(&trade.Payment{}, id)
		if err := result.Error; err != nil {
			panic(err)
		}
		if result.RowsAffected == 0 {
			return errorx.WithCause(errorx.ErrBadRequest, "未找到支付单")
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

func (uc *PaymentUseCase) ChangePaymentStatusPaid(ctx context.Context, payment *trade.Payment) (*trade.Payment, error) {
	db := uc.db.WithContext(ctx)

	payment.Status = uc.GetPaymentStatusId(ctx, trade.PaymentStatusPaid)

	err := db.Save(payment).Error

	return payment, err
}

func (uc *PaymentUseCase) IsPaymentTypeSameAs(ctx context.Context, payment *trade.Payment, paymentType string) bool {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)

	return payment.PaymentType == ucDD.GetCachedDDId(ctx, trade.TypePaymentType, paymentType)

}

func (uc *PaymentUseCase) IsPaymentStatusSameAs(ctx context.Context, payment *trade.Payment, paymentStatus string) bool {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)

	return payment.Status == ucDD.GetCachedDDId(ctx, trade.TypePaymentStatus, paymentStatus)

}

func (uc *PaymentUseCase) GetPaymentTypeId(ctx context.Context, paymentType string) (paymentTypeId int) {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)
	paymentTypeId = ucDD.GetCachedDDId(ctx, trade.TypePaymentType, paymentType)
	return paymentTypeId
}
func (uc *PaymentUseCase) GetPaymentStatusId(ctx context.Context, paymentStatus string) (paymentStatusId int) {
	ucDD := powerx.NewDataDictionaryUseCase(uc.db)
	paymentStatusId = ucDD.GetCachedDDId(ctx, trade.TypePaymentStatus, paymentStatus)
	return paymentStatusId
}
