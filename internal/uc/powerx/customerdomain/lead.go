package customerdomain

import (
	"PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type LeadUseCase struct {
	db *gorm.DB
}

func NewLeadUseCase(db *gorm.DB) *LeadUseCase {
	return &LeadUseCase{
		db: db,
	}
}

func (uc *LeadUseCase) CreateLead(ctx context.Context, lead *customerdomain.Lead) {
	if err := uc.db.WithContext(ctx).Create(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *LeadUseCase) UpsertLead(ctx context.Context, lead *customerdomain.Lead) (*customerdomain.Lead, error) {

	leads := []*customerdomain.Lead{lead}

	_, err := uc.UpsertLeads(ctx, leads)
	if err != nil {
		panic(errors.Wrap(err, "upsert lead failed"))
	}

	return lead, err
}

func (uc *LeadUseCase) UpsertLeads(ctx context.Context, leads []*customerdomain.Lead) ([]*customerdomain.Lead, error) {

	err := powermodel.UpsertModelsOnUniqueID(uc.db.WithContext(ctx), &customerdomain.Lead{}, customerdomain.LeadUniqueId, leads, nil, false)

	if err != nil {
		panic(errors.Wrap(err, "batch upsert leads failed"))
	}

	return leads, err
}

func (uc *LeadUseCase) PatchLead(ctx context.Context, id int64, lead *customerdomain.Lead) {
	if err := uc.db.WithContext(ctx).Model(&customerdomain.Lead{}).Where(id).Updates(&lead).Error; err != nil {
		panic(err)
	}
}

func (uc *LeadUseCase) GetLead(ctx context.Context, id int64) (*customerdomain.Lead, error) {
	var lead customerdomain.Lead
	if err := uc.db.WithContext(ctx).First(&lead, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
		}
		panic(err)
	}
	return &lead, nil
}

func (uc *LeadUseCase) DeleteLead(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Delete(&customerdomain.Lead{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return nil
}
