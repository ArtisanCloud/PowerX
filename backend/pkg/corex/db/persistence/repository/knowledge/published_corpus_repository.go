package knowledge

import (
	"context"
	"fmt"
	"regexp"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
)

// PublishedSemanticFrom is shared by retrieval and article enumeration. The
// vector table must already be quoted/validated by the caller.
func PublishedSemanticFrom(vectorTable string) string {
	return fmt.Sprintf(" FROM %s v JOIN %s c ON c.uuid=v.chunk_uuid AND c.space_uuid=v.space_uuid JOIN %s d ON d.uuid=c.document_uuid AND d.tenant_uuid=c.tenant_uuid AND d.space_uuid=c.space_uuid AND d.active_index_job_uuid=c.job_uuid JOIN %s j ON j.uuid=c.job_uuid AND j.tenant_uuid=c.tenant_uuid AND j.space_uuid=c.space_uuid AND j.status='succeeded' WHERE c.tenant_uuid=? AND c.space_uuid=? AND d.deleted_at IS NULL AND c.deleted_at IS NULL AND j.deleted_at IS NULL AND d.queryable=TRUE AND d.index_status='indexed' AND c.metadata->'semantic'->>'configuration_generation'=?", vectorTable, models.HostDocumentChunk{}.TableName(), models.TenantDocument{}.TableName(), models.IndexJob{}.TableName())
}

type PublishedArticle struct {
	DocumentUUID    string
	JobUUID         string
	VisibilityEpoch string
	ChunkUUID       string
}

type PublishedCorpusRepository struct {
	*base.BaseRepository[models.TenantDocument]
	db *gorm.DB
}

func NewPublishedCorpusRepository(db *gorm.DB) *PublishedCorpusRepository {
	return &PublishedCorpusRepository{BaseRepository: base.NewBaseRepository[models.TenantDocument](db), db: db}
}

func (r *PublishedCorpusRepository) Space(ctx context.Context, tenant, space string) (models.KnowledgeSpace, error) {
	var row models.KnowledgeSpace
	err := r.db.WithContext(ctx).Where("tenant_uuid=? AND uuid=? AND status<>?", tenant, space, models.KnowledgeSpaceStatusRetired).First(&row).Error
	return row, err
}
func (r *PublishedCorpusRepository) Binding(ctx context.Context, tenant, space string) (models.SemanticSpaceBinding, error) {
	var row models.SemanticSpaceBinding
	err := r.db.WithContext(ctx).Where("tenant_uuid=? AND space_uuid=?", tenant, space).First(&row).Error
	return row, err
}
func (r *PublishedCorpusRepository) Profile(ctx context.Context, tenant, id string) (models.SemanticEmbeddingProfile, error) {
	var row models.SemanticEmbeddingProfile
	err := r.db.WithContext(ctx).Where("tenant_uuid=? AND uuid=? AND status=?", tenant, id, "published").First(&row).Error
	return row, err
}
func (r *PublishedCorpusRepository) Articles(ctx context.Context, tenant, space, configuration, mode, vectorTable string, dimensions int) ([]PublishedArticle, error) {
	// This is a fixed identifier-only SQL fragment, never supplied by clients.
	if !regexp.MustCompile(`^"[a-zA-Z_][a-zA-Z0-9_]*"\."[a-zA-Z_][a-zA-Z0-9_]*"$`).MatchString(vectorTable) {
		return nil, gorm.ErrInvalidData
	}
	from := PublishedSemanticFrom(vectorTable) + " AND vector_dims(v.embedding)=? AND c.metadata->'semantic'->>'artifact_role'='source_chunk'"
	args := []any{tenant, space, configuration, dimensions}
	if mode == "hybrid" {
		from += " AND c.metadata->'semantic'->>'mode'='hybrid'"
	}
	// source offset and UUID define the first published source chunk. One item per document.
	statement := "SELECT DISTINCT ON (d.uuid) d.uuid AS document_uuid,c.job_uuid,COALESCE(d.visibility_epoch::text,'') AS visibility_epoch,c.uuid AS chunk_uuid" + from + " ORDER BY d.uuid ASC,(c.metadata->'semantic'->'source_ref'->>'char_start')::int ASC,c.uuid ASC"
	rows := []PublishedArticle{}
	err := r.db.WithContext(ctx).Raw(statement, args...).Scan(&rows).Error
	return rows, err
}
func (r *PublishedCorpusRepository) PageArtifacts(ctx context.Context, tenant, space string, rows []PublishedArticle) (map[string]models.HostDocumentChunk, map[string]models.IndexJob, error) {
	chunks := map[string]models.HostDocumentChunk{}
	jobs := map[string]models.IndexJob{}
	if len(rows) == 0 {
		return chunks, jobs, nil
	}
	chunkIDs := []string{}
	jobIDs := []string{}
	for _, r := range rows {
		chunkIDs = append(chunkIDs, r.ChunkUUID)
		jobIDs = append(jobIDs, r.JobUUID)
	}
	var chunkRows []models.HostDocumentChunk
	if err := r.db.WithContext(ctx).Where("tenant_uuid=? AND space_uuid=? AND uuid IN ?", tenant, space, chunkIDs).Find(&chunkRows).Error; err != nil {
		return nil, nil, err
	}
	var jobRows []models.IndexJob
	if err := r.db.WithContext(ctx).Where("tenant_uuid=? AND space_uuid=? AND uuid IN ? AND status=?", tenant, space, jobIDs, "succeeded").Find(&jobRows).Error; err != nil {
		return nil, nil, err
	}
	for _, c := range chunkRows {
		chunks[c.UUID.String()] = c
	}
	for _, j := range jobRows {
		jobs[j.UUID.String()] = j
	}
	return chunks, jobs, nil
}
