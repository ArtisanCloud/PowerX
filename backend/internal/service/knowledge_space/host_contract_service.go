package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const (
	KnowledgeReasonInvalidArgument    = "KNOWLEDGE_INVALID_ARGUMENT"
	KnowledgeReasonUnauthorized       = "KNOWLEDGE_UNAUTHORIZED"
	KnowledgeReasonForbidden          = "KNOWLEDGE_FORBIDDEN"
	KnowledgeReasonSpaceNotFound      = "KNOWLEDGE_SPACE_NOT_FOUND"
	KnowledgeReasonDocumentNotFound   = "KNOWLEDGE_DOCUMENT_NOT_FOUND"
	KnowledgeReasonIndexJobNotFound   = "KNOWLEDGE_INDEX_JOB_NOT_FOUND"
	KnowledgeReasonIndexConflict      = "KNOWLEDGE_INDEX_CONFLICT"
	KnowledgeReasonUpstreamDependency = "KNOWLEDGE_UPSTREAM_DEPENDENCY"

	HostIndexStatusQueued    = "queued"
	HostIndexStatusRunning   = "running"
	HostIndexStatusSucceeded = "succeeded"
	HostIndexStatusFailed    = "failed"

	HostDocumentStatusQueued  = "queued"
	HostDocumentStatusIndexed = "indexed"
	HostDocumentStatusDeleted = "deleted"

	HostIndexOperationUpsert  = "upsert"
	HostIndexOperationDelete  = "delete"
	HostIndexOperationRebuild = "rebuild"
)

// HostContractService owns tenant-scoped documents and their asynchronous
// indexing records. It never accepts a caller-provided tenant UUID.
type HostContractService struct {
	db *gorm.DB
}

func NewHostContractService(db *gorm.DB) *HostContractService {
	return &HostContractService{db: db}
}

type HostSpace struct {
	SpaceUUID string `json:"space_uuid"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

type HostDocumentInput struct {
	Title       string
	URI         string
	Content     string
	ContentType string
	Checksum    string
	Version     string
	Tags        []string
}

type HostDocumentJob struct {
	JobUUID      string `json:"job_uuid"`
	Status       string `json:"status"`
	Operation    string `json:"operation"`
	DocumentUUID string `json:"document_uuid,omitempty"`
}

type HostJob struct {
	JobUUID      string `json:"job_uuid"`
	SpaceUUID    string `json:"space_uuid"`
	DocumentUUID string `json:"document_uuid,omitempty"`
	Operation    string `json:"operation"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
}

type HostSearchCitation struct {
	SpaceUUID    string   `json:"space_uuid"`
	DocumentUUID string   `json:"document_uuid"`
	Title        string   `json:"title"`
	URI          string   `json:"uri"`
	Excerpt      string   `json:"excerpt"`
	Tags         []string `json:"tags"`
}

func (s *HostContractService) ListSpaces(ctx context.Context, tenantUUID string) ([]HostSpace, error) {
	if s == nil || s.db == nil {
		return nil, knowledgeError(http.StatusServiceUnavailable, KnowledgeReasonUpstreamDependency, errors.New("knowledge store unavailable"))
	}
	var rows []models.KnowledgeSpace
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ?", tenantUUID).Order("space_name ASC").Find(&rows).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := make([]HostSpace, 0, len(rows))
	for _, row := range rows {
		out = append(out, HostSpace{SpaceUUID: row.UUID.String(), Name: row.SpaceName, Status: row.Status})
	}
	return out, nil
}

