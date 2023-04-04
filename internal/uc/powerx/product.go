package powerx

import (
	"context"
	"gorm.io/gorm"
)

type ProductUseCase struct {
	db *gorm.DB
}

func newProductUseCase(db *gorm.DB) *ProductUseCase {
	return &ProductUseCase{
		db: db,
	}
}

func (c *ProductUseCase) CreateProducts(ctx context.Context) {

}
