// internal/server/agent/persistence/model/scope.go
package model

type ScopeRef struct {
	Env        string `gorm:"size:32;index"` // default|staging|production
	TenantUUID string `gorm:"size:128;index;default:''"`
}
