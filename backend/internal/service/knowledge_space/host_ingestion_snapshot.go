package knowledge_space

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const HostIngestionSnapshotSchema = "powerx.knowledge.document-ingestion/v1"
const KnowledgeReasonUnsupportedSetting = "KNOWLEDGE_UNSUPPORTED_INGESTION_SETTING"
const KnowledgeReasonProfileUnavailable = "KNOWLEDGE_PROFILE_UNAVAILABLE"
const KnowledgeReasonSnapshotInvalid = "KNOWLEDGE_SNAPSHOT_INVALID"

// ProfileRef 只接受租户内已发布的精确 UUID/版本，禁止 key 或 latest 引用。
type HostTaskProfileRef struct {
	UUID    string `json:"uuid"`
	Version int    `json:"version"`
}
type HostStrategyRef struct {
	UUID    string `json:"uuid"`
	Key     string `json:"key"`
	Version int    `json:"version"`
}

// 指针区分未提供与显式零/false，切块长度单位为 Unicode rune。
type HostIngestionSettings struct {
	Schema              string              `json:"schema"`
	Strategy            *HostStrategyRef    `json:"strategy,omitempty"`
	IngestionProfile    *HostTaskProfileRef `json:"ingestion_profile,omitempty"`
	ProcessorProfile    *HostTaskProfileRef `json:"processor_profile,omitempty"`
	MaskingProfile      *HostTaskProfileRef `json:"masking_profile,omitempty"`
	Priority            *string             `json:"priority,omitempty"`
	SegmentMode         *string             `json:"segment_mode,omitempty"`
	SegmentSizePolicy   *string             `json:"segment_size_policy,omitempty"`
	ChunkSize           *int                `json:"chunk_size,omitempty"`
	ChunkOverlap        *int                `json:"chunk_overlap,omitempty"`
	SegmentOrder        *[]string           `json:"segment_order,omitempty"`
	Separators          *[]string           `json:"separators,omitempty"`
	PagePriority        *bool               `json:"page_priority,omitempty"`
	AnchorHeadingPath   *bool               `json:"anchor_heading_path,omitempty"`
	AnchorClauseID      *bool               `json:"anchor_clause_id,omitempty"`
	AnchorRowNumber     *bool               `json:"anchor_row_number,omitempty"`
	AnchorSpeaker       *bool               `json:"anchor_speaker,omitempty"`
	AnchorSentenceIndex *bool               `json:"anchor_sentence_index,omitempty"`
}
type HostFrozenProfile struct {
	HostTaskProfileRef
	Key            string         `json:"key"`
	Config         datatypes.JSON `json:"config,omitempty"`
	ConfigChecksum string         `json:"config_checksum"`
}
type HostIngestionSnapshot struct {
	Indexing *SemanticIndexSnapshot `json:"indexing,omitempty"`
	Schema              string             `json:"schema"`
	IndexMode           string             `json:"index_mode"`
	LengthUnit          string             `json:"length_unit"`
	Strategy            *HostStrategyRef   `json:"strategy,omitempty"`
	IngestionProfile    *HostFrozenProfile `json:"ingestion_profile,omitempty"`
	IndexProfile        *HostFrozenProfile `json:"index_profile,omitempty"`
	RAGProfile          *HostFrozenProfile `json:"rag_profile,omitempty"`
	Priority            string             `json:"priority"`
	SegmentMode         string             `json:"segment_mode"`
	SegmentSizePolicy   string             `json:"segment_size_policy"`
	ChunkSize           int                `json:"chunk_size"`
	ChunkOverlap        int                `json:"chunk_overlap"`
	SegmentOrder        []string           `json:"segment_order"`
	Separators          []string           `json:"separators"`
	PagePriority        bool               `json:"page_priority"`
	AnchorHeadingPath   bool               `json:"anchor_heading_path"`
	AnchorClauseID      bool               `json:"anchor_clause_id"`
	AnchorRowNumber     bool               `json:"anchor_row_number"`
	AnchorSpeaker       bool               `json:"anchor_speaker"`
	AnchorSentenceIndex bool               `json:"anchor_sentence_index"`
	SourceChecksum      string             `json:"source_checksum"`
	SourceVersion       string             `json:"source_version"`
}

