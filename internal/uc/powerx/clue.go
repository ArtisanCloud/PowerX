package powerx

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"gorm.io/gorm"
)

type ClueUseCase struct {
	db *gorm.DB
}

func NewClueUseCase(db *gorm.DB) *ClueUseCase {
	return &ClueUseCase{db: db}
}

type Clue struct {
	types.Model
	Title       string
	PhoneNumber string
	Email       string
	Source      string
	Status      string
}

func (c *ClueUseCase) CreateClue(ctx context.Context, clue *Clue) {
	if err := c.db.WithContext(ctx).Create(clue).Error; err != nil {
		panic(err)
	}
}

func (c *ClueUseCase) GetClue(ctx context.Context, id uint) (clue *Clue, err error) {
	if err := c.db.WithContext(ctx).First(&clue, id).Error; err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return
}

func (c *ClueUseCase) UpdateClue(ctx context.Context, clue *Clue) {
	if err := c.db.WithContext(ctx).Save(clue).Error; err != nil {
		panic(err)
	}
}

func (c *ClueUseCase) DeleteClue(ctx context.Context, id uint) error {
	result := c.db.WithContext(ctx).Delete(&Clue{}, id)
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到线索")
	}
	return nil
}
