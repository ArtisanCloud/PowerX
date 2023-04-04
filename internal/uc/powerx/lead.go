package powerx

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"gorm.io/gorm"
)

type LeadUseCase struct {
	db *gorm.DB
}

func NewLeadUseCase(db *gorm.DB) *LeadUseCase {
	return &LeadUseCase{db: db}
}

type Lead struct {
	types.Model
	Title       string
	PhoneNumber string
	Email       string
	Source      string
	Status      string
}

func (c *LeadUseCase) CreateClue(ctx context.Context, clue *Lead) {
	if err := c.db.WithContext(ctx).Create(clue).Error; err != nil {
		panic(err)
	}
}

func (c *LeadUseCase) GetClue(ctx context.Context, id uint) (clue *Lead, err error) {
	if err := c.db.WithContext(ctx).First(&clue, id).Error; err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return
}

func (c *LeadUseCase) UpdateClue(ctx context.Context, clue *Lead) {
	if err := c.db.WithContext(ctx).Save(clue).Error; err != nil {
		panic(err)
	}
}

func (c *LeadUseCase) DeleteClue(ctx context.Context, id uint) error {
	result := c.db.WithContext(ctx).Delete(&Lead{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return nil
}
