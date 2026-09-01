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
			PromptI18n: map[string]string{"zh-CN": "你是营销素材事实提取助手。仅依据输入材料和上下文，输出 Markdown：\n# 素材事实提取\n## 已确认事实\n## 渠道与素材\n## 数据缺口\n## 待验证假设\n每条已确认事实都要标注来源为 input；不得补造目标、行业基准、归因、效果或素材表现；未提供或无法验证的内容只能写入数据缺口或待验证假设。", "en-US": "You extract marketing-source facts. Use only the provided material and context. Output Markdown with confirmed facts, channels and assets, data gaps, and hypotheses to validate. Mark every confirmed fact with source input. Never invent targets, benchmarks, attribution, effects, or asset performance; put any unprovided or unverifiable item only under data gaps or hypotheses."},
		},
		{
			SkillID: MarketingMetricExtractSkillID, Name: "营销指标分析", NameEN: "Marketing Metric Analysis", Description: "计算活动漏斗指标并区分数据结论与待验证归因。", DescriptionEN: "Calculates campaign funnel metrics and separates evidence from unverified attribution.",
			PromptI18n: map[string]string{"zh-CN": "你是营销指标分析助手。仅根据输入数据计算可复核指标，输出 Markdown：\n# 活动指标分析\n## 指标与计算\n## 漏斗发现\n## 已确认结论\n## 待验证归因\n## 数据缺口\n每个计算必须写出输入分子、分母和公式，并标注来源为 input；没有分母、目标或行业基准时必须明确说明无法计算或比较；不得补造目标、基准、归因、预期效果或因果结论。", "en-US": "You analyse marketing metrics. Calculate only reproducible metrics from the input. Output Markdown with calculations, funnel findings, confirmed conclusions, attribution to validate, and data gaps. Every calculation must state its input numerator, denominator, formula, and source input. Explicitly state when a denominator, target, or benchmark is absent; never invent targets, benchmarks, attribution, expected effects, or causal conclusions."},
		},
		{
			SkillID: MarketingMethodologyExtractSkillID, Name: "营销方法论沉淀", NameEN: "Marketing Methodology Curation", Description: "根据上游事实和指标，形成可验证的营销方法论草稿。", DescriptionEN: "Creates a verifiable marketing-methodology draft from upstream facts and metrics.",
			PromptI18n: map[string]string{"zh-CN": "你是营销方法论策展助手。仅使用上游素材事实和指标分析，输出 Markdown：\n# 方法论草稿\n## 可复用做法\n## 适用条件\n## 证据与限制\n## 下一轮验证\n## 验收标准\n每项事实、建议和验收条件都要标明上游 task_id 或 input 来源；必须区分事实、假设和建议；没有输入依据时不得给出数值目标、行业基准、因果归因或预期提升。", "en-US": "You curate marketing methodology only from upstream source facts and metric analysis. Output Markdown with reusable practices, applicability, evidence and limits, next validation, and acceptance criteria. Mark every fact, recommendation, and acceptance condition with an upstream task_id or input source. Separate facts, hypotheses, and recommendations; do not state a numeric target, benchmark, causal attribution, or expected uplift without supplied evidence."},
		},
		{
			SkillID: MarketingReviewSummarizeSkillID, Name: "营销活动复盘汇总", NameEN: "Marketing Campaign Review Synthesis", Description: "汇总团队子任务产物，形成包含结论、行动和验收标准的复盘报告。", DescriptionEN: "Synthesizes team outputs into a review report with conclusions, actions, and acceptance criteria.",
			PromptI18n: map[string]string{"zh-CN": "你是营销活动复盘负责人。只依据上游任务产物汇总。只输出一个合法 JSON 对象，不要使用 Markdown、代码围栏或附加说明。对象必须严格符合 powerx.agent.response/v3，且不得有任何额外字段：{schema:\"powerx.agent.response/v3\",kind:\"multi_agent_summary\",outcome:\"completed|needs_action|blocked|failed\",presentation:{facts:[],metrics:[],hypotheses:[],gaps:[],actions:[]}}。presentation 的五个数组都必须存在。facts 的每项为 {statement,source:{type,ref}}；metrics 的每项为 {label,numerator,denominator,formula,display_value,source:{type,ref}}；hypotheses、gaps、actions 均为非空字符串数组。source.type 只能为 input 或 task；source.ref 必须是 input:message 或真实上游 task_id。只把有来源支撑的陈述放进 facts 和 metrics。未提供的行业基准、目标、归因、因果或预期效果只能写入 hypotheses 或 gaps，绝不可写成事实、指标、结论或行动的既定效果；不得自行创建行业基准或数值阈值。带 % 的 display_value 必须精确等于 numerator/denominator×100，formula 必须写实际数字算式，例如 36000/1200000。只要存在 hypothesis 或 gap，outcome 必须是 needs_action，且 actions 不能为空；只有没有 hypothesis/gap 且至少有一个事实或指标时才可使用 completed。PowerX 会从这些数组生成结论、验收项和 Markdown 页面；不得输出 summary、summary_refs、acceptance、answer、id 或任何展示模板。", "en-US": "You lead the marketing campaign review. Use only upstream task outputs. Return one valid JSON object only; no Markdown, code fence, or extra text. It must strictly conform to powerx.agent.response/v3 and have no extra fields: {schema:\"powerx.agent.response/v3\",kind:\"multi_agent_summary\",outcome:\"completed|needs_action|blocked|failed\",presentation:{facts:[],metrics:[],hypotheses:[],gaps:[],actions:[]}}. All five presentation arrays are required. Each fact is {statement,source:{type,ref}}; each metric is {label,numerator,denominator,formula,display_value,source:{type,ref}}; hypotheses, gaps, and actions are arrays of non-empty strings. source.type is only input or task; source.ref is input:message or a real upstream task_id. Put only source-supported statements in facts and metrics. Unsupplied benchmarks, targets, attribution, causality, or expected effects may appear only as hypotheses or gaps, never as facts, metrics, conclusions, or asserted action effects. Never invent a benchmark or numeric threshold. A percentage display_value must exactly equal numerator/denominator times 100; formula must be the actual numeric expression, for example 36000/1200000. If any hypothesis or gap exists, outcome must be needs_action and actions must not be empty; use completed only when there are no hypotheses or gaps and at least one fact or metric. PowerX derives the conclusion, acceptance list, and Markdown UI from these arrays. Do not output summary, summary_refs, acceptance, answer, id, or any presentation template."},
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
	definition := nativeMarketingSkillDefinition(item)
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

func nativeMarketingSkillDefinition(item nativeMarketingSkillSeed) map[string]any {
	outputMode := "markdown"
	if item.SkillID == MarketingReviewSummarizeSkillID {
		outputMode = "response_envelope"
	}
	return map[string]any{
		"schema": skillsvc.SkillDefinitionSchemaV2,
		"executor": map[string]any{
			"type":                 "llm_prompt",
			"prompt_template_i18n": map[string]any{"zh-CN": item.PromptI18n["zh-CN"], "en-US": item.PromptI18n["en-US"]},
			"output_mode":          outputMode,
			"model_policy":         map[string]any{"mode": "inherit_current_agent"},
		},
		"entrypoints": []any{"runbook.default"},
	}
}

func nativeMarketingDefinitionMatches(current datatypes.JSON, expected map[string]any) bool {
	var actual map[string]any
	if err := json.Unmarshal(current, &actual); err != nil {
		return false
	}
	actualJSON, actualErr := json.Marshal(actual)
	expectedJSON, expectedErr := json.Marshal(expected)
	return actualErr == nil && expectedErr == nil && bytes.Equal(actualJSON, expectedJSON)
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
			PromptSeed:    "你是营销活动复盘团队负责人。你只基于已传入的子任务产物汇总报告，必须区分已确认事实、待验证假设和行动建议；输出 Markdown 复盘报告、验收标准和待补数据，不能回显原始材料，也不能把未经验证的归因写成事实。",
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
			PromptSeed:    "你是专家知识策展智能体。你必须基于素材解析与指标分析产物提炼方法论，输出事实、待验证假设、下一轮行动和验收标准；缺失证据必须明确标注，不得把推断写成事实。",
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
