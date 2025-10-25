package seed

import (
	"errors"

	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	"gorm.io/gorm"
)

var capabilityErrorTaxonomySeeds = []capmodel.CapabilityErrorTaxonomy{
	{
		TenantID:  0,
		Namespace: "validation",
		Category:  "bad_request",
		Code:      "INVALID_ARGUMENT",
		Severity:  "ERROR",
		Stage:     "validate",
		Status:    1,
	},
	{
		TenantID:  0,
		Namespace: "transport",
		Category:  "upstream",
		Code:      "UPSTREAM_TIMEOUT",
		Severity:  "ERROR",
		Stage:     "invoke",
		Status:    1,
	},
	{
		TenantID:  0,
		Namespace: "permission",
		Category:  "authorization",
		Code:      "PERMISSION_DENIED",
		Severity:  "ERROR",
		Stage:     "validate",
		Status:    1,
	},
	{
		TenantID:  0,
		Namespace: "system",
		Category:  "internal",
		Code:      "INTERNAL_ERROR",
		Severity:  "ERROR",
		Stage:     "invoke",
		Status:    1,
	},
}

// SeedCapabilityErrorTaxonomies 预置常用的能力错误枚举，避免契约首次写入时重复创建。
func SeedCapabilityErrorTaxonomies(db *gorm.DB) error {
	for i := range capabilityErrorTaxonomySeeds {
		seed := capabilityErrorTaxonomySeeds[i]
		var existing capmodel.CapabilityErrorTaxonomy
		if err := db.Where("tenant_id = ? AND namespace = ? AND category = ? AND code = ?",
			seed.TenantID, seed.Namespace, seed.Category, seed.Code).
			First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := db.Create(&seed).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			updates := map[string]interface{}{
				"severity": seed.Severity,
				"stage":    seed.Stage,
				"status":   seed.Status,
			}
			if err := db.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
