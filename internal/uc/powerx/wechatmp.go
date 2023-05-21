package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/miniProgram"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// WechatMiniProgramUseCase MiniProgram Use Case
type WechatMiniProgramUseCase struct {
	App *miniProgram.MiniProgram
	db  *gorm.DB
}

func NewWechatMiniProgramUseCase(db *gorm.DB, conf *config.Config) *WechatMiniProgramUseCase {
	// 初始化微信小程序API SDK
	app, err := miniProgram.NewMiniProgram(&miniProgram.UserConfig{
		AppID:  conf.WechatMP.AppId,
		Secret: conf.WechatMP.Secret,
		OAuth: miniProgram.OAuth{
			Callback: "https://wechat-mp.artisan-cloud.com/callback",
			Scopes:   nil,
		},
		//Token:     "Aj9T3rkHmbzCnpoUgRO3mPgkxFV",
		AESKey:    conf.WechatMP.AESKey,
		HttpDebug: true,
	})

	if err != nil {
		panic(errors.Wrap(err, "miniprogram init failed"))
	}

	return &WechatMiniProgramUseCase{
		App: app,
		db:  db,
	}
}

func (uc *WechatMiniProgramUseCase) buildFindQueryNoPage(query *gorm.DB, opt *model.FindMPCustomerOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	//if len(opt.Names) > 0 {
	//	query.Where("name in ?", opt.Names)
	//} else if opt.LikeName != "" {
	//	query.Where("name like ?", fmt.Sprintf("%s%%", opt.LikeName))
	//}
	//if len(opt.Emails) > 0 {
	//	query.Where("email in ?", opt.Emails)
	//} else if opt.LikeEmail != "" {
	//	query.Where("email like ?", fmt.Sprintf("%s%%", opt.LikeEmail))
	//}
	//if len(opt.PhoneNumbers) > 0 {
	//	query.Where("mobile_phone in ?")
	//} else if opt.LikePhoneNumber != "" {
	//	query.Where("mobile_phone like ?", fmt.Sprintf("%s%%", opt.LikePhoneNumber))
	//}
	if len(opt.OpenIds) > 0 {
		query.Where("open_id in ?", opt.OpenIds)
	}
	//if len(opt.Accounts) > 0 {
	//	query.Where("account in ?", opt.Accounts)
	//}
	//if len(opt.DepIds) > 0 {
	//	query.Where("? && department_ids", pq.Int64Array(opt.DepIds))
	//}
	//if len(opt.Statuses) > 0 {
	//	query.Where("status in ?", opt.Statuses)
	//}
	return query
}

func (uc *WechatMiniProgramUseCase) FindManyMPCustomers(ctx context.Context, opt *model.FindMPCustomerOption) types.Page[*model.WechatMPCustomer] {
	var mpCustomers []*model.WechatMPCustomer
	var count int64
	query := uc.db.WithContext(ctx).Model(&model.WechatMPCustomer{})

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
	return types.Page[*model.WechatMPCustomer]{
		List:      mpCustomers,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}
}

func (uc *WechatMiniProgramUseCase) FindOneMPCustomer(ctx context.Context, opt *model.FindMPCustomerOption) (*model.WechatMPCustomer, error) {
	var mpCustomer *model.WechatMPCustomer
	query := uc.db.WithContext(ctx).Model(&model.WechatMPCustomer{})
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

func (uc *WechatMiniProgramUseCase) UpsertMPCustomer(ctx context.Context, customer *model.WechatMPCustomer) (*model.WechatMPCustomer, error) {

	mpCustomers := []*model.WechatMPCustomer{customer}

	_, err := uc.UpsertMPCustomers(ctx, mpCustomers)
	if err != nil {
		panic(errors.Wrap(err, "upsert mp customers failed"))
	}

	return customer, err
}

func (uc *WechatMiniProgramUseCase) UpsertMPCustomers(ctx context.Context, customers []*model.WechatMPCustomer) ([]*model.WechatMPCustomer, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &model.WechatMPCustomer{}, model.WechatMpCustomerUniqueId, customers, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert mp customers failed"))
	}

	return customers, err
}
