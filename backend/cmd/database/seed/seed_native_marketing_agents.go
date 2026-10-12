package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	appcfg "github.com/ArtisanCloud/PowerX/config"
	mediad "github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repoagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	MarketingDirectorAdvisorAgentKey   = "marketing.director_advisor"
	MarketingContentStrategistAgentKey = "marketing.content_strategist"
	ExpertKnowledgeCuratorAgentKey     = "knowledge.expert_curator"
	MarketingCampaignReviewerAgentKey  = "marketing.campaign_reviewer"

	MarketingKnowledgeCaptureWorkflowKey   = "marketing_knowledge_capture"
	CampaignReviewToMethodologyWorkflowKey = "campaign_review_to_methodology"
	MarketingSourceParseSkillID            = "marketing.audio_or_document_parse"
	MarketingMethodologyExtractSkillID     = "marketing.extract_methodology"
	MarketingMetricExtractSkillID          = "marketing.metric_extract"
	MarketingReviewSummarizeSkillID        = "marketing.review_summarize"
	MarketingCampaignReviewTeamKey         = "marketing.campaign_review"
	MarketingCampaignReviewTeamName        = "营销活动复盘协作团队"
	MarketingCampaignReviewTeamNameEN      = "Marketing Campaign Review Team"
	MarketingCampaignReviewTeamNameJA      = "マーケティングキャンペーン振り返り協働チーム"
	MarketingCampaignReviewTeamNameKO      = "마케팅 캠페인 회고 협업 팀"
)

type nativeMarketingAgentSeed struct {
	Key           string
	Name          string
	NameEN        string
	Description   string
	DescriptionEN string
	Role          string
	Category      string
	Scene         string
	PromptSeed    string
	SkillIDs      []string
	WorkflowKeys  []string
}

type nativeMarketingSkillSeed struct {
	SkillID       string
	Name          string
	NameEN        string
	Description   string
	DescriptionEN string
	PromptI18n    map[string]string
}

// nativeMarketingSkillSeeds is data only. The generic declaration runtime
// selects executor.type and never branches on one of these identifiers.
func nativeMarketingSkillSeeds() []nativeMarketingSkillSeed {
	return []nativeMarketingSkillSeed{
		{
			SkillID: MarketingSourceParseSkillID, Name: "营销素材事实提取", NameEN: "Marketing Source Fact Extraction", Description: "从输入材料中提取可追溯事实、渠道、素材和数据缺口。", DescriptionEN: "Extracts traceable facts, channels, assets, and data gaps from campaign materials.",
			PromptI18n: nativeMarketingPromptI18n["source_analysis"],
		},
		{
			SkillID: MarketingMetricExtractSkillID, Name: "营销指标分析", NameEN: "Marketing Metric Analysis", Description: "计算活动漏斗指标并区分数据结论与待验证归因。", DescriptionEN: "Calculates campaign funnel metrics and separates evidence from unverified attribution.",
			PromptI18n: nativeMarketingPromptI18n["campaign_analysis"],
		},
		{
			SkillID: MarketingMethodologyExtractSkillID, Name: "营销方法论沉淀", NameEN: "Marketing Methodology Curation", Description: "根据上游事实和指标，形成可验证的营销方法论草稿。", DescriptionEN: "Creates a verifiable marketing-methodology draft from upstream facts and metrics.",
			PromptI18n: nativeMarketingPromptI18n["knowledge_curation"],
		},
		{
			SkillID: MarketingReviewSummarizeSkillID, Name: "营销活动复盘汇总", NameEN: "Marketing Campaign Review Synthesis", Description: "汇总团队子任务产物，形成包含结论、行动和验收标准的复盘报告。", DescriptionEN: "Synthesizes team outputs into a review report with conclusions, actions, and acceptance criteria.",
			PromptI18n: nativeMarketingPromptI18n["summary"],
		},
	}
}

