// pkg/corex/db/persistence/model/setting/domain_tls_gorm.go
package setting

import (
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
	"time"
)

// 仅保存证书“引用”，不落私钥明文。证书实体在文件/密管，Ref 指向它。
type TLSCertRef struct {
	coremodel.PowerModel

	// 作用域（可选）：系统级/租户级证书
	TenantUUID string `gorm:"column:tenant_uuid;type:varchar(128);not null;default:'';index:uk_tls_cert_scope_kind_ref,priority:1" json:"tenant_uuid"`

	// 引用信息
	Kind string `gorm:"column:kind;type:varchar(32);not null;index:uk_tls_cert_scope_kind_ref,priority:2;comment:ref类型:file|vault|acme" json:"kind"`
	Ref  string `gorm:"column:ref;type:varchar(512);not null;index:uk_tls_cert_scope_kind_ref,priority:3;comment:路径/密管条目/证书ID" json:"ref"`

	// 元信息（可选）
	Subject       *string    `gorm:"column:subject;type:varchar(256)" json:"subject,omitempty"`
	Fingerprint   *string    `gorm:"column:fingerprint;type:varchar(128)" json:"fingerprint,omitempty"`
	NotBefore     *time.Time `gorm:"column:not_before" json:"not_before,omitempty"`
	NotAfter      *time.Time `gorm:"column:not_after"  json:"not_after,omitempty"`
	ManagedByACME bool       `gorm:"column:managed_by_acme;default:false" json:"managed_by_acme"`
}

func (m *TLSCertRef) TableName() string { return coremodel.PowerXSchema + "." + TableTLSCertRef }
func (m *TLSCertRef) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableTLSCertRef
}

// 域名绑定（按租户/子域可多条）；cert 引用到 TLSCertRef.ID 或外部引用
type DomainBinding struct {
	coremodel.PowerModel

	TenantUUID string `gorm:"column:tenant_uuid;type:char(36);not null;index:idx_domain_tenant_uuid_host,priority:1;uniqueIndex:uk_domain_tenant_uuid_host,priority:1" json:"tenant_uuid"`
	Host       string `gorm:"column:host;type:varchar(255);not null;index:idx_domain_tenant_uuid_host,priority:2;uniqueIndex:uk_domain_tenant_uuid_host,priority:2" json:"host"`

	HTTPSMode string  `gorm:"column:https_mode;type:varchar(16);not null;default:'disable'" json:"https_mode"` // auto|manual|disable
	CertRefID *uint64 `gorm:"column:cert_ref_id;index" json:"cert_ref_id,omitempty"`                           // 关联 TLSCertRef

	CDNDomain *string `gorm:"column:cdn_domain;type:varchar(255)" json:"cdn_domain,omitempty"`
	// 可选：绑定状态/生效时间窗
	Active    bool       `gorm:"column:active;default:true;index" json:"active"`
	ValidFrom *time.Time `gorm:"column:valid_from"                json:"valid_from,omitempty"`
	ValidTo   *time.Time `gorm:"column:valid_to"                  json:"valid_to,omitempty"`
}

func (m *DomainBinding) TableName() string { return coremodel.PowerXSchema + "." + TableDomainBinding }
func (m *DomainBinding) GetTableName(full bool) string {
	if full {
		return m.TableName()
	}
	return TableDomainBinding
}

// 外键关系（可选）：删除证书引用不级联删绑定，避免误伤
func (m *DomainBinding) BeforeDelete(tx *gorm.DB) error {
	return nil
}
