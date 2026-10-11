package knowledge_space

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"sort"
	"strings"
	"time"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type ContentOverviewService struct {
	db         *gorm.DB
	repo       *repo.PublishedCorpusRepository
	semantic   *SemanticRuntime
	signingKey []byte
	clock      func() time.Time
}

func NewContentOverviewService(db *gorm.DB, semantic *SemanticRuntime, secret []byte) *ContentOverviewService {
	var key []byte
	if len(secret) >= 32 {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("powerx.knowledge.content-overview.cursor/v1"))
		key = mac.Sum(nil)
	}
	return &ContentOverviewService{db: db, repo: repo.NewPublishedCorpusRepository(db), semantic: semantic, signingKey: key, clock: time.Now}
}

type contentCursor struct {
	Version     int    `json:"v"`
	Parameters  string `json:"p"`
	Actor       string `json:"a"`
	Generation  string `json:"g"`
	Fingerprint string `json:"f"`
	After       string `json:"after"`
	ExpiresAt   int64  `json:"exp"`
}

func digestContent(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *ContentOverviewService) encodeCursor(c contentCursor) string {
	raw, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *ContentOverviewService) decodeCursor(value string) (contentCursor, error) {
	var c contentCursor
	invalid := semanticError(400, "KNOWLEDGE_CONTENT_CURSOR_INVALID")
	if len(value) > 4096 {
		return c, invalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return c, invalid
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return c, invalid
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return c, invalid
	}
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return c, invalid
	}
	if strictJSON(raw, &c) != nil || c.Version != 1 || !validSemanticUUID(c.After) {
		return c, invalid
	}
	if s.clock().Unix() >= c.ExpiresAt {
		return c, semanticError(409, "KNOWLEDGE_CONTENT_CURSOR_EXPIRED")
	}
	return c, nil
}

func contentActor(ctx context.Context) string {
	c := reqctx.GetClaims(ctx)
	if c == nil {
		return ""
	}
	if hash := reqctx.AuthenticatedAPIKeyHash(ctx); hash != "" {
		return digestContent([]string{"api_key", hash})
	}
	return digestContent([]any{c.Issuer, c.Subject, c.PluginID, c.UserID, c.MemberID})
}

func normalizeContentQuery(in ContentOverviewQuery) (ContentOverviewQuery, int, int, error) {
	if in.Schema != ContentOverviewQuerySchema || !validSemanticUUID(in.SpaceUUID) || (in.Mode != SemanticMode && in.Mode != HybridMode) {
		return in, 0, 0, semanticError(400, "KNOWLEDGE_CONTENT_INVALID_ARGUMENT")
	}
	in.SpaceUUID = canonicalSemanticUUID(in.SpaceUUID)
	if in.RequiredCorpusGeneration != "" {
		if !validSemanticUUID(in.RequiredCorpusGeneration) {
			return in, 0, 0, semanticError(400, "KNOWLEDGE_CONTENT_INVALID_ARGUMENT")
		}
		in.RequiredCorpusGeneration = canonicalSemanticUUID(in.RequiredCorpusGeneration)
	}
	page, preview := 20, 320
	if in.PageSize != nil {
		page = *in.PageSize
	}
	if in.PreviewCodepoints != nil {
		preview = *in.PreviewCodepoints
	}
	if page < 1 || page > 100 || preview < 0 || preview > 2000 {
		return in, 0, 0, semanticError(400, "KNOWLEDGE_CONTENT_INVALID_ARGUMENT")
	}
	return in, page, preview, nil
}