func unsupportedSetting(message string) error {
	return knowledgeError(http.StatusUnprocessableEntity, KnowledgeReasonUnsupportedSetting, errors.New(message))
}
func profileUnavailable() error {
	return knowledgeError(http.StatusUnprocessableEntity, KnowledgeReasonProfileUnavailable, errors.New("profile UUID/version is unavailable in current tenant"))
}
func checksumBytes(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func checksumJSON(body []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return ""
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return checksumBytes(canonical)
}
func (s *HostContractService) freezeIngestion(ctx context.Context, space *models.KnowledgeSpace, in HostDocumentInput) (HostIngestionSnapshot, error) {
	out := HostIngestionSnapshot{Schema: HostIngestionSnapshotSchema, IndexMode: "lexical_chunks", LengthUnit: "unicode_rune", Priority: "normal", SegmentMode: "unit", SegmentSizePolicy: "cap", ChunkSize: 1024, ChunkOverlap: 0, SegmentOrder: []string{"page", "segment", "separator", "size"}, Separators: []string{}, SourceChecksum: strings.ToLower(in.Checksum), SourceVersion: in.Version}
	// 空间绑定缺失只允许既有无配置文本入口；显式配置必须绑定真实 published Profile。
	for _, kind := range []string{"ingestion", "index", "rag"} {
		var id *uuid.UUID
		switch kind {
		case "ingestion":
			id = space.IngestionProfileUUID
		case "index":
			id = space.IndexProfileUUID
		case "rag":
			id = space.RAGProfileUUID
		}
		var requested *HostTaskProfileRef
		if in.Ingestion != nil && kind == "ingestion" {
			requested = in.Ingestion.IngestionProfile
		}
		if requested != nil {
			parsed, err := uuid.Parse(requested.UUID)
			if err != nil || parsed == uuid.Nil || requested.Version < 1 {
				return out, KnowledgeInvalidArgumentError(errors.New("invalid profile UUID/version"))
			}
			id = &parsed
		}
		if id == nil {
			if in.Ingestion != nil && kind == "ingestion" {
				return out, profileUnavailable()
			}
			continue
		}
		profile, err := s.freezeProfile(ctx, space.TenantUUID, kind, *id, requested)
		if err != nil {
			return out, err
		}
		switch kind {
		case "ingestion":
			out.IngestionProfile = profile
		case "index":
			out.IndexProfile = profile
		case "rag":
			out.RAGProfile = profile
		}
	}
	var flags []string
	_ = json.Unmarshal(space.FeatureFlags, &flags)
	for _, f := range flags {
		if strings.HasPrefix(f, "rag.strategy_package:") {
			out.Strategy = &HostStrategyRef{Key: strings.TrimPrefix(f, "rag.strategy_package:"), Version: 1}
		}
	}
	if out.Strategy != nil {
		for _, f := range flags {
			if strings.HasPrefix(f, "rag.strategy_version:") {
				v, err := strconv.Atoi(strings.TrimPrefix(f, "rag.strategy_version:"))
				if err != nil || v < 1 {
					return out, unsupportedSetting("invalid frozen space strategy version")
				}
				out.Strategy.Version = v
			}
		}
		out.Strategy.UUID = hostStrategyUUID(out.Strategy.Key, out.Strategy.Version)
	}
	if out.IngestionProfile != nil {
		var config map[string]json.RawMessage
		if json.Unmarshal(out.IngestionProfile.Config, &config) != nil {
			return out, profileUnavailable()
		}
		if raw := config["chunking"]; len(raw) > 0 {
			var defaults HostIngestionSettings
			if err := strictJSON(raw, &defaults); err != nil {
				return out, unsupportedSetting("unsupported ingestion Profile chunking configuration")
			}
			if defaults.Strategy != nil || defaults.IngestionProfile != nil || defaults.ProcessorProfile != nil || defaults.MaskingProfile != nil {
				return out, unsupportedSetting("Profile chunking cannot override profile references")
			}
			applyHostSettings(&out, &defaults)
		}
	}
	if settings := in.Ingestion; settings != nil {
		if settings.Schema != HostIngestionSnapshotSchema {
			return out, KnowledgeInvalidArgumentError(errors.New("ingestion.schema is required and must be powerx.knowledge.document-ingestion/v1"))
		}
		if settings.ProcessorProfile != nil || settings.MaskingProfile != nil {
			return out, unsupportedSetting("independent processor/masking Profile references are not supported by the TXT/Markdown Host processor")
		}
		if settings.Strategy != nil && (out.Strategy == nil || settings.Strategy.Version != out.Strategy.Version || (settings.Strategy.Key != "" && settings.Strategy.Key != out.Strategy.Key) || (settings.Strategy.UUID != "" && settings.Strategy.UUID != out.Strategy.UUID) || (settings.Strategy.Key == "" && settings.Strategy.UUID == "")) {
			return out, unsupportedSetting("task strategy must match the space strategy and version; strategy overrides are not supported")
		}
		applyHostSettings(&out, settings)
	}
	if err := validateHostSnapshot(out, in.ContentType); err != nil {
		return out, err
	}
	if err := validateHostChunkBudget(out, in.Content); err != nil {
		return out, err
	}
	return out, nil
}
func (s *HostContractService) freezeProfile(ctx context.Context, tenant, kind string, id uuid.UUID, ref *HostTaskProfileRef) (*HostFrozenProfile, error) {
	var table any
	switch kind {
	case "ingestion":
		table = &models.IngestionProfileVersion{}
	case "index":
		table = &models.IndexProfileVersion{}
	default:
		table = &models.RAGProfileVersion{}
	}
	var row struct {
		UUID       uuid.UUID
		ProfileKey string
		Version    int
		Config     datatypes.JSON
	}
	q := s.db.WithContext(ctx).Model(table).Select("uuid,profile_key,version,config").Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, id, models.ProfileStatusPublished)
	if ref != nil {
		q = q.Where("version = ?", ref.Version)
	}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, profileUnavailable()
		}
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	return &HostFrozenProfile{HostTaskProfileRef: HostTaskProfileRef{row.UUID.String(), row.Version}, Key: row.ProfileKey, Config: append(datatypes.JSON(nil), row.Config...), ConfigChecksum: checksumJSON(row.Config)}, nil
}
func applyHostSettings(out *HostIngestionSnapshot, in *HostIngestionSettings) {
	if in.Priority != nil {
		out.Priority = *in.Priority
	}
	if in.SegmentMode != nil {
		out.SegmentMode = *in.SegmentMode
	}
	if in.SegmentSizePolicy != nil {
		out.SegmentSizePolicy = *in.SegmentSizePolicy
	}
	if in.ChunkSize != nil {
		out.ChunkSize = *in.ChunkSize
	}
	if in.ChunkOverlap != nil {
		out.ChunkOverlap = *in.ChunkOverlap
	}
	if in.SegmentOrder != nil {
		out.SegmentOrder = append([]string{}, (*in.SegmentOrder)...)
	}
	if in.Separators != nil {
		out.Separators = append([]string{}, (*in.Separators)...)
	}
	if in.PagePriority != nil {
		out.PagePriority = *in.PagePriority
	}
	if in.AnchorHeadingPath != nil {
		out.AnchorHeadingPath = *in.AnchorHeadingPath
	}
	if in.AnchorClauseID != nil {
		out.AnchorClauseID = *in.AnchorClauseID
	}
	if in.AnchorRowNumber != nil {
		out.AnchorRowNumber = *in.AnchorRowNumber
	}
	if in.AnchorSpeaker != nil {
		out.AnchorSpeaker = *in.AnchorSpeaker
	}
	if in.AnchorSentenceIndex != nil {
		out.AnchorSentenceIndex = *in.AnchorSentenceIndex
	}
}
func validateHostSnapshot(in HostIngestionSnapshot, contentType string) error {
	if !slices.Contains([]string{"normal", "high"}, in.Priority) {
		return KnowledgeInvalidArgumentError(errors.New("invalid priority"))
	}
	if !slices.Contains([]string{"unit", "heading", "clause", "table_row", "code_block", "conversation"}, in.SegmentMode) {
		return unsupportedSetting("unsupported segment_mode")
	}
	if !slices.Contains([]string{"cap", "target"}, in.SegmentSizePolicy) {
		return unsupportedSetting("unsupported segment_size_policy")
	}
	if in.ChunkSize < 0 || in.ChunkSize > 65536 || in.ChunkOverlap < 0 || (in.ChunkSize == 0 && in.ChunkOverlap != 0) || (in.ChunkSize > 0 && in.ChunkOverlap >= in.ChunkSize) {
		return KnowledgeInvalidArgumentError(errors.New("chunk_size must be 0..65536; chunk_overlap must be 0..chunk_size-1, or zero for chunk_size=0"))
	}
	if in.ChunkSize == 0 && in.SegmentSizePolicy != "cap" {
		return unsupportedSetting("target size policy requires positive chunk_size")
	}
	if len(in.SegmentOrder) != 4 {
		return KnowledgeInvalidArgumentError(errors.New("segment_order must contain page, segment, separator and size once each"))
	}
	seen := map[string]bool{}
	for _, value := range in.SegmentOrder {
		if !slices.Contains([]string{"page", "segment", "separator", "size"}, value) || seen[value] {
			return KnowledgeInvalidArgumentError(errors.New("invalid or duplicate segment_order"))
		}
		seen[value] = true
	}
	if len(in.Separators) > 16 {
		return KnowledgeInvalidArgumentError(errors.New("too many separators"))
	}
	seen = map[string]bool{}
	for _, value := range in.Separators {
		if value == "" || utf8.RuneCountInString(value) > 32 || seen[value] || strings.Contains(value, `\n`) || strings.Contains(value, `\t`) {
			return KnowledgeInvalidArgumentError(errors.New("separators must be unique literal strings, using JSON escapes for newlines/tabs"))
		}
		seen[value] = true
	}
	if in.PagePriority {
		return unsupportedSetting("page_priority is unsupported for TXT/Markdown; submit PDF through its published ingestion route")
	}
	if (in.AnchorHeadingPath && in.SegmentMode != "heading") || (in.AnchorClauseID && in.SegmentMode != "clause") || (in.AnchorSentenceIndex && in.SegmentMode != "clause") || (in.AnchorRowNumber && in.SegmentMode != "table_row") || (in.AnchorSpeaker && in.SegmentMode != "conversation") {
		return unsupportedSetting("anchor option is incompatible with segment_mode")
	}
	if contentType != "text/markdown" && (in.SegmentMode == "heading" || in.SegmentMode == "table_row" || in.SegmentMode == "code_block") {
		return unsupportedSetting("this segment_mode requires text/markdown")
	}
	return nil
}

