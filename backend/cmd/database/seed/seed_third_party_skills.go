package seed

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

func SeedDemoThirdPartySkills(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	now := time.Now().UTC()
	manifest := datatypes.JSON([]byte(`{
		"name":"Prompt Template Renderer",
		"description":"Render text prompt templates with variables for agent pre-processing.",
		"version":"1.0.0",
		"schema":"powerx.skill-manifest.v1",
		"entrypoints":["runbook.default"],
		"input_schema":{
			"type":"object",
			"required":["template"],
			"properties":{
				"template":{"type":"string"},
				"variables":{"type":"object","additionalProperties":true}
			}
		},
		"output_schema":{
			"type":"object",
			"properties":{
				"rendered_text":{"type":"string"},
				"variables_used":{"type":"integer"}
			}
		}
	}`))

	record := &skillmodel.SkillRegistryRecord{
		SkillID:            "skill.thirdparty.prompt-template",
		Version:            "1.0.0",
		Source:             skillmodel.SkillSourceThirdParty,
		Status:             skillmodel.SkillStatusPublished,
		IsLatestPublished:  true,
		BundleURI:          "s3://skills/thirdparty/prompt-template/1.0.0.tgz",
		Checksum:           "sha256:thirdparty-prompt-template-1.0.0",
		ManifestJSON:       manifest,
		SourceURL:          "https://github.com/example/prompt-template-skill",
		SourceRef:          "refs/tags/v1.0.0",
		ImportType:         "upload",
		UpdatedBy:          "seed",
		PublishedAt:        &now,
		LatestSwitchedAt:   &now,
		ApprovalNote:       "seed demo third-party skill",
		IntegrityPolicyRef: "default",
	}
	record.Normalize()

	if err := db.WithContext(seedCtx()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"source":               record.Source,
			"status":               record.Status,
			"is_latest_published":  record.IsLatestPublished,
			"bundle_uri":           record.BundleURI,
			"checksum":             record.Checksum,
			"manifest_json":        record.ManifestJSON,
			"source_url":           record.SourceURL,
			"source_ref":           record.SourceRef,
			"import_type":          record.ImportType,
			"updated_by":           record.UpdatedBy,
			"published_at":         record.PublishedAt,
			"latest_switched_at":   record.LatestSwitchedAt,
			"approval_note":        record.ApprovalNote,
			"integrity_policy_ref": record.IntegrityPolicyRef,
			"updated_at":           now,
		}),
	}).Create(record).Error; err != nil {
		return fmt.Errorf("upsert demo third-party skill failed: %w", err)
	}

	fmt.Println("[seed] demo third-party skill ready: skill.thirdparty.prompt-template@1.0.0")
	return nil
}
