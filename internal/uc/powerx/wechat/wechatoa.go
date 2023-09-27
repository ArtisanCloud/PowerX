package wechat

import (
	"PowerX/internal/config"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/wechat"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/officialAccount"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// WechatOfficialAccountUseCase Official Account Use Case
type WechatOfficialAccountUseCase struct {
	App *officialAccount.OfficialAccount
	db  *gorm.DB
}

func NewWechatOfficialAccountUseCase(db *gorm.DB, conf *config.Config) *WechatOfficialAccountUseCase {
	// 初始化微信公众号API SDK
	app, err := officialAccount.NewOfficialAccount(&officialAccount.UserConfig{
		AppID:  conf.WechatOA.AppId,
		Secret: conf.WechatOA.Secret,
		OAuth: officialAccount.OAuth{
			Callback: conf.WechatOA.OAuth.Callback,
			Scopes:   conf.WechatOA.OAuth.Scopes,
		},
		AESKey:    conf.WechatOA.AESKey,
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "official account init failed"))
	}

	return &WechatOfficialAccountUseCase{
		App: app,
		db:  db,
	}
}

type FindOACustomerOption struct {
	Ids             []int64
	SessionKey      string
	OpenIds         []string
	UnionIds        []string
	PhoneNumbers    []string
	PhoneNumberLike string
	NickNames       []string
	NickNameLike    string
	Gender          int64
	Country         string
	Province        string
	City            string
	//Statuses        []OACustomerStatus
	PageIndex int
	PageSize  int
}

func (uc *WechatOfficialAccountUseCase) buildFindQueryNoPage(query *gorm.DB, opt *FindOACustomerOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}

	if len(opt.OpenIds) > 0 {
		query.Where("open_id in ?", opt.OpenIds)
	}

	return query
}

func (uc *WechatOfficialAccountUseCase) FindManyOACustomers(ctx context.Context, opt *FindOACustomerOption) types.Page[*wechat.WechatOACustomer] {
	var mpCustomers []*wechat.WechatOACustomer
	var count int64
	query := uc.db.WithContext(ctx).Model(&wechat.WechatOACustomer{})

	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find mpCustomers failed"))
	}
	if err := query.Find(&mpCustomers).Error; err != nil {
		panic(errors.Wrap(err, "find mpCustomers failed"))
	}
	return types.Page[*wechat.WechatOACustomer]{
		List:      mpCustomers,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}
}

func (uc *WechatOfficialAccountUseCase) FindOneOACustomer(ctx context.Context, opt *FindOACustomerOption) (*wechat.WechatOACustomer, error) {
	var mpCustomer *wechat.WechatOACustomer
	query := uc.db.WithContext(ctx).Model(&wechat.WechatOACustomer{})
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		Preload("Customer").
		First(&mpCustomer).Error; err != nil {
		return nil, errorx.ErrRecordNotFound
	}
	return mpCustomer, nil
}

func (uc *WechatOfficialAccountUseCase) UpsertOACustomer(ctx context.Context, customer *wechat.WechatOACustomer) (*wechat.WechatOACustomer, error) {

	mpCustomers := []*wechat.WechatOACustomer{customer}

	_, err := uc.UpsertOACustomers(ctx, mpCustomers)
	if err != nil {
		panic(errors.Wrap(err, "upsert mp customers failed"))
	}

	return customer, err
}

func (uc *WechatOfficialAccountUseCase) UpsertOACustomers(ctx context.Context, customers []*wechat.WechatOACustomer) ([]*wechat.WechatOACustomer, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &wechat.WechatOACustomer{}, wechat.WechatMpCustomerUniqueId, customers, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert mp customers failed"))
	}

	return customers, err
}