func HostDocumentIngestionCapabilities() map[string]any {
	return map[string]any{"schema": HostIngestionSnapshotSchema, "index_mode": "lexical_chunks", "content_types": []string{"text/plain", "text/markdown"}, "length_unit": "unicode_rune", "chunk_size_min": 0, "chunk_size_max": 65536, "priorities": []string{"normal", "high"}, "priority_semantics": "non_preemptive_high_first", "segment_modes": []string{"unit", "heading", "clause", "table_row", "code_block", "conversation"}, "segment_size_policies": []string{"cap", "target"}, "segment_order_items": []string{"page", "segment", "separator", "size"}, "processor_profile_supported": false, "masking_profile_supported": false, "page_priority_supported": false, "strategy_override_supported": false, "anchor_mode_requirements": map[string]string{"anchor_heading_path": "heading", "anchor_clause_id": "clause", "anchor_sentence_index": "clause", "anchor_row_number": "table_row", "anchor_speaker": "conversation"}, "defaults": map[string]any{"priority": "normal", "segment_mode": "unit", "segment_size_policy": "cap", "chunk_size": 1024, "chunk_overlap": 0, "segment_order": []string{"page", "segment", "separator", "size"}, "separators": []string{}, "page_priority": false, "anchors": false}, "max_content_chunks": 4096, "profile_precedence": []string{"explicit_request", "frozen_published_ingestion_profile.chunking", "core_defaults"}}
}