func seedNativeMarketingSkillDefinitions(ctx context.Context, db *gorm.DB, cfg *appcfg.Config, tenantUUID, actorMemberUUID string) error {
	driverName := strings.ToLower(strings.TrimSpace(cfg.Storage.DefaultDriver))
	if driverName != "local" && driverName != "s3" {
		return fmt.Errorf("seed_native_marketing_skills_storage_driver_unsupported")
	}
	if driverName == "local" && strings.TrimSpace(cfg.Storage.Local.BasePath) == "" {
		return fmt.Errorf("seed_native_marketing_skills_local_storage_path_required")
	}
	if driverName == "s3" && (strings.TrimSpace(cfg.Storage.S3.Endpoint) == "" || strings.TrimSpace(cfg.Storage.S3.Bucket) == "") {
		return fmt.Errorf("seed_native_marketing_skills_s3_storage_config_required")
	}
	manager, _ := mediasvc.BuildMediaStack(ctx, db, nil, mediasvc.StorageOptions{
		DefaultDriver: driverName, TTLSeconds: cfg.Storage.TTLSeconds,
		Local: mediasvc.StorageLocalOptions{BasePath: cfg.Storage.Local.BasePath, PublicBaseURL: cfg.Storage.Local.PublicBaseURL, UploadTokenSecret: cfg.Storage.Local.UploadTokenSecret, PublicTokenSecret: cfg.Storage.Local.PublicTokenSecret, MaxUploadSizeBytes: cfg.Storage.Local.MaxUploadSizeBytes},
		S3:    mediasvc.StorageS3Options{Endpoint: cfg.Storage.S3.Endpoint, Region: cfg.Storage.S3.Region, AccessKey: cfg.Storage.S3.AccessKey, SecretKey: cfg.Storage.S3.SecretKey, SessionToken: cfg.Storage.S3.SessionToken, Bucket: cfg.Storage.S3.Bucket, UseSSL: cfg.Storage.S3.UseSSL, ForcePathStyle: cfg.Storage.S3.ForcePathStyle, ExternalDomain: cfg.Storage.S3.ExternalDomain, PresignEndpoint: cfg.Storage.S3.PresignEndpoint},
	})
	if manager == nil || !seedMediaHasDriver(manager, driverName) {
		return fmt.Errorf("seed_native_marketing_skills_storage_driver_unavailable")
	}
	definitionRepo := skillrepo.NewSkillDefinitionRepository(db)
	definitions := skillsvc.NewDefinitionService(definitionRepo)
	publisher := skillsvc.NewPackagePublisher(seedSkillPackageStore{manager: manager, driverName: driverName, bucket: cfg.Storage.S3.Bucket})
	for _, item := range nativeMarketingSkillSeeds() {
		if err := seedNativeMarketingSkillDefinition(ctx, definitionRepo, definitions, publisher, tenantUUID, actorMemberUUID, item); err != nil {
			return err
		}
	}
	return nil
}

func seedNativeMarketingSkillDefinition(ctx context.Context, repo *skillrepo.SkillDefinitionRepository, definitions *skillsvc.DefinitionService, publisher *skillsvc.PackagePublisher, tenantUUID, actorMemberUUID string, item nativeMarketingSkillSeed) error {
	if strings.TrimSpace(item.SkillID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.NameEN) == "" || strings.TrimSpace(item.Description) == "" || strings.TrimSpace(item.DescriptionEN) == "" || strings.TrimSpace(item.PromptI18n["zh-CN"]) == "" || strings.TrimSpace(item.PromptI18n["en-US"]) == "" {
		return fmt.Errorf("seed_native_marketing_skill_definition_invalid")
	}
	definition, err := nativeMarketingSkillDefinition(item)
	if err != nil {
		return fmt.Errorf("build native marketing skill definition %s: %w", item.SkillID, err)
	}
	existing, err := repo.GetDraftBySkillID(ctx, tenantUUID, item.SkillID)
	if err == nil {
		if existing.Status != skillmodel.SkillDefinitionDraftStatusPublished {
			return fmt.Errorf("seed_native_marketing_skill_existing_draft_requires_manual_review")
		}
		current, getErr := repo.GetCurrentRevision(ctx, tenantUUID, existing.UUID.String())
		if getErr != nil {
			return fmt.Errorf("read native marketing skill revision %s: %w", item.SkillID, getErr)
		}
		if nativeMarketingDefinitionMatches(current.DefinitionJSON, definition) {
			return nil
		}
		_, revision, appendErr := definitions.AppendRevision(ctx, tenantUUID, existing.UUID.String(), actorMemberUUID, definition, "seed_native_marketing_skill_contract_v2", "")
		if appendErr != nil {
			return fmt.Errorf("append native marketing skill definition %s: %w", item.SkillID, appendErr)
		}
		published, publishErr := publisher.PublishCanonical(ctx, skillsvc.CanonicalSkillPackageInput{
			TenantUUID: tenantUUID, SkillID: item.SkillID, RevisionUUID: revision.UUID.String(),
			DisplayName: item.Name, Description: item.Description, Definition: definition,
		})
		if publishErr != nil {
			return fmt.Errorf("publish native marketing skill package %s: %w", item.SkillID, publishErr)
		}
		if _, _, publishErr = definitions.PublishCurrentRevision(ctx, skillsvc.PublishDefinitionInput{
			TenantUUID: tenantUUID, DraftUUID: existing.UUID.String(), ArtifactURI: published.ArtifactURI,
			Checksum: published.Checksum, UpdatedByMemberUUID: actorMemberUUID,
		}); publishErr != nil {
			return fmt.Errorf("publish native marketing skill definition %s: %w", item.SkillID, publishErr)
		}
		return nil
	}
	if !errors.Is(err, skillrepo.ErrSkillDefinitionDraftNotFound) {
		return fmt.Errorf("find native marketing skill definition %s: %w", item.SkillID, err)
	}
	sourceArtifact, err := publisher.PublishAuthoringSource(ctx, skillsvc.SourceSkillPackageInput{
		TenantUUID: tenantUUID, SkillID: item.SkillID, SourceUUID: uuid.NewString(),
		DisplayName: item.Name, Description: item.Description, Definition: definition,
	})
	if err != nil {
		return fmt.Errorf("publish native marketing skill source %s: %w", item.SkillID, err)
	}
	source, err := definitions.CreatePackageSource(ctx, skillsvc.CreatePackageSourceInput{
		TenantUUID: tenantUUID, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		ArtifactURI: sourceArtifact.ArtifactURI, Checksum: sourceArtifact.Checksum, ContentType: sourceArtifact.ContentType,
		StandardManifest: map[string]any{"name": item.Name, "description": item.Description},
		PowerXExtension:  map[string]any{"schema": skillsvc.SkillDefinitionSchemaV2}, CreatedByMemberUUID: actorMemberUUID,
	})
	if err != nil {
		return fmt.Errorf("record native marketing skill source %s: %w", item.SkillID, err)
	}
	draft, revision, err := definitions.CreateDraft(ctx, skillsvc.CreateDefinitionDraftInput{
		TenantUUID: tenantUUID, SkillID: item.SkillID,
		DisplayNameI18n: map[string]string{"zh-CN": item.Name, "en-US": item.NameEN},
		DescriptionI18n: map[string]string{"zh-CN": item.Description, "en-US": item.DescriptionEN},
		SourceKind:      skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: definition,
		ChangeSummary: "seed_native_marketing_skill", AuthorMemberUUID: actorMemberUUID,
		InitialDraftStatus: skillmodel.SkillDefinitionDraftStatusReadyForReview,
	})
	if err != nil {
		return fmt.Errorf("create native marketing skill definition %s: %w", item.SkillID, err)
	}
	published, err := publisher.PublishCanonical(ctx, skillsvc.CanonicalSkillPackageInput{
		TenantUUID: tenantUUID, SkillID: item.SkillID, RevisionUUID: revision.UUID.String(),
		DisplayName: item.Name, Description: item.Description, Definition: definition,
	})
	if err != nil {
		return fmt.Errorf("publish native marketing skill package %s: %w", item.SkillID, err)
	}
	_, _, err = definitions.PublishCurrentRevision(ctx, skillsvc.PublishDefinitionInput{
		TenantUUID: tenantUUID, DraftUUID: draft.UUID.String(), ArtifactURI: published.ArtifactURI,
		Checksum: published.Checksum, UpdatedByMemberUUID: actorMemberUUID,
	})
	if err != nil {
		return fmt.Errorf("publish native marketing skill definition %s: %w", item.SkillID, err)
	}
	return nil
}

