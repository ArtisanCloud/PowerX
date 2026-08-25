package shared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	knowledgemodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	metamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type workflowSkillInvoker struct {
	invoke *skillservice.InvokeService
}

func newWorkflowSkillInvoker(db *gorm.DB) workflowSkillInvoker {
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	return workflowSkillInvoker{invoke: skillservice.NewInvokeService(registryRepo, auditSvc)}
}

func (i workflowSkillInvoker) InvokeSkill(ctx context.Context, req workflowsvc.SkillInvokeRequest) (workflowsvc.SkillInvokeResponse, error) {
	if i.invoke == nil {
		return workflowsvc.SkillInvokeResponse{}, fmt.Errorf("workflow.skill_invoker_unavailable")
	}
	version := strings.TrimSpace(stringFromMap(req.Config, "skill_version", "version"))
	entrypoint := strings.TrimSpace(stringFromMap(req.Config, "entrypoint"))
	if entrypoint == "" {
		return workflowsvc.SkillInvokeResponse{}, fmt.Errorf("workflow.skill_entrypoint_required")
	}
	contextMap := map[string]interface{}{
		"source":          "workflow",
		"node_ref":        strings.TrimSpace(req.NodeRef),
		"agent_uuid":      strings.TrimSpace(req.AgentUUID),
		"trace_id":        strings.TrimSpace(req.TraceID),
		"workflow_config": req.Config,
	}
	if override, ok := req.Config["model_override"]; ok {
		contextMap["model_override"] = override
	}
	result, err := i.invoke.Execute(ctx, skillservice.InvokeRequest{
		TenantUUID: strings.TrimSpace(req.TenantUUID),
		SkillID:    strings.TrimSpace(req.SkillID),
		Version:    version,
		Entrypoint: entrypoint,
		InvokePath: "workflow.skill.invoke",
		TraceID:    strings.TrimSpace(req.TraceID),
	}, req.Input, contextMap)
	if err != nil {
		return workflowsvc.SkillInvokeResponse{}, err
	}
	return workflowsvc.SkillInvokeResponse{Output: result.Result}, nil
}

type workflowMetadataClassifier struct {
	db *gorm.DB
}

