package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type policyTemplateSeed struct {
	TemplateName string
	Version      string
	RAGProfile   map[string]any
	GraphProfile map[string]any
	Masking      map[string]any
	Alerting     map[string]any
}

func SeedKnowledgePolicyTemplates(db *gorm.DB) error {
	items := []policyTemplateSeed{
		{
			TemplateName: "default",
			Version:      "v1",
			RAGProfile:   map[string]any{"profile": "default"},
			GraphProfile: map[string]any{},
			Masking:      map[string]any{},
			Alerting:     map[string]any{},
		},
	}

	now := time.Now().UTC()
	for _, it := range items {
		name := strings.TrimSpace(it.TemplateName)
		ver := strings.TrimSpace(it.Version)
		if name == "" || ver == "" {
			continue
		}

		ragRaw, _ := json.Marshal(it.RAGProfile)
		graphRaw, _ := json.Marshal(it.GraphProfile)
		maskingRaw, _ := json.Marshal(it.Masking)
		alertRaw, _ := json.Marshal(it.Alerting)

		fp := sha256.Sum256([]byte(name + ":" + ver + ":" + string(ragRaw) + ":" + string(graphRaw) + ":" + string(maskingRaw) + ":" + string(alertRaw)))
		hash := hex.EncodeToString(fp[:])

		var existing models.PolicyTemplateVersion
		err := db.WithContext(seedCtx()).
			Where("template_name = ? AND version = ?", name, ver).
			Take(&existing).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		if err == nil && existing.ID > 0 {
			// 若已存在，仅在 hash 为空时补齐（避免覆盖你手工调优过的 profile）
			if strings.TrimSpace(existing.ImmutableHash) == "" {
				if err := db.WithContext(seedCtx()).
					Model(&models.PolicyTemplateVersion{}).
					Where("id = ?", existing.ID).
					Updates(map[string]any{
						"immutable_hash": hash,
						"approved_by":    "seed",
						"approved_at":    &now,
					}).Error; err != nil {
					return err
				}
			}
			continue
		}

		row := &models.PolicyTemplateVersion{
			TemplateName:    name,
			Version:         ver,
			RAGProfile:      datatypes.JSON(ragRaw),
			GraphProfile:    datatypes.JSON(graphRaw),
			MaskingProfile:  datatypes.JSON(maskingRaw),
			AlertingProfile: datatypes.JSON(alertRaw),
			ApprovedBy:      "seed",
			ApprovedAt:      &now,
			ImmutableHash:   hash,
		}
		if err := db.WithContext(seedCtx()).Create(row).Error; err != nil {
			return err
		}
		logger.InfoF(context.Background(), "[seed] policy templates ready: %s-%s (id=%d)", name, ver, row.ID)
	}
	return nil
}
