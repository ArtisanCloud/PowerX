package knowledge_space

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func validSemanticVector(v []float32, dimensions int) bool {
	if len(v) != dimensions || dimensions == 0 {
		return false
	}
	norm := float64(0)
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return false
		}
		norm += float64(x) * float64(x)
	}
	return norm > 0
}

// Exact tenant/visibility filtering joins authoritative Core records. This
// contract requires pgvector and those records to reside in the same database.
func (s *SemanticRuntime) checkVectorDatabase(ctx context.Context) error {
	if s.vectors == nil || s.vectors.Driver() != "pgvector" {
		return semanticError(501, "KNOWLEDGE_SEMANTIC_UNSUPPORTED")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, s.vectors.pgBase.DSN)
	if err != nil {
		return semanticError(503, "KNOWLEDGE_VECTOR_STORE_UNAVAILABLE")
	}
	defer conn.Close(context.WithoutCancel(ctx))
	const statement = "SELECT json_build_array(current_database(),COALESCE(inet_server_addr()::text,''),COALESCE(inet_server_port(),0))::text"
	var vectorIdentity, coreIdentity string
	if conn.QueryRow(ctx, statement).Scan(&vectorIdentity) != nil || s.db.WithContext(ctx).Raw(statement).Scan(&coreIdentity).Error != nil {
		return semanticError(503, "KNOWLEDGE_VECTOR_STORE_UNAVAILABLE")
	}
	if vectorIdentity != coreIdentity {
		return semanticError(501, "KNOWLEDGE_VECTOR_DATABASE_UNSUPPORTED")
	}
	return nil
}

func (s *SemanticRuntime) vectorTable(ctx context.Context, space string, snapshot *SemanticIndexSnapshot) (string, error) {
	index, err := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, uuid.MustParse(space), snapshot.VectorIndexKey)
	if err != nil || index == nil || index.Dimensions != snapshot.Dimensions {
		return "", semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
	}
	schema, err := quoteSemanticIdentifier(s.vectors.pgBase.WithDefaults().Schema)
	if err != nil {
		return "", err
	}
	table, err := quoteSemanticIdentifier(index.VectorTable)
	if err != nil {
		return "", err
	}
	return schema + "." + table, nil
}

func (s *SemanticRuntime) verifyVectors(ctx context.Context, job models.IndexJob, snapshot *SemanticIndexSnapshot, rows []models.HostDocumentChunk) error {
	// Unit fixtures use an injected store. Production never bypasses readback.
	if s.embedOverride != nil && s.vectors == nil {
		return nil
	}
	if err := s.checkVectorDatabase(ctx); err != nil {
		return err
	}
	table, err := s.vectorTable(ctx, job.SpaceUUID, snapshot)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UUID.String())
	}
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE space_uuid = ? AND chunk_uuid IN ? AND vector_dims(embedding) = ? AND metadata->>'host_job_uuid' = ?", table)
	if s.db.WithContext(ctx).Raw(query, job.SpaceUUID, ids, snapshot.Dimensions, job.UUID.String()).Scan(&count).Error != nil || count != int64(len(rows)) {
		return semanticError(503, "KNOWLEDGE_VECTOR_VERIFICATION_FAILED")
	}
	return nil
}

func validateSemanticHydration(row semanticCandidate, meta semanticChunkMetadata, source hostDocumentSource) error {
	for _, artifact := range source.Artifacts {
		if artifact.UUID != meta.ArtifactUUID {
			continue
		}
		r := meta.SourceRef
		original := artifact.SourceRef
		runes := []rune(artifact.Text)
		if artifact.Checksum != checksumBytes([]byte(artifact.Text)) || r.SourceUUID != original.SourceUUID || r.Kind != original.Kind || r.Version != artifact.Version || r.Checksum != artifact.Checksum || r.PositionUnit != "unicode_codepoint" || r.CharStart < 0 || r.CharEnd <= r.CharStart || r.CharEnd > len(runes) || string(runes[r.CharStart:r.CharEnd]) != row.Content {
			return semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
		}
		return nil
	}
	return semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
}

func semanticJobSource(job models.IndexJob, document string) (hostDocumentSource, error) {
	var source hostDocumentSource
	if job.Operation == HostIndexOperationUpsert {
		if strictJSON(job.SourceSnapshot, &source) == nil {
			return source, nil
		}
	}
	if job.Operation == HostIndexOperationRebuild {
		var batch hostRebuildBatch
		if json.Unmarshal(job.ConfigSnapshot, &batch) == nil {
			for _, item := range batch.Documents {
				if item.DocumentUUID == document {
					return item.Source, nil
				}
			}
		}
	}
	return source, semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
}

func (s *SemanticRuntime) cleanupVectors(ctx context.Context, space string, rows []models.HostDocumentChunk) {
	ids := []uuid.UUID{}
	for _, row := range rows {
		var metadata struct {
			Semantic *semanticChunkMetadata `json:"semantic"`
		}
		if json.Unmarshal(row.Metadata, &metadata) == nil && metadata.Semantic != nil {
			ids = append(ids, row.UUID)
		}
	}
	if len(ids) == 0 || s.vectorWriter == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.vectorWriter.DeleteByChunkIDs(cleanupCtx, uuid.MustParse(space), ids); err != nil {
		logger.WarnF(cleanupCtx, "semantic staged vector cleanup pending: space=%s error=%s", space, err.Error())
	}
}
