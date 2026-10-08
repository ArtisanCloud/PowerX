package knowledge_space

const (
	SemanticIndexSchema  = "powerx.knowledge.semantic-index/v1"
	SemanticQuerySchema  = "powerx.knowledge.retrieval-query/v1"
	SemanticResultSchema = "powerx.knowledge.retrieval-result/v1"
	SemanticMode         = "semantic"
	HybridMode           = "hybrid"
)

type SemanticIndexingSettings struct {
	Mode             string             `json:"mode"`
	EmbeddingProfile HostTaskProfileRef `json:"embedding_profile"`
	ArtifactRoles    []string           `json:"artifact_roles"`
}

type SemanticIndexSnapshot struct {
	SemanticIndexingSettings
	ConfigurationGeneration string `json:"configuration_generation"`
	ModelKey string `json:"model_key"`
	ModelRevision string `json:"model_revision"`
	ConfigChecksum string `json:"config_checksum"`
	Dimensions int `json:"dimensions"`
	VectorIndexKey string `json:"vector_index_key"`
}

type SemanticExternalRef struct {
	DocumentUUID string `json:"document_uuid"`
}

type SemanticSourceRef struct {
	Kind         string `json:"kind"`
	SourceUUID   string `json:"source_uuid"`
	Version      string `json:"version"`
	Checksum     string `json:"checksum"`
	PositionUnit string `json:"position_unit"`
	CharStart    int    `json:"char_start"`
	CharEnd      int    `json:"char_end"`
}

type SemanticArtifactInput struct {
	UUID                 string            `json:"uuid"`
	Role                 string            `json:"role"`
	Text                 string            `json:"text"`
	Checksum             string            `json:"checksum"`
	Version              string            `json:"version"`
	SourceRef            SemanticSourceRef `json:"source_ref"`
	KnowledgeProfileUUID *string           `json:"knowledge_profile_uuid,omitempty"`
	CaseUUID             *string           `json:"case_uuid,omitempty"`
	CategoryCodes        []string          `json:"category_codes"`
	TagUUIDs             []string          `json:"tag_uuids"`
}

type SemanticFilters struct {
	DocumentUUIDs []string `json:"document_uuids"`
	CategoryCodes []string `json:"category_codes"`
	TagUUIDs      []string `json:"tag_uuids"`
	ArtifactRoles []string `json:"artifact_roles"`
}

type SemanticQuery struct {
	Schema              string            `json:"schema"`
	Query               string            `json:"query"`
	SpaceUUIDs          []string          `json:"space_uuids"`
	Mode                string            `json:"mode"`
	TopK                int               `json:"top_k"`
	Filters             SemanticFilters   `json:"filters"`
	RequiredGenerations map[string]string `json:"required_generations,omitempty"`
}

type SemanticScore struct {
	Source    string  `json:"source"`
	Score     float64 `json:"score"`
	ScoreType string  `json:"score_type"`
	Direction string  `json:"direction"`
}

type SemanticMatch struct {
	SpaceUUID            string               `json:"space_uuid"`
	DocumentUUID         string               `json:"document_uuid"`
	ChunkUUID            string               `json:"chunk_uuid"`
	ArtifactUUID         string               `json:"artifact_uuid"`
	ArtifactRole         string               `json:"artifact_role"`
	KnowledgeProfileUUID *string              `json:"knowledge_profile_uuid,omitempty"`
	CaseUUID             *string              `json:"case_uuid,omitempty"`
	Title                string               `json:"title"`
	Text                 string               `json:"text"`
	Score                float64              `json:"score"`
	ScoreType            string               `json:"score_type"`
	Scores               []SemanticScore      `json:"scores"`
	RetrievalSources     []string             `json:"retrieval_sources"`
	ExternalRef          *SemanticExternalRef `json:"external_ref,omitempty"`
	SourceRef            SemanticSourceRef    `json:"source_ref"`
	DocumentVersion      string               `json:"document_version"`
	BundleGeneration     string               `json:"bundle_generation"`
}

type SemanticGeneration struct {
	SpaceUUID               string             `json:"space_uuid"`
	ConfigurationGeneration string             `json:"configuration_generation"`
	CorpusGeneration        string             `json:"corpus_generation"`
	EmbeddingProfile        HostTaskProfileRef `json:"embedding_profile"`
	ModelKey                string             `json:"model_key"`
	ModelRevision           string             `json:"model_revision"`
	Dimensions              int                `json:"dimensions"`
	DistanceMetric          string             `json:"distance_metric"`
}

type SemanticResult struct {
	Schema        string               `json:"schema"`
	QueryUUID     string               `json:"query_uuid"`
	RequestedMode string               `json:"requested_mode"`
	EffectiveMode string               `json:"effective_mode"`
	Generations   []SemanticGeneration `json:"generations"`
	Items         []SemanticMatch      `json:"items"`
	TraceID       string               `json:"trace_id"`
}