func nativeMarketingSkillDefinition(item nativeMarketingSkillSeed) (map[string]any, error) {
	outputMode := "markdown"
	if item.SkillID == MarketingReviewSummarizeSkillID {
		outputMode = "response_envelope"
	}
	promptI18n := make(map[string]any, len(item.PromptI18n))
	for locale, prompt := range item.PromptI18n {
		guard := nativeMarketingNumericEvidenceRulesI18n[locale]
		if guard == "" {
			return nil, fmt.Errorf("native_marketing_numeric_evidence_rule_locale_required: %s", locale)
		}
		formulaGuide := nativeMarketingFormulaGuideI18n[locale]
		if formulaGuide == "" {
			return nil, fmt.Errorf("native_marketing_formula_guide_locale_required: %s", locale)
		}
		promptI18n[locale] = strings.TrimSpace(fmt.Sprintf("%s\n%s\n%s", prompt, guard, formulaGuide))
	}

	definition := map[string]any{
		"schema": skillsvc.SkillDefinitionSchemaV2,
		"executor": map[string]any{
			"type":                 "llm_prompt",
			"prompt_template_i18n": promptI18n,
			"output_mode":          outputMode,
			"model_policy":         map[string]any{"mode": "inherit_current_agent"},
		},
		"entrypoints": []any{"runbook.default"},
	}
	if outputMode == "response_envelope" {
		executor := definition["executor"].(map[string]any)
		policy, err := nativeMarketingCalculationPolicy()
		if err != nil {
			return nil, err
		}
		executor["calculation_policy"] = policy
		review, err := nativeMarketingReviewPolicy()
		if err != nil {
			return nil, err
		}
		executor["review_policy"] = review
		executor["response_contract"] = evidence.ReportSchema
		executor["evidence_sources"] = []string{"/message"}
		executor["model_policy"].(map[string]any)["parameters"] = map[string]any{"thinking": false, "max_tokens": 4096}
		definition["tool_dependencies"] = skillsvc.ToolRequirements{Schema: skillsvc.ToolDependencySchema, Tools: []skillsvc.ToolDependency{skillsvc.CalculatorDependency()}}
	}
	return definition, nil
}

