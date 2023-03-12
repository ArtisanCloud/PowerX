package uc

import (
	"context"
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

func (c *ClueUseCase) CreateClues(ctx context.Context, clues []Clue) (id []int64) {
	c.db.WithContext(ctx).Create(clues)
}
