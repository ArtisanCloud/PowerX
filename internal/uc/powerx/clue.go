package powerx

import (
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ClueUseCase struct {
	db *gorm.DB
}

type Clue struct {
	Title       string
	PhoneNumber string
	Email       string
	Source      string
	Status      string
	*gorm.Model
}

func (c *ClueUseCase) CreateClue(ctx context.Context, clue *Clue) {
	if err := c.db.WithContext(ctx).Create(&clue).Error; err != nil {
		panic(errors.Wrap(err, "create clues failed"))
	}
}