// nativeMarketingNumericEvidenceRulesI18n applies to every demo subtask. It
// prevents a Markdown handoff from upgrading a stated rate into a computed
// fact before the platform-owned response envelope has verified it.
var nativeMarketingNumericEvidenceRulesI18n = map[string]string{
	"zh-CN": "数值证据规则：只有同一来源明确给出原始数量或金额分子和分母时，才可请求比率计算。ROI、CTR、转化率、百分比和归因主张本身不是可继续相除的原始操作数，不得互相相除或反推人数。已有金额、原文报告值和业务目标必须保留，不能说成缺失；缺操作数限制独立复算，口径尚未核实则列为核实事项，不自动抹去原文报告值。子任务不心算，最终计算由工具执行。",
	"en-US": "Numeric evidence rule: request a rate calculation only with source-supplied raw count or currency operands. ROI, CTR, percentages and attribution claims are not raw operands for further division or inferred counts. Preserve supplied amounts, reported rates and business goals; never call them missing. Missing operands limit independent recalculation; unverified definitions are checks, not grounds to erase reported values. Subtasks do not calculate; the final tool owns computation.",
}

// nativeMarketingFormulaGuideI18n is a versioned, business-level formula
// dictionary for the marketing-review demo. It is prompt data packaged with
// the Skills, not a runtime branch for a particular Team or tenant.
var nativeMarketingFormulaGuideI18n = map[string]string{
	"zh-CN": "营销复盘公式词典：1) 活动产投比（GMV/投入）= 活动标记GMV÷活动投入；只有业务明确把该口径命名为“财务 ROI”时，才可在标签中保留该原文名称，绝不可改为“投入÷GMV”。2) 严格 ROI（收益-成本）÷成本，只有原始输入明确给出收益定义、成本范围并要求该口径时才计算；不得把产投比冒充严格 ROI。3) 增量产投比=可归因增量GMV÷活动投入；若原文声称的“增量 ROI”与该式结果不同，必须把它写为口径冲突，要求提供归因模型、收益定义、分子和分母，不能任选其一作为已确认指标。4) 点击率=点击数÷曝光数；落地页转化率=表单提交数÷落地页访问数；点击到下单转化率=下单数÷点击数；线索转化率=有效线索数÷表单提交数；成交转化率=成交数÷有效线索数。5) 复购率=规定周期内复购客户数÷对应客户池总数；留存率=期末仍活跃客户数÷期初客户数。6) 增量转化提升=实验组转化率-对照组转化率，必须有同口径实验组、对照组和样本量。对应原始计数或金额齐全即可执行声明公式并注明口径条件；缺原始操作数才是复算缺口，未核实的归因、收益或成本定义属于核实事项。不同渠道的比率差异本身不是矛盾。",
	"en-US": "Marketing review formula guide: 1) Campaign return multiple (GMV/cost) = attributed campaign GMV divided by campaign spend. Preserve a business source's label of financial ROI only when that source explicitly defines it this way; never invert it to cost/GMV. 2) Strict ROI = (return - cost) / cost, and may be calculated only when the source defines return and cost scope and requests that metric; never present a return multiple as strict ROI. 3) Incremental return multiple = attributable incremental GMV divided by campaign spend. If a claimed incremental ROI conflicts with this calculation, record a definition conflict and request the attribution model, return definition, numerator, and denominator; never choose either as a confirmed metric. 4) CTR = clicks/impressions; landing-page conversion = form submissions/landing-page visits; click-to-order conversion = orders/clicks; lead conversion = qualified leads/form submissions; deal conversion = deals/qualified leads. 5) Repeat-purchase rate = customers repurchasing in the stated period / total matching customer cohort; retention = active customers at period end / customers at period start. 6) Incremental conversion lift = treatment conversion rate - control conversion rate and requires comparable treatment/control cohorts and sample sizes. Supplied matching raw operands permit declared calculations with explicit definition conditions. Only missing operands are recalculation gaps; unverified attribution, return or cost definitions are checks. Different channel ratios alone are not contradictions.",
}

func nativeMarketingDefinitionMatches(current datatypes.JSON, expected map[string]any) bool {
	var actual map[string]any
	if err := json.Unmarshal(current, &actual); err != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	if actualErr != nil || expectedErr != nil {
		return false
	}
	// 声明中可包含 typed 工具依赖；结构体字段顺序与 JSONB map 顺序不同。
	// 两端均归一化为 JSON 对象后再比较，避免重复发布同一份语义声明。
	var normalizedExpected map[string]any
	if err := json.Unmarshal(expectedJSON, &normalizedExpected); err != nil {
		return false
	}
	expectedJSON, expectedErr = json.Marshal(normalizedExpected)
	return expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
}

type seedSkillPackageStore struct {
	manager    *mediamgr.MediaManager
	driverName string
	bucket     string
}

