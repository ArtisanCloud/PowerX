package powerx

import (
	"PowerX/internal/types"
	"context"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ContactUseCase struct {
	db *gorm.DB
}

func newContactUseCase(db *gorm.DB) *ContactUseCase {
	return &ContactUseCase{
		db: db,
	}
}

type LiveQRCode struct {
	Uid          string `gorm:"unique"`
	RedirectTo   string
	IconUrl      string
	AccessNumber int64
	CreateBy     int64
	*types.Model
}

func (c *ContactUseCase) CreateLiveQRCode(ctx context.Context, code *LiveQRCode) {
	code.Uid = uuid.New().String()
	if err := c.db.WithContext(ctx).Model(&LiveQRCode{}).Create(&code).Error; err != nil {
		panic(errors.Wrap(err, "create live qrcode failed"))
	}
}

func (c *ContactUseCase) GetLiveQRCode(ctx context.Context, uid string) *LiveQRCode {
	var code LiveQRCode
	if err := c.db.WithContext(ctx).Model(&LiveQRCode{}).Where("uid = ?", uid).Find(&code).Error; err != nil {
		panic(errors.Wrap(err, "find live qrcode failed"))
	}
	return &code
}