func hostStrategyUUID(key string, version int) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("powerx:knowledge:strategy:%s:v%d", key, version))).String()
}

func publicHostSnapshot(in HostIngestionSnapshot) HostIngestionSnapshot {
	for _, p := range []*HostFrozenProfile{in.IngestionProfile, in.IndexProfile, in.RAGProfile} {
		if p != nil {
			copy := *p
			copy.Config = nil
			switch {
			case p == in.IngestionProfile:
				in.IngestionProfile = &copy
			case p == in.IndexProfile:
				in.IndexProfile = &copy
			default:
				in.RAGProfile = &copy
			}
		}
	}
	return in
}

func validateHostChunkBudget(config HostIngestionSnapshot, content string) error {
	const maxChunks = 4096
	if config.ChunkSize > 0 {
		step := config.ChunkSize - config.ChunkOverlap
		if step <= 0 || utf8.RuneCountInString(content) > step*maxChunks {
			return KnowledgeInvalidArgumentError(errors.New("configuration exceeds the 4096 content-chunk budget"))
		}
	}
	boundaries := strings.Count(content, "\n")
	if config.SegmentMode == "clause" {
		boundaries += strings.Count(content, "。") + strings.Count(content, "；") + strings.Count(content, ";")
	}
	for _, separator := range config.Separators {
		boundaries += strings.Count(content, separator)
	}
	if boundaries > maxChunks {
		return KnowledgeInvalidArgumentError(errors.New("too many segment boundaries"))
	}
	return nil
}