func (s *HostContractService) Search(ctx context.Context, tenantUUID, query string, spaceUUIDs []string, limit int) ([]HostSearchCitation, error) {
	if s == nil || s.db == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge store unavailable"))
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, KnowledgeInvalidArgumentError(errors.New("query is required"))
	}
	if limit <= 0 || limit > 100 {
		return nil, KnowledgeInvalidArgumentError(errors.New("invalid limit"))
	}
	parsedSpaces, err := parseUniqueUUIDs(spaceUUIDs)
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	if len(parsedSpaces) > 0 {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.KnowledgeSpace{}).Where("tenant_uuid = ? AND uuid IN ?", tenantUUID, parsedSpaces).Count(&count).Error; err != nil {
			return nil, KnowledgeUpstreamDependencyError(err)
		}
		if count != int64(len(parsedSpaces)) {
			return nil, KnowledgeSpaceNotFoundError(errors.New("knowledge space not found"))
		}
	}
	base := s.db.WithContext(ctx).Model(&models.TenantDocument{}).
		Where("tenant_uuid = ? AND index_status = ?", tenantUUID, HostDocumentStatusIndexed).
		Where("(LOWER(title) LIKE ? OR LOWER(content) LIKE ?)", "%"+strings.ToLower(query)+"%", "%"+strings.ToLower(query)+"%")
	if len(parsedSpaces) > 0 {
		base = base.Where("space_uuid IN ?", parsedSpaces)
	}
	var rows []models.TenantDocument
	if err := base.Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := make([]HostSearchCitation, 0, len(rows))
	for _, row := range rows {
		out = append(out, HostSearchCitation{SpaceUUID: row.SpaceUUID, DocumentUUID: row.UUID.String(), Title: row.Title, URI: row.URI, Excerpt: searchExcerpt(row.Content, query), Tags: decodeTags(row.Tags)})
	}
	return out, nil
}

func (s *HostContractService) UpsertDocument(ctx context.Context, tenantUUID, spaceUUID string, in HostDocumentInput) (HostDocumentJob, error) {
	if s == nil || s.db == nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(errors.New("knowledge store unavailable"))
	}
	if err := validateDocumentInput(in); err != nil {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(err)
	}
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	space, err := s.requireSpace(ctx, tenantUUID, spaceUUID)
	if err != nil {
		return HostDocumentJob{}, err
	}
	_ = space
	tags, _ := json.Marshal(normalizeTags(in.Tags))
	var document models.TenantDocument
	var job models.IndexJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("tenant_uuid = ? AND space_uuid = ? AND uri = ?", tenantUUID, spaceUUID, strings.TrimSpace(in.URI)).First(&document).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			document = models.TenantDocument{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, Title: strings.TrimSpace(in.Title), URI: strings.TrimSpace(in.URI), Content: in.Content, ContentType: strings.TrimSpace(in.ContentType), Checksum: strings.ToLower(strings.TrimSpace(in.Checksum)), Version: strings.TrimSpace(in.Version), Tags: tags, IndexStatus: HostDocumentStatusQueued}
			if err := tx.Create(&document).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if document.Checksum == strings.ToLower(strings.TrimSpace(in.Checksum)) && document.IndexStatus != HostDocumentStatusDeleted {
				return KnowledgeIndexConflictError(errors.New("same document checksum is already accepted"))
			}
			document.Title, document.Content, document.ContentType = strings.TrimSpace(in.Title), in.Content, strings.TrimSpace(in.ContentType)
			document.Checksum, document.Version, document.Tags, document.IndexStatus, document.IndexedAt = strings.ToLower(strings.TrimSpace(in.Checksum)), strings.TrimSpace(in.Version), tags, HostDocumentStatusQueued, nil
			if err := tx.Save(&document).Error; err != nil {
				return err
			}
		}
		docUUID := document.UUID.String()
		job = models.IndexJob{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, DocumentUUID: &docUUID, Operation: HostIndexOperationUpsert, Status: HostIndexStatusQueued}
		return tx.Create(&job).Error
	})
	if err != nil {
		return HostDocumentJob{}, mapHostDBError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return HostDocumentJob{JobUUID: job.UUID.String(), Status: job.Status, Operation: job.Operation, DocumentUUID: document.UUID.String()}, nil
}

