package knowledge_space

const (
	KnowledgeContentOverviewCapabilityID = "com.corex.knowledge.content_overview.service_read"
	ContentOverviewQuerySchema           = "powerx.knowledge.content-overview-query/v1"
	ContentOverviewResultSchema          = "powerx.knowledge.content-overview/v1"
)

type ContentOverviewQuery struct {
	Schema                   string `json:"schema"`
	SpaceUUID                string `json:"space_uuid"`
	Mode                     string `json:"mode"`
	RequiredCorpusGeneration string `json:"required_corpus_generation,omitempty"`
	PageSize                 *int   `json:"page_size,omitempty"`
	PreviewCodepoints        *int   `json:"preview_codepoints,omitempty"`
	Cursor                   string `json:"cursor,omitempty"`
}

type ContentPreview struct {
	Kind         string `json:"kind"`
	Text         string `json:"text"`
	PositionUnit string `json:"position_unit"`
	CharStart    int    `json:"char_start"`
	CharEnd      int    `json:"char_end"`
	Truncated    bool   `json:"truncated"`
}
type ContentOverviewItem struct {
	DocumentUUID     string            `json:"document_uuid"`
	Title            string            `json:"title"`
	URI              string            `json:"uri"`
	DocumentVersion  string            `json:"document_version"`
	BundleGeneration string            `json:"bundle_generation"`
	ChunkUUID        string            `json:"chunk_uuid"`
	ArtifactUUID     string            `json:"artifact_uuid"`
	SourceRef        SemanticSourceRef `json:"source_ref"`
	Preview          ContentPreview    `json:"preview"`
}
type ContentOverviewPagination struct {
	PageSize   int     `json:"page_size"`
	Total      int     `json:"total"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}
type ContentDerivedStatus struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code"`
}
type ContentOverviewResult struct {
	Schema               string                    `json:"schema"`
	SpaceUUID            string                    `json:"space_uuid"`
	Mode                 string                    `json:"mode"`
	Generation           SemanticGeneration        `json:"generation"`
	Items                []ContentOverviewItem     `json:"items"`
	Pagination           ContentOverviewPagination `json:"pagination"`
	Overview             ContentDerivedStatus      `json:"overview"`
	Themes               ContentDerivedStatus      `json:"themes"`
	RecommendedQuestions ContentDerivedStatus      `json:"recommended_questions"`
	AuthorizationScope   string                    `json:"authorization_scope"`
	TraceID              string                    `json:"trace_id"`
}