func (s seedSkillPackageStore) PutSkillPackage(ctx context.Context, objectKey, contentType string, body []byte) (string, error) {
	if s.manager == nil {
		return "", fmt.Errorf("seed_skill_package_store_unavailable")
	}
	driverName := strings.ToLower(strings.TrimSpace(s.driverName))
	uri, err := seedSkillPackageURI(driverName, s.bucket, objectKey)
	if err != nil {
		return "", err
	}
	bucket := ""
	if driverName == "s3" {
		bucket = strings.TrimSpace(s.bucket)
	}
	existing, err := s.manager.Get(ctx, driverName, mediad.GetObjectInput{Bucket: bucket, ObjectKey: objectKey})
	if err == nil {
		defer existing.Body.Close()
		existingBody, readErr := io.ReadAll(existing.Body)
		if readErr != nil {
			return "", fmt.Errorf("seed_skill_package_read_existing: %w", readErr)
		}
		if !bytes.Equal(existingBody, body) {
			return "", fmt.Errorf("seed_skill_package_object_conflict")
		}
		return uri, nil
	}
	if !errors.Is(err, mediad.ErrNotFound) {
		return "", fmt.Errorf("seed_skill_package_get_existing: %w", err)
	}
	if _, err := s.manager.Put(ctx, driverName, mediad.PutObjectInput{Bucket: bucket, ObjectKey: objectKey, Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: contentType, Overwrite: false}); err != nil {
		return "", err
	}
	return uri, nil
}

// seedSkillPackageURI records the configured Media Storage driver, never an
// OS path. local:// is resolved through storage.local.base_path by Media.
func seedSkillPackageURI(driverName, bucket, objectKey string) (string, error) {
	driverName = strings.ToLower(strings.TrimSpace(driverName))
	objectKey = strings.TrimPrefix(strings.TrimSpace(objectKey), "/")
	if objectKey == "" {
		return "", fmt.Errorf("seed_skill_package_object_key_required")
	}
	switch driverName {
	case "local":
		return "local://" + objectKey, nil
	case "s3":
		bucket = strings.TrimSpace(bucket)
		if bucket == "" {
			return "", fmt.Errorf("seed_skill_package_s3_bucket_required")
		}
		return "s3://" + bucket + "/" + objectKey, nil
	default:
		return "", fmt.Errorf("seed_skill_package_storage_driver_unsupported")
	}
}

func seedMediaHasDriver(manager *mediamgr.MediaManager, name string) bool {
	for _, item := range manager.Drivers() {
		if item == name {
			return true
		}
	}
	return false
}

func seedRootActorMemberUUID(ctx context.Context, db *gorm.DB, tenantUUID string) (string, error) {
	var root iammodel.User
	if err := db.WithContext(ctx).Where("is_root = ?", true).Take(&root).Error; err != nil {
		return "", fmt.Errorf("seed_native_marketing_skills_root_user_missing: %w", err)
	}
	var member iammodel.Member
	if err := db.WithContext(ctx).Where("tenant_uuid = ? AND user_uuid = ?", tenantUUID, root.UUID.String()).Take(&member).Error; err != nil {
		return "", fmt.Errorf("seed_native_marketing_skills_root_member_missing: %w", err)
	}
	if strings.TrimSpace(member.UUID.String()) == "" {
		return "", fmt.Errorf("seed_native_marketing_skills_root_member_uuid_missing")
	}
	return member.UUID.String(), nil
}

// SeedNativeMarketingSkills 只发布内置营销 Skill 的新 Revision，不重置 Agent、
// 团队配置或其他模块的种子数据；历史 Revision 和运行结果保持不可变。
func SeedNativeMarketingSkills(db *gorm.DB, cfg *appcfg.Config) error {
	if db == nil || cfg == nil {
		return fmt.Errorf("seed_native_marketing_skills_requires_db_and_config")
	}
	ctx := seedCtx()
	sysTenant, err := tenantrepo.NewTenantRepository(db).EnsureByKey(ctx, tenantmodel.SystemTenantKey, "System", tenantmodel.TenantPlanFree, tenantmodel.TenantTypeSystem)
	if err != nil {
		return err
	}
	actor, err := seedRootActorMemberUUID(ctx, db, sysTenant.UUID.String())
	if err != nil {
		return err
	}
	return seedNativeMarketingSkillDefinitions(ctx, db, cfg, sysTenant.UUID.String(), actor)
}

