package metadata

import "gorm.io/gorm"

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type TenantFilter struct {
	TenantUUID string
	Status     string
	Q          string
	Module     string
	Page       int
	PageSize   int
}

func (r *Repository) DB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.db
}