func (c workflowMetadataClassifier) ClassifyMetadata(ctx context.Context, req workflowsvc.MetadataClassifyRequest) (workflowsvc.MetadataClassifyResponse, error) {
	if c.db == nil {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata_classifier_unavailable")
	}
	tenantUUID := strings.TrimSpace(req.TenantUUID)
	if tenantUUID == "" {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.tenant_uuid_required")
	}
	taxonomy, err := metarepo.NewTaxonomyRepository(c.db).GetTaxonomyByNamespace(ctx, tenantUUID, req.TaxonomyNamespace)
	if err != nil {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.taxonomy_not_found: %w", err)
	}
	if taxonomy.Status != metamodel.StatusEnabled {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.taxonomy_disabled")
	}
	dictionary, err := metarepo.NewDictionaryRepository(c.db).GetNamespaceByNamespace(ctx, tenantUUID, req.DictionaryNamespace)
	if err != nil {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.dictionary_not_found: %w", err)
	}
	if dictionary.Status != metamodel.StatusEnabled {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.dictionary_disabled")
	}
	resourceType, err := metarepo.NewResourceTypeRepository(c.db).GetByResourceType(ctx, tenantUUID, req.ResourceTypeNamespace)
	if err != nil {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.resource_type_not_found: %w", err)
	}
	if resourceType.Status != metamodel.StatusEnabled {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.resource_type_disabled")
	}
	tags, total, err := metarepo.NewTagRepository(c.db).ListTags(ctx, metarepo.TagListOptions{
		TenantUUID:   tenantUUID,
		Namespace:    strings.TrimSpace(req.TagNamespace),
		ResourceType: strings.TrimSpace(req.ResourceTypeNamespace),
		Status:       metamodel.StatusEnabled,
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.tags_lookup_failed: %w", err)
	}
	if total == 0 {
		return workflowsvc.MetadataClassifyResponse{}, fmt.Errorf("workflow.metadata.tags_not_found")
	}

	out := cloneRuntimeMap(req.Input)
	out["metadata"] = map[string]interface{}{
		"classification_strategy": strings.TrimSpace(stringFromMap(req.Config, "classification_strategy")),
		"taxonomy_namespace":      strings.TrimSpace(req.TaxonomyNamespace),
		"taxonomy_uuid":           taxonomy.UUID.String(),
		"tag_namespace":           strings.TrimSpace(req.TagNamespace),
		"tag_count":               total,
		"sample_tag_codes":        sampleTagCodes(tags),
		"dictionary_namespace":    strings.TrimSpace(req.DictionaryNamespace),
		"dictionary_uuid":         dictionary.UUID.String(),
		"resource_type":           strings.TrimSpace(req.ResourceTypeNamespace),
		"resource_type_uuid":      resourceType.UUID.String(),
	}
	return workflowsvc.MetadataClassifyResponse{Output: out}, nil
}

type workflowKnowledgeOperator struct {
	db *gorm.DB
}

func (o workflowKnowledgeOperator) StageKnowledge(ctx context.Context, req workflowsvc.KnowledgeStageRequest) (workflowsvc.KnowledgeOperationResponse, error) {
	spaceUUID, err := o.requireActiveSpace(ctx, req.TenantUUID, req.KnowledgeSpaceUUID)
	if err != nil {
		return workflowsvc.KnowledgeOperationResponse{}, err
	}
	content := strings.TrimSpace(extractKnowledgeContent(req.Input))
	if content == "" {
		return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.content_required")
	}
	chunkUUID := uuid.New()
	metadataJSON, err := json.Marshal(map[string]interface{}{
		"workflow_state":   "review_pending",
		"draft_schema_ref": strings.TrimSpace(req.DraftSchemaRef),
		"input":            req.Input,
		"config":           req.Config,
		"staged_at":        time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return workflowsvc.KnowledgeOperationResponse{}, err
	}
	row := knowledgemodel.KnowledgeChunk{
		SpaceUUID: spaceUUID,
		ChunkUUID: chunkUUID,
		Kind:      "workflow_draft",
		Content:   content,
		Metadata:  datatypes.JSON(metadataJSON),
	}
	if err := o.db.WithContext(ctx).Create(&row).Error; err != nil {
		return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.stage_insert_failed: %w", err)
	}
	out := cloneRuntimeMap(req.Input)
	out["draft_refs"] = []map[string]interface{}{
		{
			"knowledge_space_uuid": spaceUUID.String(),
			"chunk_uuid":           chunkUUID.String(),
			"kind":                 row.Kind,
		},
	}
	return workflowsvc.KnowledgeOperationResponse{Output: out}, nil
}

func (o workflowKnowledgeOperator) PublishKnowledge(ctx context.Context, req workflowsvc.KnowledgePublishRequest) (workflowsvc.KnowledgeOperationResponse, error) {
	spaceUUID, err := o.requireActiveSpace(ctx, req.TenantUUID, req.KnowledgeSpaceUUID)
	if err != nil {
		return workflowsvc.KnowledgeOperationResponse{}, err
	}
	refs := extractDraftRefs(req.Input)
	if len(refs) == 0 {
		return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.draft_refs_required")
	}
	published := make([]map[string]interface{}, 0, len(refs))
	for _, ref := range refs {
		chunkUUID, err := uuid.Parse(ref)
		if err != nil {
			return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.invalid_draft_ref: %w", err)
		}
		result := o.db.WithContext(ctx).
			Model(&knowledgemodel.KnowledgeChunk{}).
			Where("space_uuid = ? AND chunk_uuid = ? AND kind = ?", spaceUUID, chunkUUID, "workflow_draft").
			Updates(map[string]interface{}{
				"kind":       "workflow_published",
				"updated_at": time.Now().UTC(),
			})
		if result.Error != nil {
			return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.publish_update_failed: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return workflowsvc.KnowledgeOperationResponse{}, fmt.Errorf("workflow.knowledge.draft_not_found")
		}
		published = append(published, map[string]interface{}{
			"knowledge_space_uuid": spaceUUID.String(),
			"chunk_uuid":           chunkUUID.String(),
			"kind":                 "workflow_published",
		})
	}
	out := cloneRuntimeMap(req.Input)
	out["published_refs"] = published
	out["publish_policy"] = strings.TrimSpace(req.PublishPolicy)
	return workflowsvc.KnowledgeOperationResponse{Output: out}, nil
}

func (o workflowKnowledgeOperator) requireActiveSpace(ctx context.Context, tenantUUID, spaceUUID string) (uuid.UUID, error) {
	if o.db == nil {
		return uuid.Nil, fmt.Errorf("workflow.knowledge_operator_unavailable")
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	parsed, err := uuid.Parse(strings.TrimSpace(spaceUUID))
	if err != nil {
		return uuid.Nil, fmt.Errorf("workflow.knowledge.invalid_space_uuid: %w", err)
	}
	var space knowledgemodel.KnowledgeSpace
	err = o.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", tenantUUID, parsed).
		Take(&space).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, fmt.Errorf("workflow.knowledge.space_not_found")
	}
	if err != nil {
		return uuid.Nil, err
	}
	if space.Status != knowledgemodel.KnowledgeSpaceStatusActive {
		return uuid.Nil, fmt.Errorf("workflow.knowledge.space_not_active")
	}
	return parsed, nil
}

func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}

func cloneRuntimeMap(in map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sampleTagCodes(tags []metamodel.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		code := strings.TrimSpace(tag.Code)
		if code != "" {
			out = append(out, code)
		}
	}
	return out
}

func extractKnowledgeContent(input map[string]any) string {
	for _, key := range []string{"content", "summary", "rendered_text", "text"} {
		if value := strings.TrimSpace(fmt.Sprintf("%v", input[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	if items, ok := input["draft_items"].([]interface{}); ok && len(items) > 0 {
		if first, ok := items[0].(map[string]interface{}); ok {
			return extractKnowledgeContent(first)
		}
	}
	return ""
}

func extractDraftRefs(input map[string]any) []string {
	raw, ok := input["draft_refs"]
	if !ok {
		raw = input["drafts"]
	}
	var out []string
	switch refs := raw.(type) {
	case []map[string]interface{}:
		for _, ref := range refs {
			if value := strings.TrimSpace(fmt.Sprintf("%v", ref["chunk_uuid"])); value != "" && value != "<nil>" {
				out = append(out, value)
			}
		}
	case []interface{}:
		for _, item := range refs {
			if ref, ok := item.(map[string]interface{}); ok {
				if value := strings.TrimSpace(fmt.Sprintf("%v", ref["chunk_uuid"])); value != "" && value != "<nil>" {
					out = append(out, value)
				}
			}
		}
	}
	return out
}