func SeedNativeMarketingAgents(db *gorm.DB, cfg *appcfg.Config) error {
	if db == nil || cfg == nil {
		return fmt.Errorf("seed_native_marketing_agents_requires_db_and_config")
	}
	ctx := seedCtx()
	env := envOrDefault("POWERX_ENV", "dev")

	tenantRepo := tenantrepo.NewTenantRepository(db)
	sysTenant, err := tenantRepo.EnsureByKey(ctx, tenantmodel.SystemTenantKey, "System", tenantmodel.TenantPlanFree, tenantmodel.TenantTypeSystem)
	if err != nil {
		return fmt.Errorf("ensure system tenant: %w", err)
	}
	tenantUUID := sysTenant.UUID.String()
	actorMemberUUID, err := seedRootActorMemberUUID(ctx, db, tenantUUID)
	if err != nil {
		return err
	}
	if err := seedNativeMarketingSkillDefinitions(ctx, db, cfg, tenantUUID, actorMemberUUID); err != nil {
		return err
	}

	agentIDs := make(map[string]uint64)
	for _, item := range nativeMarketingAgentSeeds() {
		agentID, errAgent := seedNativeMarketingAgent(ctx, db, env, tenantUUID, item)
		if errAgent != nil {
			return errAgent
		}
		if errBind := agentrepo.NewAgentSkillBindingRepository(db).Replace(ctx, env, &tenantUUID, agentID, item.SkillIDs); errBind != nil {
			return fmt.Errorf("bind native marketing skills for %s failed: %w", item.Key, errBind)
		}
		agentIDs[item.Key] = agentID
	}
	if err := seedMarketingCampaignReviewTeam(ctx, db, tenantUUID, agentIDs); err != nil {
		return err
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] native marketing agents ready")
	return nil
}

func seedMarketingCampaignReviewTeam(ctx context.Context, db *gorm.DB, tenantUUID string, agentIDs map[string]uint64) error {
	parentID := agentIDs[MarketingDirectorAdvisorAgentKey]
	if parentID == 0 {
		return fmt.Errorf("marketing campaign review coordinator is required")
	}
	for _, key := range []string{MarketingContentStrategistAgentKey, MarketingCampaignReviewerAgentKey, ExpertKnowledgeCuratorAgentKey} {
		if agentIDs[key] == 0 {
			return fmt.Errorf("marketing campaign review member is required: %s", key)
		}
	}
	orchestrationSpec, err := teamOrchestrationSpecJSON([]modelagent.TeamOrchestrationTask{
		{TaskID: "source_analysis", NodeKind: "agent_handoff", AssigneeRole: modelagent.TeamRoleRetriever, SkillID: MarketingSourceParseSkillID, Stage: 1, FailurePolicy: modelagent.FailurePolicyFailFast},
		{TaskID: "campaign_analysis", NodeKind: "agent_handoff", AssigneeRole: modelagent.TeamRoleExecutor, SkillID: MarketingMetricExtractSkillID, Stage: 1, FailurePolicy: modelagent.FailurePolicyFailFast},
		{TaskID: "knowledge_curation", NodeKind: "agent_handoff", AssigneeRole: modelagent.TeamRoleReviewer, SkillID: MarketingMethodologyExtractSkillID, Stage: 2, DependsOn: []string{"source_analysis", "campaign_analysis"}, FailurePolicy: modelagent.FailurePolicyFailFast},
		{TaskID: "campaign_review_synthesis", NodeKind: "skill", AssigneeRole: modelagent.TeamRolePlanner, SkillID: MarketingReviewSummarizeSkillID, Stage: 3, DependsOn: []string{"source_analysis", "campaign_analysis", "knowledge_curation"}, FailurePolicy: modelagent.FailurePolicyFailFast},
	})
	if err != nil {
		return err
	}

	displayNames, err := teamDisplayNameI18n(MarketingCampaignReviewTeamName, MarketingCampaignReviewTeamNameEN, MarketingCampaignReviewTeamNameJA, MarketingCampaignReviewTeamNameKO)
	if err != nil {
		return err
	}
	var teams []modelagent.AgentTeam
	err = db.WithContext(ctx).Where("tenant_uuid = ? AND team_key IN ?", strings.ToLower(strings.TrimSpace(tenantUUID)), []string{MarketingCampaignReviewTeamKey, MarketingCampaignReviewTeamName}).Order("id ASC").Find(&teams).Error
	if err != nil {
		return fmt.Errorf("find marketing campaign review team: %w", err)
	}
	if len(teams) > 1 {
		return fmt.Errorf("duplicated marketing campaign review seed teams require manual cleanup")
	}
	var team modelagent.AgentTeam
	if len(teams) == 1 {
		team = teams[0]
		err = nil
	} else {
		err = gorm.ErrRecordNotFound
	}
	switch err {
	case nil:
		if err = db.WithContext(ctx).Model(&modelagent.AgentTeam{}).Where("id = ?", team.ID).Updates(map[string]any{
			"team_key": MarketingCampaignReviewTeamKey, "display_name_i18n": displayNames,
			"parent_agent_id": parentID, "dispatch_mode": modelagent.DispatchModeMixed,
			"default_failure_policy": modelagent.FailurePolicyFailFast, "status": modelagent.TeamStatusActive,
			"orchestration_spec": orchestrationSpec,
			"updated_at":         time.Now().UTC(),
		}).Error; err != nil {
			return fmt.Errorf("update marketing campaign review team: %w", err)
		}
		if err = db.WithContext(ctx).First(&team, team.ID).Error; err != nil {
			return fmt.Errorf("reload marketing campaign review team: %w", err)
		}
	case gorm.ErrRecordNotFound:
		team = modelagent.AgentTeam{TenantUUID: tenantUUID, ParentAgentID: parentID, TeamKey: MarketingCampaignReviewTeamKey, DisplayNameI18n: displayNames, DispatchMode: modelagent.DispatchModeMixed, DefaultFailurePolicy: modelagent.FailurePolicyFailFast, Status: modelagent.TeamStatusActive, CreatedBy: "seed", OrchestrationSpec: orchestrationSpec}
		team.Normalize()
		if err = db.WithContext(ctx).Create(&team).Error; err != nil {
			return fmt.Errorf("create marketing campaign review team: %w", err)
		}
	default:
		return fmt.Errorf("find marketing campaign review team: %w", err)
	}

	members := []struct {
		key, role string
		priority  int
	}{
		{MarketingContentStrategistAgentKey, "retriever", 10},
		{MarketingCampaignReviewerAgentKey, "executor", 20},
		{ExpertKnowledgeCuratorAgentKey, "reviewer", 30},
	}
	memberRepo := repoagent.NewAgentTeamMemberRepository(db)
	for _, member := range members {
		if _, err = memberRepo.Upsert(ctx, &modelagent.AgentTeamMember{TeamID: team.ID, TenantUUID: tenantUUID, ChildAgentID: agentIDs[member.key], Role: member.role, Priority: member.priority, Enabled: true}); err != nil {
			return fmt.Errorf("upsert marketing campaign review team member %s: %w", member.key, err)
		}
	}
	return nil
}

