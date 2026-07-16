package metadata

import (
	"context"

	"gorm.io/gorm"
)

type AuditPublisher interface {
	PublishMetadataAudit(ctx context.Context, event AuditEvent) error
}

type PermissionChecker interface {
	Can(ctx context.Context, permissionCode string) error
}

type AuditEvent struct {
	TenantUUID string
	Operation  string
	ObjectType string
	ObjectUUID string
	ErrorCode  string
}

type Deps struct {
	DB                *gorm.DB
	AuditPublisher    AuditPublisher
	PermissionChecker PermissionChecker
	ValidatorRegistry ResourceValidatorRegistry
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) (*Service, error) {
	if deps.DB == nil {
		return nil, ErrMissingDB
	}
	if deps.ValidatorRegistry == nil {
		deps.ValidatorRegistry = NewStaticResourceValidatorRegistry(nil)
	}
	return &Service{deps: deps}, nil
}
