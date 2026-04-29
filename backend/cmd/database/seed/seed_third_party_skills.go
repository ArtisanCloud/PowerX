package seed

import (
	"context"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
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
	helloManifest := datatypes.JSON([]byte(`{
		"name":"Hello Echo",
		"description":"The smallest demo skill package for installation smoke tests.",
		"version":"1.0.0",
		"schema":"powerx.skill-manifest.v1",
		"entrypoints":["runbook.default"],
		"input_schema":{
			"type":"object",
			"properties":{"text":{"type":"string"}}
		},
		"output_schema":{
			"type":"object",
			"properties":{"message":{"type":"string"}}
		}
	}`))
	triageManifest := datatypes.JSON([]byte(`{
		"name":"Incident Triage",
		"description":"Analyze incident context and provide structured triage suggestions.",
		"version":"1.0.0",
		"schema":"powerx.skill-manifest.v1",
		"entrypoints":["runbook.default"],
		"input_schema":{
			"type":"object",
			"properties":{
				"incident_id":{"type":"string"},
				"context":{"type":"string"}
			}
		},
		"output_schema":{
			"type":"object",
			"properties":{
				"summary":{"type":"string"},
				"severity":{"type":"string"},
				"actions":{"type":"array","items":{"type":"string"}}
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

	hello := &skillmodel.SkillRegistryRecord{
		SkillID:            "skill.thirdparty.hello-echo",
		Version:            "1.0.0",
		Source:             skillmodel.SkillSourceThirdParty,
		Status:             skillmodel.SkillStatusPublished,
		IsLatestPublished:  true,
		BundleURI:          "file://$CODEX_HOME/skills/hello-echo",
		Checksum:           "sha256:thirdparty-hello-echo-1.0.0",
		ManifestJSON:       helloManifest,
		SourceURL:          "https://github.com/ArtisanCloud/powerx-skill-examples",
		SourceRef:          "refs/heads/main",
		ImportType:         "install_task",
		UpdatedBy:          "seed",
		PublishedAt:        &now,
		LatestSwitchedAt:   &now,
		ApprovalNote:       "seed demo installable skill package",
		IntegrityPolicyRef: "default",
	}
	hello.Normalize()
	if err := db.WithContext(seedCtx()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"source":               hello.Source,
			"status":               hello.Status,
			"is_latest_published":  hello.IsLatestPublished,
			"bundle_uri":           hello.BundleURI,
			"checksum":             hello.Checksum,
			"manifest_json":        hello.ManifestJSON,
			"source_url":           hello.SourceURL,
			"source_ref":           hello.SourceRef,
			"import_type":          hello.ImportType,
			"updated_by":           hello.UpdatedBy,
			"published_at":         hello.PublishedAt,
			"latest_switched_at":   hello.LatestSwitchedAt,
			"approval_note":        hello.ApprovalNote,
			"integrity_policy_ref": hello.IntegrityPolicyRef,
			"updated_at":           now,
		}),
	}).Create(hello).Error; err != nil {
		return fmt.Errorf("upsert demo installable skill failed: %w", err)
	}

	triage := &skillmodel.SkillRegistryRecord{
		SkillID:            "incident-triage",
		Version:            "1.0.0",
		Source:             skillmodel.SkillSourcePlugin,
		Status:             skillmodel.SkillStatusPublished,
		IsLatestPublished:  true,
		BundleURI:          "s3://skills/plugin/incident-triage/1.0.0.tgz",
		Checksum:           "sha256:plugin-incident-triage-1.0.0",
		ManifestJSON:       triageManifest,
		SourceURL:          "https://github.com/ArtisanCloud/powerx-skill-examples",
		SourceRef:          "refs/heads/main",
		ImportType:         "upload",
		UpdatedBy:          "seed",
		PublishedAt:        &now,
		LatestSwitchedAt:   &now,
		ApprovalNote:       "seed demo incident triage skill",
		IntegrityPolicyRef: "default",
	}
	triage.Normalize()
	if err := db.WithContext(seedCtx()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"source":               triage.Source,
			"status":               triage.Status,
			"is_latest_published":  triage.IsLatestPublished,
			"bundle_uri":           triage.BundleURI,
			"checksum":             triage.Checksum,
			"manifest_json":        triage.ManifestJSON,
			"source_url":           triage.SourceURL,
			"source_ref":           triage.SourceRef,
			"import_type":          triage.ImportType,
			"updated_by":           triage.UpdatedBy,
			"published_at":         triage.PublishedAt,
			"latest_switched_at":   triage.LatestSwitchedAt,
			"approval_note":        triage.ApprovalNote,
			"integrity_policy_ref": triage.IntegrityPolicyRef,
			"updated_at":           now,
		}),
	}).Create(triage).Error; err != nil {
		return fmt.Errorf("upsert demo incident triage skill failed: %w", err)
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] demo third-party skills ready: skill.thirdparty.prompt-template@1.0.0, skill.thirdparty.hello-echo@1.0.0, incident-triage@1.0.0")
	return nil
}
