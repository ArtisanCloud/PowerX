package scrm

import (
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work"
	"gorm.io/gorm"
)

type OrganizationUseCase struct {
	db     *gorm.DB
	wework *work.Work
}

func NewOrganizationUseCase(db *gorm.DB, wework *work.Work) *OrganizationUseCase {
	return &OrganizationUseCase{db: db, wework: wework}
}

func (uc *OrganizationUseCase) SyncEmployee() {
}