func (s *HostContractService) DeleteDocument(ctx context.Context, tenantUUID, spaceUUID, documentUUID string) (HostDocumentJob, error) {
	if _, err := s.requireSpace(ctx, tenantUUID, spaceUUID); err != nil {
		return HostDocumentJob{}, err
	}
	docID, err := uuid.Parse(strings.TrimSpace(documentUUID))
	if err != nil {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(err)
	}
	var document models.TenantDocument
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", tenantUUID, spaceUUID, docID).First(&document).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return HostDocumentJob{}, KnowledgeDocumentNotFoundError(err)
		}
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	docUUID := document.UUID.String()
	job := models.IndexJob{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, DocumentUUID: &docUUID, Operation: HostIndexOperationDelete, Status: HostIndexStatusQueued}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&document).Updates(map[string]any{"index_status": HostDocumentStatusDeleted}).Error; err != nil {
			return err
		}
		return tx.Create(&job).Error
	}); err != nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return HostDocumentJob{JobUUID: job.UUID.String(), Status: job.Status, Operation: job.Operation, DocumentUUID: docUUID}, nil
}

func (s *HostContractService) RebuildIndex(ctx context.Context, tenantUUID, spaceUUID string) (HostDocumentJob, error) {
	if _, err := s.requireSpace(ctx, tenantUUID, spaceUUID); err != nil {
		return HostDocumentJob{}, err
	}
	var active int64
	if err := s.db.WithContext(ctx).Model(&models.IndexJob{}).Where("tenant_uuid = ? AND space_uuid = ? AND operation = ? AND status IN ?", tenantUUID, spaceUUID, HostIndexOperationRebuild, []string{HostIndexStatusQueued, HostIndexStatusRunning}).Count(&active).Error; err != nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	if active > 0 {
		return HostDocumentJob{}, KnowledgeIndexConflictError(errors.New("rebuild already queued"))
	}
	job := models.IndexJob{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, Operation: HostIndexOperationRebuild, Status: HostIndexStatusQueued}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return HostDocumentJob{JobUUID: job.UUID.String(), Status: job.Status, Operation: job.Operation}, nil
}

func (s *HostContractService) GetIndexJob(ctx context.Context, tenantUUID, jobUUID string) (HostJob, error) {
	id, err := uuid.Parse(strings.TrimSpace(jobUUID))
	if err != nil {
		return HostJob{}, KnowledgeInvalidArgumentError(err)
	}
	var job models.IndexJob
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, id).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return HostJob{}, KnowledgeIndexJobNotFoundError(err)
		}
		return HostJob{}, KnowledgeUpstreamDependencyError(err)
	}
	out := HostJob{JobUUID: job.UUID.String(), SpaceUUID: job.SpaceUUID, Operation: job.Operation, Status: job.Status, ErrorCode: job.ErrorCode}
	if job.DocumentUUID != nil {
		out.DocumentUUID = *job.DocumentUUID
	}
	return out, nil
}

func (s *HostContractService) requireSpace(ctx context.Context, tenantUUID, spaceUUID string) (*models.KnowledgeSpace, error) {
	id, err := uuid.Parse(strings.TrimSpace(spaceUUID))
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	var space models.KnowledgeSpace
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, id).First(&space).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, KnowledgeSpaceNotFoundError(err)
		}
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	return &space, nil
}

