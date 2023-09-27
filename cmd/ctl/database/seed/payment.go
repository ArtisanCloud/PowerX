package seed

import (
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/uc/powerx"
	trade2 "PowerX/internal/uc/powerx/trade"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"time"
)

func CreatePayments(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&trade.Payment{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init payment failed"))
	}

	data := DefaultPayment(db)
	if count == 0 {
		if err = db.Model(&trade.Payment{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price category failed"))
		}

	}
	return err
}

func DefaultPayment(db *gorm.DB) (data []*trade.Payment) {

	ucOrder := trade2.NewOrderUseCase(db)
	ucDD := powerx.NewDataDictionaryUseCase(db)

	orderStatusToBePaid := ucDD.GetCachedDD(context.Background(), trade.TypePaymentStatus, trade.PaymentStatusPaid)

	res, err := ucOrder.FindManyOrders(context.Background(), &trade2.FindManyOrdersOption{})
	if err != nil {
		return nil
	}

	//now := carbon.Now()
	data = []*trade.Payment{}

	for i := 0; i < len(res.List); i++ {

		if i%2 == 1 {
			continue
		}
		order := res.List[i]
		paymentStatus := int(orderStatusToBePaid.Id)

		seedPayment := &trade.Payment{
			OrderId:         order.Id,
			PaymentDate:     time.Now(),
			PaymentType:     order.PaymentType,
			PaidAmount:      order.ListPrice,
			PaymentNumber:   trade.GeneratePaymentNumber(),
			ReferenceNumber: trade.GeneratePaymentNumber(),
			Remark:          fmt.Sprintf("payment id for %d", order.Id),
			Status:          paymentStatus,
		}

		seedPayment.Items = DefaultPaymentItems(seedPayment)

		data = append(data, seedPayment)
	}

	return data
}

func DefaultPaymentItems(payment *trade.Payment) []*trade.PaymentItem {

	items := []*trade.PaymentItem{
		{
			Quantity:            1,
			UnitPrice:           payment.PaidAmount,
			PaymentCustomerName: "Test",
			BankInformation:     "xxx",
			BankResponseCode:    "123",
			CarrierType:         "微信",
			CreditCardNumber:    "11111",
			DeductMembershipId:  "",
			DeductionPoint:      0,
			InvoiceCreateTime:   time.Now(),
			InvoiceNumber:       "xxxxx",
			InvoiceTotalAmount:  payment.PaidAmount,
		},
	}
	return items

}