func teamOrchestrationSpecJSON(tasks []modelagent.TeamOrchestrationTask) (datatypes.JSON, error) {
	raw, err := json.Marshal(modelagent.TeamOrchestrationSpec{Schema: modelagent.TeamOrchestrationSchemaV1, Tasks: tasks})
	if err != nil {
		return nil, fmt.Errorf("marshal team orchestration: %w", err)
	}
	if _, err := modelagent.ParseTeamOrchestrationSpec(raw); err != nil {
		return nil, err
	}
	return datatypes.JSON(raw), nil
}

func teamDisplayNameI18n(zhCN, enUS, ja, ko string) (datatypes.JSON, error) {
	raw, err := json.Marshal(map[string]string{"zh-CN": strings.TrimSpace(zhCN), "en-US": strings.TrimSpace(enUS), "ja": strings.TrimSpace(ja), "ko": strings.TrimSpace(ko)})
	if err != nil {
		return nil, fmt.Errorf("encode team display names: %w", err)
	}
	return datatypes.JSON(raw), nil
}

func nativeMarketingAgentSeeds() []nativeMarketingAgentSeed {
	return []nativeMarketingAgentSeed{
		{
			Key:           MarketingDirectorAdvisorAgentKey,
			Name:          "营销负责人智能体",
			NameEN:        "Marketing Director Advisor",
			Description:   "面向市场负责人，沉淀营销策略、渠道复盘、客户信号和跨团队协作方法论。",
			DescriptionEN: "For marketing leaders. Curates strategy, channel reviews, customer signals, and cross-team collaboration methodology.",
			Role:          "marketing_director",
			Category:      "marketing_growth",
			Scene:         "marketing.knowledge_curation",
			PromptSeed:    "你是营销活动复盘团队负责人。你只基于已传入的子任务产物汇总报告，必须区分已确认事实、待验证假设、数据缺口和行动建议；最终回复必须由已绑定的汇总 Skill 返回平台响应契约，PowerX 统一生成 Markdown、结论和验收项。不能回显原始材料，也不能把未经验证的归因写成事实。",
			SkillIDs:      []string{MarketingSourceParseSkillID, MarketingMethodologyExtractSkillID, MarketingMetricExtractSkillID, MarketingReviewSummarizeSkillID},
			WorkflowKeys:  []string{MarketingKnowledgeCaptureWorkflowKey, CampaignReviewToMethodologyWorkflowKey},
		},
		{
			Key:           MarketingContentStrategistAgentKey,
			Name:          "内容营销智能体",
			NameEN:        "Content Marketing Strategist",
			Description:   "面向内容团队，整理选题、脚本、素材复用和内容活动复盘，沉淀可复用内容方法。",
			DescriptionEN: "For content teams. Organizes topics, scripts, asset reuse, and content campaign reviews into reusable content methodology.",
			Role:          "content_strategist",
			Category:      "marketing_growth",
			Scene:         "marketing.content_strategy",
			PromptSeed:    "你是内容营销智能体。你负责把活动原始材料解析为可追溯的事实、渠道与素材信息、已知观察和约束；不得输出原材料摘要来替代结构化事实。",
			SkillIDs:      []string{MarketingSourceParseSkillID, MarketingMethodologyExtractSkillID},
			WorkflowKeys:  []string{MarketingKnowledgeCaptureWorkflowKey},
		},
		{
			Key:           ExpertKnowledgeCuratorAgentKey,
			Name:          "专家知识策展智能体",
			NameEN:        "Expert Knowledge Curator",
			Description:   "把专家访谈、会议纪要、文档和经验输入转成结构化知识草稿，并进入审核发布流程。",
			DescriptionEN: "Turns expert interviews, meeting notes, documents, and experience input into structured knowledge drafts for review and publishing.",
			Role:          "knowledge_curator",
			Category:      "knowledge_curation",
			Scene:         "knowledge.expert_curation",
			PromptSeed:    "你是专家知识策展智能体。你必须基于素材解析与指标分析产物提炼方法论，输出事实、待验证假设和下一轮验证动作；缺失证据必须明确标注，不得把推断写成事实，也不得自行创建验收阈值或数值目标。",
			SkillIDs:      []string{MarketingSourceParseSkillID, MarketingMethodologyExtractSkillID},
			WorkflowKeys:  []string{MarketingKnowledgeCaptureWorkflowKey},
		},
		{
			Key:           MarketingCampaignReviewerAgentKey,
			Name:          "活动复盘分析智能体",
			NameEN:        "Campaign Review Analyst",
			Description:   "面向营销活动复盘，抽取指标、问题、结论和优化动作，并沉淀为活动方法论知识。",
			DescriptionEN: "For campaign reviews. Extracts metrics, issues, conclusions, and optimization actions into campaign methodology knowledge.",
			Role:          "campaign_reviewer",
			Category:      "marketing_growth",
			Scene:         "marketing.campaign_review",
			PromptSeed:    "你是活动复盘分析智能体。你负责从活动数据中计算目标完成率和漏斗转化率，明确可由数据支持的结论与仍待验证的归因；输出结构化指标、发现和数据缺口。",
			SkillIDs:      []string{MarketingSourceParseSkillID, MarketingMetricExtractSkillID, MarketingReviewSummarizeSkillID, MarketingMethodologyExtractSkillID},
			WorkflowKeys:  []string{CampaignReviewToMethodologyWorkflowKey, MarketingKnowledgeCaptureWorkflowKey},
		},
	}
}