func (s *HostContractService) runDocumentJobAsync(jobUUID uuid.UUID) {
	go func() {
		ctx := context.Background()
		var job models.IndexJob
		if err := s.db.WithContext(ctx).First(&job, "uuid = ?", jobUUID).Error; err != nil {
			return
		}
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&job).Updates(map[string]any{"status": HostIndexStatusRunning, "started_at": now}).Error; err != nil {
			return
		}
		var err error
		switch job.Operation {
		case HostIndexOperationUpsert:
			if job.DocumentUUID == nil {
				err = errors.New("document UUID missing")
			} else {
				err = s.db.WithContext(ctx).Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", job.TenantUUID, job.SpaceUUID, *job.DocumentUUID).Updates(map[string]any{"index_status": HostDocumentStatusIndexed, "indexed_at": time.Now()}).Error
			}
		case HostIndexOperationDelete:
			if job.DocumentUUID == nil {
				err = errors.New("document UUID missing")
			} else {
				err = s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", job.TenantUUID, job.SpaceUUID, *job.DocumentUUID).Delete(&models.TenantDocument{}).Error
			}
		case HostIndexOperationRebuild:
			err = s.db.WithContext(ctx).Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND space_uuid = ? AND index_status <> ?", job.TenantUUID, job.SpaceUUID, HostDocumentStatusDeleted).Updates(map[string]any{"index_status": HostDocumentStatusIndexed, "indexed_at": time.Now()}).Error
		default:
			err = errors.New("unsupported index operation")
		}
		completed := time.Now()
		updates := map[string]any{"completed_at": completed, "status": HostIndexStatusSucceeded, "error_code": ""}
		if err != nil {
			updates["status"], updates["error_code"] = HostIndexStatusFailed, KnowledgeReasonUpstreamDependency
		}
		_ = s.db.WithContext(ctx).Model(&job).Updates(updates).Error
	}()
}

func validateDocumentInput(in HostDocumentInput) error {
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.URI) == "" || strings.TrimSpace(in.Content) == "" || strings.TrimSpace(in.ContentType) == "" || strings.TrimSpace(in.Version) == "" {
		return errors.New("title, uri, content, content_type and version are required")
	}
	if contentType := strings.ToLower(strings.TrimSpace(in.ContentType)); contentType != "text/markdown" && contentType != "text/plain" {
		return errors.New("content_type must be text/markdown or text/plain")
	}
	checksum := strings.TrimSpace(in.Checksum)
	if len(checksum) != 64 {
		return errors.New("checksum must be a SHA-256 hex string")
	}
	for _, r := range checksum {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return errors.New("checksum must be a SHA-256 hex string")
		}
	}
	return nil
}

func parseUniqueUUIDs(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		value := id.String()
		if _, ok := seen[value]; ok {
			return nil, errors.New("duplicate UUID")
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, raw := range tags {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func decodeTags(raw []byte) []string { var tags []string; _ = json.Unmarshal(raw, &tags); return tags }
func searchExcerpt(content, query string) string {
	content = strings.TrimSpace(content)
	lowered := strings.ToLower(content)
	index := strings.Index(lowered, strings.ToLower(strings.TrimSpace(query)))
	if index < 0 {
		if len(content) > 320 {
			return content[:320]
		}
		return content
	}
	start := index - 120
	if start < 0 {
		start = 0
	}
	end := index + len(query) + 200
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

func KnowledgeInvalidArgumentError(err error) error {
	return knowledgeError(http.StatusBadRequest, KnowledgeReasonInvalidArgument, err)
}
func KnowledgeUnauthorizedError(err error) error {
	return knowledgeError(http.StatusUnauthorized, KnowledgeReasonUnauthorized, err)
}
func KnowledgeForbiddenError(err error) error {
	return knowledgeError(http.StatusForbidden, KnowledgeReasonForbidden, err)
}
func KnowledgeSpaceNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonSpaceNotFound, err)
}
func KnowledgeDocumentNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonDocumentNotFound, err)
}
func KnowledgeIndexJobNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonIndexJobNotFound, err)
}
func KnowledgeIndexConflictError(err error) error {
	return knowledgeError(http.StatusConflict, KnowledgeReasonIndexConflict, err)
}
func KnowledgeUpstreamDependencyError(err error) error {
	return knowledgeError(http.StatusServiceUnavailable, KnowledgeReasonUpstreamDependency, err)
}
func knowledgeError(status int, code string, err error) error {
	return dto.NewErrorWithCode(status, code, code, err)
}
func mapHostDBError(err error) error {
	if dto.CodeOf(err) != "" {
		return err
	}
	return KnowledgeUpstreamDependencyError(fmt.Errorf("knowledge persistence failed: %w", err))
}
