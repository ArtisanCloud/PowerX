package service

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"gorm.io/gorm"
)

type BaseService struct {
	DB *gorm.DB
}

// ======== helpers ========
func IsRoot(ctx context.Context) bool {
	if c := auth.GetJWTClaims(ctx); c != nil && c.IsRoot {
		return true
	}
	return false
}
