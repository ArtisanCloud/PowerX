package iam

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"

	"gorm.io/gorm"
)

type CredentialRepository struct {
	*repository.BaseRepository[dbmodel.Credential]
	db *gorm.DB
}

func NewCredentialRepository(db *gorm.DB) *CredentialRepository {
	return &CredentialRepository{
		BaseRepository: repository.NewBaseRepository[dbmodel.Credential](db),
		db:             db,
	}
}

func (r *CredentialRepository) Create(ctx context.Context, c *dbmodel.Credential) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *CredentialRepository) Upsert(ctx context.Context, c *dbmodel.Credential, fields ...string) error {
	// 简易实现：先查再更新/创建（可改为 OnConflict）
	var old dbmodel.Credential
	err := r.db.WithContext(ctx).
		Where("provider = ? AND identifier = ?", c.Provider, c.Identifier).
		First(&old).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.db.WithContext(ctx).Create(c).Error
		}
		return err
	}
	c.ID = old.ID
	tx := r.db.WithContext(ctx).Model(&c)
	if len(fields) > 0 {
		tx = tx.Select(fields)
	}
	return tx.Updates(c).Error
}

func (r *CredentialRepository) GetByProviderIdentifier(ctx context.Context, provider, identifier string) (*dbmodel.Credential, error) {
	var c dbmodel.Credential
	if err := r.db.WithContext(ctx).Where("provider = ? AND identifier = ?", provider, identifier).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CredentialRepository) GetUserByCredential(ctx context.Context, provider, identifier string) (*dbmodel.User, *dbmodel.Credential, error) {
	c, err := r.GetByProviderIdentifier(ctx, provider, identifier)
	if err != nil {
		return nil, nil, err
	}
	var u dbmodel.User
	if err := r.db.WithContext(ctx).First(&u, c.UserID).Error; err != nil {
		return nil, nil, err
	}
	return &u, c, nil
}
