package operation

import (
	"PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/crm/operation"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"strings"
)

type MembershipUseCase struct {
	DB *gorm.DB
}

func NewMembershipUseCase(db *gorm.DB) *MembershipUseCase {
	return &MembershipUseCase{
		DB: db,
	}
}

type FindManyMembershipsOption struct {
	LikeName   string
	LikeMobile string
	Mobile     string
	Statuses   []int
	Sources    []int
	OrderBy    string
	types.PageEmbedOption
}

func (uc *MembershipUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyMembershipsOption) *gorm.DB {
	if opt.LikeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeName+"%")
	}

	if len(opt.Statuses) > 0 {
		db = db.Where("status IN ?", opt.Statuses)
	}

	orderBy := "id desc"
	if opt.OrderBy != "" {
		orderBy = opt.OrderBy + "," + orderBy
	}
	db.Order(orderBy)

	return db
}

func (uc *MembershipUseCase) FindManyMemberships(ctx context.Context, opt *FindManyMembershipsOption) (pageList types.Page[*operation.Membership], err error) {
	var memberships []*operation.Membership
	db := uc.DB.WithContext(ctx).Model(&operation.Membership{})

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
		Find(&memberships).Error; err != nil {
		panic(err)
	}

	return types.Page[*operation.Membership]{
		List:      memberships,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *MembershipUseCase) CreateMembership(ctx context.Context, membership *operation.Membership) error {
	if err := uc.DB.WithContext(ctx).Create(&membership).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrDuplicatedInsert, "该对象不能重复创建")
		}
		panic(err)
	}
	return nil
}

func (uc *MembershipUseCase) UpsertMembership(ctx context.Context, m *operation.Membership) (*operation.Membership, error) {

	arrayMembership := []*operation.Membership{m}

	err := uc.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := powermodel.UpsertModelsOnUniqueID(tx, &operation.Membership{}, operation.MembershipUniqueId, arrayMembership, nil, false)

		if err != nil {
			panic(errors.Wrap(err, "upsert membershipdomain failed"))
		}

		return err
	})

	return m, err
}

func (uc *MembershipUseCase) UpsertMemberships(ctx context.Context, memberships []*operation.Membership) ([]*operation.Membership, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.DB.WithContext(ctx), &operation.Membership{}, operation.MembershipUniqueId, memberships, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert memberships failed"))
	}

	return memberships, err
}

func (uc *MembershipUseCase) UpdateMembership(ctx context.Context, id int64, m *operation.Membership) error {
	//fmt.Dump(membership)
	err := uc.DB.WithContext(ctx).Model(&operation.Membership{}).
		//Debug().
		Where(id).Updates(m).Error

	return err
}

func (uc *MembershipUseCase) GetMembership(ctx context.Context, id int64) (*operation.Membership, error) {
	var membership operation.Membership
	if err := uc.DB.WithContext(ctx).First(&membership, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到会籍")
		}
		panic(err)
	}
	return &membership, nil
}

func (uc *MembershipUseCase) GetMembershipBy(ctx context.Context, customer *customerdomain.Customer, membershipTypeId int64) (*operation.Membership, error) {
	var membership operation.Membership
	if err := uc.DB.WithContext(ctx).
		Where("customer_id = ? and type =?", customer.Id, membershipTypeId).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		panic(err)
	}
	return &membership, nil

}
func (uc *MembershipUseCase) GetMembershipByOrderId(ctx context.Context, inviteCode string) (*operation.Membership, error) {
	var membership operation.Membership
	if err := uc.DB.WithContext(ctx).
		Where("order_id", inviteCode).
		First(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到会籍")
		}
		panic(err)
	}
	return &membership, nil
}

func (uc *MembershipUseCase) DeleteMembership(ctx context.Context, id int64) error {
	result := uc.DB.WithContext(ctx).Delete(&operation.Membership{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到会籍")
	}
	return nil
}