func (s *ContentOverviewService) Read(ctx context.Context, tenant string, input ContentOverviewQuery) (ContentOverviewResult, error) {
	missing := ContentDerivedStatus{Status: "not_available", ReasonCode: "KNOWLEDGE_CONTENT_DERIVED_NOT_PRODUCED"}
	out := ContentOverviewResult{Schema: ContentOverviewResultSchema, Items: []ContentOverviewItem{}, Overview: missing, Themes: missing, RecommendedQuestions: missing, AuthorizationScope: "current_credential_tenant_published_corpus", TraceID: reqctx.GetTraceID(ctx)}
	in, pageSize, previewSize, err := normalizeContentQuery(input)
	if err != nil {
		return out, err
	}
	out.SpaceUUID, out.Mode = in.SpaceUUID, in.Mode
	out.Pagination.PageSize = pageSize
	if s == nil || s.db == nil || s.semantic == nil || len(s.signingKey) == 0 {
		return out, semanticError(503, "KNOWLEDGE_CONTENT_UNAVAILABLE")
	}
	if err := s.authorize(ctx, tenant); err != nil {
		return out, err
	}
	actor := contentActor(ctx)
	if actor == "" {
		return out, KnowledgeUnauthorizedError(errors.New("trusted actor required"))
	}
	parameters := digestContent([]any{tenant, in.SpaceUUID, in.Mode, pageSize, previewSize})
	var cursor contentCursor
	if in.Cursor != "" {
		cursor, err = s.decodeCursor(in.Cursor)
		if err != nil {
			return out, err
		}
		if cursor.Parameters != parameters || cursor.Actor != actor {
			return out, semanticError(400, "KNOWLEDGE_CONTENT_CURSOR_SCOPE_MISMATCH")
		}
	}
	space, err := s.repo.Space(ctx, tenant, in.SpaceUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, KnowledgeSpaceNotFoundError(err)
	}
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	binding, err := s.repo.Binding(ctx, tenant, in.SpaceUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
	}
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if in.RequiredCorpusGeneration != "" && in.RequiredCorpusGeneration != binding.CorpusGeneration {
		return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
	}
	if in.Cursor != "" && cursor.Generation != binding.CorpusGeneration {
		return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
	}
	var modes []string
	if json.Unmarshal(binding.Modes, &modes) != nil {
		return out, semanticError(503, "KNOWLEDGE_CONTENT_UNAVAILABLE")
	}
	modeAllowed := false
	for _, mode := range modes {
		modeAllowed = modeAllowed || mode == in.Mode
	}
	if !modeAllowed {
		return out, semanticError(422, "KNOWLEDGE_SEMANTIC_MODE_NOT_READY")
	}
	profile, err := s.repo.Profile(ctx, tenant, binding.EmbeddingProfileUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, semanticError(422, "KNOWLEDGE_SEMANTIC_INDEX_NOT_READY")
	}
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if s.semantic.vectors == nil || s.semantic.vectors.Driver() != "pgvector" {
		return out, KnowledgeSemanticUnsupportedError()
	}
	if err = s.semantic.checkVectorDatabase(ctx); err != nil {
		return out, err
	}
	index, err := repo.NewKnowledgeVectorIndexRepository(s.db).FindBySpaceAndKey(ctx, space.UUID, binding.VectorIndexKey)
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if index == nil || index.Status != "active" || space.ActiveVectorIndexKey != binding.VectorIndexKey || index.Dimensions != profile.Dimensions {
		return out, semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
	}
	schema, err := quoteSemanticIdentifier(s.semantic.vectors.pgBase.WithDefaults().Schema)
	if err != nil {
		return out, err
	}
	table, err := quoteSemanticIdentifier(index.VectorTable)
	if err != nil {
		return out, err
	}
	vectorTable := schema + "." + table
	rows, err := s.repo.Articles(ctx, tenant, in.SpaceUUID, binding.ConfigurationGeneration, in.Mode, vectorTable, profile.Dimensions)
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	fingerprint := digestContent(rows)
	if in.Cursor != "" && cursor.Fingerprint != fingerprint {
		return out, semanticError(409, "KNOWLEDGE_CONTENT_SNAPSHOT_CONFLICT")
	}
	out.Pagination.Total = len(rows)
	start := 0
	if in.Cursor != "" {
		start = sort.Search(len(rows), func(i int) bool { return rows[i].DocumentUUID > cursor.After })
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	selected := rows[start:end]
	chunks, jobs, err := s.repo.PageArtifacts(ctx, tenant, in.SpaceUUID, selected)
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	for _, row := range selected {
		chunk, ok := chunks[row.ChunkUUID]
		job, jobOK := jobs[row.JobUUID]
		if !ok || !jobOK || chunk.DocumentUUID != row.DocumentUUID || chunk.JobUUID != row.JobUUID {
			return out, semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
		}
		var metadata struct {
			Semantic semanticChunkMetadata `json:"semantic"`
		}
		if strictJSONSemanticMetadata(chunk.Metadata, &metadata) != nil {
			return out, semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
		}
		m := metadata.Semantic
		source, err := semanticJobSource(job, row.DocumentUUID)
		if err != nil {
			return out, err
		}
		if m.ArtifactRole != "source_chunk" || m.ConfigurationGeneration != binding.ConfigurationGeneration || m.DocumentVersion != source.Version || chunk.Checksum != checksumBytes([]byte(chunk.Content)) {
			return out, semanticError(503, "KNOWLEDGE_RESULT_HYDRATION_FAILED")
		}
		if err := validateSemanticHydration(semanticCandidate{Content: chunk.Content}, m, source); err != nil {
			return out, err
		}
		runes := []rune(chunk.Content)
		size := len(runes)
		if size > previewSize {
			size = previewSize
		}
		out.Items = append(out.Items, ContentOverviewItem{DocumentUUID: row.DocumentUUID, Title: source.Title, URI: source.URI, DocumentVersion: m.DocumentVersion, BundleGeneration: row.JobUUID, ChunkUUID: row.ChunkUUID, ArtifactUUID: m.ArtifactUUID, SourceRef: m.SourceRef, Preview: ContentPreview{Kind: "published_source_chunk", Text: string(runes[:size]), PositionUnit: "unicode_codepoint", CharStart: m.SourceRef.CharStart, CharEnd: m.SourceRef.CharStart + size, Truncated: size < len(runes)}})
	}
	// Recheck publication and visibility after hydration; never serve a stale page.
	finalSpace, err := s.repo.Space(ctx, tenant, in.SpaceUUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, KnowledgeSpaceNotFoundError(err)
	}
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if finalSpace.ActiveVectorIndexKey != space.ActiveVectorIndexKey {
		return out, semanticError(409, "KNOWLEDGE_MODEL_BINDING_CONFLICT")
	}
	final, err := s.repo.Binding(ctx, tenant, in.SpaceUUID)
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if final.ConfigurationGeneration != binding.ConfigurationGeneration || final.CorpusGeneration != binding.CorpusGeneration {
		return out, semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
	}
	latest, err := s.repo.Articles(ctx, tenant, in.SpaceUUID, binding.ConfigurationGeneration, in.Mode, vectorTable, profile.Dimensions)
	if err != nil {
		return out, KnowledgeUpstreamDependencyError(err)
	}
	if digestContent(latest) != fingerprint {
		return out, semanticError(409, "KNOWLEDGE_CONTENT_SNAPSHOT_CONFLICT")
	}
	if err := s.authorize(ctx, tenant); err != nil {
		return out, err
	}
	out.Generation = SemanticGeneration{Env: profile.Env, SpaceUUID: in.SpaceUUID, ConfigurationGeneration: binding.ConfigurationGeneration, CorpusGeneration: binding.CorpusGeneration, EmbeddingProfile: HostTaskProfileRef{UUID: profile.UUID.String(), Version: profile.Version}, ModelKey: profile.ProfileKey, ModelRevision: profile.ModelRevision, Dimensions: profile.Dimensions, DistanceMetric: "cosine"}
	out.Pagination.HasMore = end < len(rows)
	if out.Pagination.HasMore {
		expires := s.clock().Add(15 * time.Minute).Unix()
		if in.Cursor != "" {
			expires = cursor.ExpiresAt
		}
		next := s.encodeCursor(contentCursor{Version: 1, Parameters: parameters, Actor: actor, Generation: binding.CorpusGeneration, Fingerprint: fingerprint, After: rows[end-1].DocumentUUID, ExpiresAt: expires})
		out.Pagination.NextCursor = &next
	}
	return out, nil
}

func (s *ContentOverviewService) authorize(ctx context.Context, tenant string) error {
	if cap.ServiceCredential(ctx) {
		actual, err := NewHostContractAccess(s.db).AuthorizeContentOverviewRead(ctx, reqctx.AuthenticatedAPIKeyHash(ctx))
		if err != nil {
			return err
		}
		if actual != tenant {
			return KnowledgeForbiddenError(errors.New("tenant context mismatch"))
		}
		return nil
	}
	claims := reqctx.GetClaims(ctx)
	actual, err := reqctx.RequireTenantUUID(ctx)
	if claims == nil || claims.UserID == 0 || err != nil || actual != tenant {
		return KnowledgeUnauthorizedError(errors.New("admin user identity required"))
	}
	if !claims.IsRoot && reqctx.GetMemberID(ctx) == 0 {
		return KnowledgeUnauthorizedError(errors.New("tenant member identity required"))
	}
	allowed, err := iam.NewRBACService(s.db).EnforceWithSource(ctx, iam.ActorContext{TenantUUID: tenant, IsRoot: claims.IsRoot}, tenant, reqctx.GetMemberID(ctx), "knowledge_space", "knowledge_content_overview", "read", "platform_capability")
	if err != nil {
		return KnowledgeUpstreamDependencyError(err)
	}
	if !allowed {
		return KnowledgeForbiddenError(errors.New("content overview read permission missing"))
	}
	return nil
}

// Chunk metadata also contains provenance fields outside the semantic object.
func strictJSONSemanticMetadata(raw []byte, out any) error { return json.Unmarshal(raw, out) }
