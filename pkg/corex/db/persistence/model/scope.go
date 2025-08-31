// internal/server/agent/persistence/model/scope.go
package model

type ScopeRef struct {
	Env      string  `gorm:"size:32;index"` // default|staging|production
	TenantID *uint64 `gorm:"index"`
}
