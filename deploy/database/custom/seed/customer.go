package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/model/customerdomain"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"math/rand"
)

func CreateCustomer(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&customerdomain.Customer{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init price book  failed"))
	}

	data := DefaultCustomer()
	if count == 0 {
		if err = db.Model(&customerdomain.Customer{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init customer failed"))
		}
	}
	return err
}

func DefaultCustomer() (customers []*customerdomain.Customer) {

	sourceWechat := UseCaseDD.GetCachedDD(context.Background(), model.TypeSourceChannel, model.ChannelWechat)
	sourceDouyin := UseCaseDD.GetCachedDD(context.Background(), model.TypeSourceChannel, model.ChannelDouYin)

	typePersonal := UseCaseDD.GetCachedDD(context.Background(), customerdomain.TypeCustomerType, customerdomain.CustomerPersonal)
	typeCompany := UseCaseDD.GetCachedDD(context.Background(), customerdomain.TypeCustomerType, customerdomain.CustomerCompany)

	customers = []*customerdomain.Customer{}
	for i := 0; i < 20; i++ {
		sourceC := sourceWechat
		typeC := typeCompany
		if i%2 == 0 {
			sourceC = sourceDouyin
			typeC = typePersonal
		}

		customer := &customerdomain.Customer{
			Name:        fmt.Sprintf("test-%d", rand.Int()),
			Mobile:      fmt.Sprintf("135646742%02d", i),
			Email:       fmt.Sprintf("test@test.com-%02d", i),
			InviterId:   0,
			Source:      sourceC,
			Type:        typeC,
			IsActivated: true,
		}
		customers = append(customers, customer)

	}

	return customers

}
