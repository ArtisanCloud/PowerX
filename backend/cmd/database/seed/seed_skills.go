package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type builtinSkillSeed struct {
	CatalogSkillID      string
	SkillID             string
	Version             string
	RiskLevel           string
	Category            string
	Summary             string
	Maintainer          string
	OfficialReleaseNote string
	BundleURI           string
	Checksum            string
}

func SeedOfficialBuiltinSkills(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	now := time.Now().UTC()
	seeds := []builtinSkillSeed{
		{CatalogSkillID: "catalog.healthcheck", SkillID: "skill.builtin.healthcheck", Version: "1.0.0", RiskLevel: "L1", Category: "platform", Summary: "环境连通性与依赖健康检查", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin health diagnostics", BundleURI: "builtin://skills/healthcheck/1.0.0", Checksum: "sha256:builtin-healthcheck-1.0.0"},
		{CatalogSkillID: "catalog.session-logs", SkillID: "skill.builtin.session-logs", Version: "1.0.0", RiskLevel: "L1", Category: "platform", Summary: "会话日志检索与摘要", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin session log triage", BundleURI: "builtin://skills/session-logs/1.0.0", Checksum: "sha256:builtin-session-logs-1.0.0"},
		{CatalogSkillID: "catalog.model-usage", SkillID: "skill.builtin.model-usage", Version: "1.0.0", RiskLevel: "L1", Category: "platform", Summary: "模型调用量与成本概览", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin model usage insights", BundleURI: "builtin://skills/model-usage/1.0.0", Checksum: "sha256:builtin-model-usage-1.0.0"},
		{CatalogSkillID: "catalog.coding-agent", SkillID: "skill.builtin.coding-agent", Version: "1.0.0", RiskLevel: "L2", Category: "dev", Summary: "代码任务执行工作流模板", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin coding workflow", BundleURI: "builtin://skills/coding-agent/1.0.0", Checksum: "sha256:builtin-coding-agent-1.0.0"},
		{CatalogSkillID: "catalog.github", SkillID: "skill.builtin.github", Version: "1.0.0", RiskLevel: "L2", Category: "dev", Summary: "GitHub 仓库与 PR 常用操作", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin GitHub assistant", BundleURI: "builtin://skills/github/1.0.0", Checksum: "sha256:builtin-github-1.0.0"},
		{CatalogSkillID: "catalog.gh-issues", SkillID: "skill.builtin.gh-issues", Version: "1.0.0", RiskLevel: "L2", Category: "dev", Summary: "Issue 分诊与状态推进", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin issue triage", BundleURI: "builtin://skills/gh-issues/1.0.0", Checksum: "sha256:builtin-gh-issues-1.0.0"},
		{CatalogSkillID: "catalog.tmux", SkillID: "skill.builtin.tmux", Version: "1.0.0", RiskLevel: "L2", Category: "dev", Summary: "长任务会话编排", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin tmux orchestration", BundleURI: "builtin://skills/tmux/1.0.0", Checksum: "sha256:builtin-tmux-1.0.0"},
		{CatalogSkillID: "catalog.summarize", SkillID: "skill.builtin.summarize", Version: "1.0.0", RiskLevel: "L1", Category: "doc", Summary: "文本与网页总结", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin summarization", BundleURI: "builtin://skills/summarize/1.0.0", Checksum: "sha256:builtin-summarize-1.0.0"},
		{CatalogSkillID: "catalog.nano-pdf", SkillID: "skill.builtin.nano-pdf", Version: "1.0.0", RiskLevel: "L1", Category: "doc", Summary: "PDF 提取与问答", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin pdf extraction", BundleURI: "builtin://skills/nano-pdf/1.0.0", Checksum: "sha256:builtin-nano-pdf-1.0.0"},
		{CatalogSkillID: "catalog.notion", SkillID: "skill.builtin.notion", Version: "1.0.0", RiskLevel: "L2", Category: "knowledge", Summary: "Notion 页面检索与写入", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin notion connector", BundleURI: "builtin://skills/notion/1.0.0", Checksum: "sha256:builtin-notion-1.0.0"},
		{CatalogSkillID: "catalog.obsidian", SkillID: "skill.builtin.obsidian", Version: "1.0.0", RiskLevel: "L2", Category: "knowledge", Summary: "Obsidian 笔记管理", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin obsidian connector", BundleURI: "builtin://skills/obsidian/1.0.0", Checksum: "sha256:builtin-obsidian-1.0.0"},
		{CatalogSkillID: "catalog.slack", SkillID: "skill.builtin.slack", Version: "1.0.0", RiskLevel: "L2", Category: "comm", Summary: "Slack 消息与频道操作", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin slack messaging", BundleURI: "builtin://skills/slack/1.0.0", Checksum: "sha256:builtin-slack-1.0.0"},
		{CatalogSkillID: "catalog.discord", SkillID: "skill.builtin.discord", Version: "1.0.0", RiskLevel: "L2", Category: "comm", Summary: "Discord 消息与频道操作", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin discord messaging", BundleURI: "builtin://skills/discord/1.0.0", Checksum: "sha256:builtin-discord-1.0.0"},
		{CatalogSkillID: "catalog.trello", SkillID: "skill.builtin.trello", Version: "1.0.0", RiskLevel: "L2", Category: "pm", Summary: "Trello 看板卡片管理", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin trello workflow", BundleURI: "builtin://skills/trello/1.0.0", Checksum: "sha256:builtin-trello-1.0.0"},
		{CatalogSkillID: "catalog.openai-whisper", SkillID: "skill.builtin.openai-whisper", Version: "1.0.0", RiskLevel: "L1", Category: "media", Summary: "语音转文本", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin speech-to-text", BundleURI: "builtin://skills/openai-whisper/1.0.0", Checksum: "sha256:builtin-openai-whisper-1.0.0"},
		{CatalogSkillID: "catalog.openai-image-gen", SkillID: "skill.builtin.openai-image-gen", Version: "1.0.0", RiskLevel: "L2", Category: "media", Summary: "图像生成与编辑", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin image generation", BundleURI: "builtin://skills/openai-image-gen/1.0.0", Checksum: "sha256:builtin-openai-image-gen-1.0.0"},
		{CatalogSkillID: "catalog.video-frames", SkillID: "skill.builtin.video-frames", Version: "1.0.0", RiskLevel: "L2", Category: "media", Summary: "视频抽帧处理", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin video frame utility", BundleURI: "builtin://skills/video-frames/1.0.0", Checksum: "sha256:builtin-video-frames-1.0.0"},
		{CatalogSkillID: "catalog.marketing-source-parse", SkillID: "marketing.audio_or_document_parse", Version: "1.0.0", RiskLevel: "L2", Category: "marketing", Summary: "workflow.skill.marketingSourceParse.summary", Maintainer: "powerx-core", OfficialReleaseNote: "workflow.skill.marketingSourceParse.releaseNote", BundleURI: "builtin://skills/marketing/audio-or-document-parse/1.0.0", Checksum: "sha256:builtin-marketing-audio-or-document-parse-1.0.0"},
		{CatalogSkillID: "catalog.marketing-methodology-extract", SkillID: "marketing.extract_methodology", Version: "1.0.0", RiskLevel: "L2", Category: "marketing", Summary: "workflow.skill.marketingMethodologyExtract.summary", Maintainer: "powerx-core", OfficialReleaseNote: "workflow.skill.marketingMethodologyExtract.releaseNote", BundleURI: "builtin://skills/marketing/extract-methodology/1.0.0", Checksum: "sha256:builtin-marketing-extract-methodology-1.0.0"},
		{CatalogSkillID: "catalog.marketing-metric-extract", SkillID: "marketing.metric_extract", Version: "1.0.0", RiskLevel: "L2", Category: "marketing", Summary: "workflow.skill.marketingMetricExtract.summary", Maintainer: "powerx-core", OfficialReleaseNote: "workflow.skill.marketingMetricExtract.releaseNote", BundleURI: "builtin://skills/marketing/metric-extract/1.0.0", Checksum: "sha256:builtin-marketing-metric-extract-1.0.0"},
		{CatalogSkillID: "catalog.marketing-review-summarize", SkillID: "marketing.review_summarize", Version: "1.0.0", RiskLevel: "L2", Category: "marketing", Summary: "workflow.skill.marketingReviewSummarize.summary", Maintainer: "powerx-core", OfficialReleaseNote: "workflow.skill.marketingReviewSummarize.releaseNote", BundleURI: "builtin://skills/marketing/review-summarize/1.0.0", Checksum: "sha256:builtin-marketing-review-summarize-1.0.0"},
		{CatalogSkillID: "catalog.camsnap", SkillID: "skill.builtin.camsnap", Version: "1.0.0", RiskLevel: "L3", Category: "device", Summary: "摄像头抓拍", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin camera capture", BundleURI: "builtin://skills/camsnap/1.0.0", Checksum: "sha256:builtin-camsnap-1.0.0"},
		{CatalogSkillID: "catalog.voice-call", SkillID: "skill.builtin.voice-call", Version: "1.0.0", RiskLevel: "L3", Category: "channel", Summary: "语音通话控制", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin voice calling", BundleURI: "builtin://skills/voice-call/1.0.0", Checksum: "sha256:builtin-voice-call-1.0.0"},
		{CatalogSkillID: "catalog.1password", SkillID: "skill.builtin.1password", Version: "1.0.0", RiskLevel: "L3", Category: "security", Summary: "凭据安全读取与填充", Maintainer: "powerx-core", OfficialReleaseNote: "Builtin secret workflow", BundleURI: "builtin://skills/1password/1.0.0", Checksum: "sha256:builtin-1password-1.0.0"},
	}

	for i := range seeds {
		s := seeds[i]
		manifest, err := builtinSkillManifest(s)
		if err != nil {
			return fmt.Errorf("build builtin manifest %s: %w", s.SkillID, err)
		}

		catalog := &skillmodel.OfficialSkillCatalogEntry{
			CatalogSkillID:      s.CatalogSkillID,
			SkillID:             s.SkillID,
			RecommendedVersion:  s.Version,
			RiskLevel:           s.RiskLevel,
			Category:            s.Category,
			Summary:             s.Summary,
			Active:              true,
			Maintainer:          s.Maintainer,
			OfficialReleaseNote: s.OfficialReleaseNote,
		}
		catalog.Normalize()
		if err := db.WithContext(seedCtx()).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "catalog_skill_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"skill_id":              catalog.SkillID,
				"recommended_version":   catalog.RecommendedVersion,
				"risk_level":            catalog.RiskLevel,
				"category":              catalog.Category,
				"summary":               catalog.Summary,
				"active":                catalog.Active,
				"maintainer":            catalog.Maintainer,
				"official_release_note": catalog.OfficialReleaseNote,
				"updated_at":            now,
			}),
		}).Create(catalog).Error; err != nil {
			return fmt.Errorf("upsert official catalog %s failed: %w", s.CatalogSkillID, err)
		}

		registry := &skillmodel.SkillRegistryRecord{
			SkillID:            s.SkillID,
			Version:            s.Version,
			Source:             skillmodel.SkillSourceBuiltin,
			Status:             skillmodel.SkillStatusPublished,
			IsLatestPublished:  true,
			BundleURI:          s.BundleURI,
			Checksum:           s.Checksum,
			ManifestJSON:       manifest,
			ImportType:         "official_catalog",
			UpdatedBy:          "seed",
			PublishedAt:        &now,
			LatestSwitchedAt:   &now,
			ApprovalNote:       "seed builtin official catalog",
			IntegrityPolicyRef: "builtin-default",
		}
		registry.Normalize()
		if err := db.WithContext(seedCtx()).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "skill_id"}, {Name: "version"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"source":               registry.Source,
				"status":               registry.Status,
				"is_latest_published":  registry.IsLatestPublished,
				"bundle_uri":           registry.BundleURI,
				"checksum":             registry.Checksum,
				"manifest_json":        registry.ManifestJSON,
				"import_type":          registry.ImportType,
				"updated_by":           registry.UpdatedBy,
				"published_at":         registry.PublishedAt,
				"latest_switched_at":   registry.LatestSwitchedAt,
				"approval_note":        registry.ApprovalNote,
				"integrity_policy_ref": registry.IntegrityPolicyRef,
				"updated_at":           now,
			}),
		}).Create(registry).Error; err != nil {
			return fmt.Errorf("upsert builtin registry %s failed: %w", s.SkillID, err)
		}
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] skills builtin catalog ready: %d", len(seeds))
	return nil
}

// builtinSkillManifest only persists catalog metadata. A skill becomes
// executable exclusively through its published declarative manifest; Core
// never branches by a business Skill ID.
func builtinSkillManifest(seed builtinSkillSeed) (datatypes.JSON, error) {
	manifest := map[string]any{
		"name":           "builtin skill",
		"description":    "seeded by platform",
		"entrypoints":    []string{"default"},
		"schema_version": "1.0",
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(raw), nil
}
