package iam

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	eventBus "github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
	"time"
)

type User struct {
	ID, TenantID, Account, DisplayName string
	Active                             bool
	CreatedAt                          time.Time
}
type CreateUserDTO struct {
	Account, DisplayName string
}

type Service interface {
	CreateUser(ctx context.Context, in CreateUserDTO) (User, error)
	ListUsers(ctx context.Context, page, size int) ([]User, int64, error)
	// Department/Membership 省略…
}

type service struct {
	db  *gorm.DB
	bus eventBus.EventBus
	aud audit.Auditor
}

func NewService(gdb *gorm.DB, bus eventBus.EventBus, aud audit.Auditor) Service {
	return &service{db: gdb, bus: bus, aud: aud}
}

// 伪实现：落库后发事件 iam.user.created