func seedNativeMarketingAgent(ctx context.Context, db *gorm.DB, env string, tenantUUID string, item nativeMarketingAgentSeed) (uint64, error) {
	if strings.TrimSpace(item.Key) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.NameEN) == "" {
		return 0, fmt.Errorf("native marketing agent seed requires key, name and name_en")
	}
	if len(item.WorkflowKeys) == 0 || strings.TrimSpace(item.WorkflowKeys[0]) == "" {
		return 0, fmt.Errorf("native marketing agent %s requires primary workflow key", item.Key)
	}
	if len(item.SkillIDs) == 0 {
		return 0, fmt.Errorf("native marketing agent %s requires at least one skill", item.Key)
	}
	agentRepo := agentrepo.NewAgentRepository(db)
	a := &agentmodel.Agent{
		Env:            env,
		TenantUUID:     &tenantUUID,
		Key:            item.Key,
		Name:           item.Name,
		Description:    item.Description,
		TypeID:         item.Role,
		Scene:          item.Scene,
		PromptSeed:     item.PromptSeed,
		Persona:        item.Role,
		Source:         "core",
		Scope:          agentmodel.AgentScopeTenant,
		Visibility:     agentmodel.AgentVisibilityTenant,
		Status:         agentmodel.AgentStatusActive,
		BlueprintRefs:  datatypes.JSON([]byte(`[]`)),
		IntentCardsRef: datatypes.JSON([]byte(`[]`)),
		ToolAllowlist:  datatypes.JSON([]byte(`[]`)),
		KBStrategy:     agentmodel.KBStrategyUnion,
		Meta: datatypes.JSONMap{
			"builtin":              true,
			"builtin_demo":         true,
			"business_demo":        "marketing_knowledge_curation",
			"protected":            true,
			"protect_from_delete":  true,
			"readonly_reason":      "core_seed_business_demo",
			"category":             item.Category,
			"role":                 item.Role,
			"managed_by":           "powerx_core_seed",
			"title_i18n":           map[string]string{"zh-CN": item.Name, "zh": item.Name, "en": item.NameEN, "en-US": item.NameEN},
			"description_i18n":     map[string]string{"zh-CN": item.Description, "zh": item.Description, "en": item.DescriptionEN, "en-US": item.DescriptionEN},
			"workflow_keys":        item.WorkflowKeys,
			"primary_workflow_key": item.WorkflowKeys[0],
			"input_modes":          []string{"text", "link", "asset_ref"},
			"clone_required":       true,
		},
	}
	if err := agentRepo.UpsertByScopeKey(ctx, env, &tenantUUID, a); err != nil {
		return 0, fmt.Errorf("upsert native marketing agent %s failed: %w", item.Key, err)
	}
	found, err := agentRepo.FindByScopeKey(ctx, env, &tenantUUID, item.Key)
	if err != nil {
		return 0, fmt.Errorf("find native marketing agent %s failed: %w", item.Key, err)
	}
	return found.ID, nil
}
