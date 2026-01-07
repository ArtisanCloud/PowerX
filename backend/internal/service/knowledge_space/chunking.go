package knowledge_space

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ChunkDocument converts processed document units into multi-granularity chunks.
// It emits at least: doc_summary, section_summary, chunk.
func ChunkDocument(spaceID uuid.UUID, format string, sourceURI string, units []DocumentUnit) []IngestionChunk {
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	src := strings.TrimSpace(sourceURI)

	chunks := make([]IngestionChunk, 0, 1+len(units)*2)

	docSummary := IngestionChunk{
		ID:      uuid.NewSHA1(spaceID, []byte("doc_summary|"+normalizedFormat+"|"+src)),
		Kind:    "doc_summary",
		Content: fmt.Sprintf("Summary for %s (%s)", src, normalizedFormat),
		Metadata: map[string]any{
			"format":     normalizedFormat,
			"source_uri": src,
			"provenance": map[string]any{},
		},
	}
	chunks = append(chunks, docSummary)

	for idx, unit := range units {
		prov := unit.Provenance
		if prov == nil {
			prov = map[string]any{}
		}
		sectionKey := fmt.Sprintf("section_summary|%s|%s|%d", normalizedFormat, src, idx+1)
		sectionSummary := IngestionChunk{
			ID:      uuid.NewSHA1(spaceID, []byte(sectionKey)),
			Kind:    "section_summary",
			Content: fmt.Sprintf("Section %d summary for %s", idx+1, src),
			Metadata: map[string]any{
				"format":     normalizedFormat,
				"source_uri": src,
				"provenance": prov,
				"section":    idx + 1,
			},
			Confidence: unit.Confidence,
		}
		chunks = append(chunks, sectionSummary)

		content := strings.TrimSpace(unit.Content)
		if content == "" {
			continue
		}
		contentKey := fmt.Sprintf("chunk|%s|%s|%d", normalizedFormat, src, idx+1)
		chunks = append(chunks, IngestionChunk{
			ID:      uuid.NewSHA1(spaceID, []byte(contentKey)),
			Kind:    "chunk",
			Content: content,
			Metadata: map[string]any{
				"format":     normalizedFormat,
				"source_uri": src,
				"provenance": prov,
				"section":    idx + 1,
			},
			Confidence: unit.Confidence,
		})
	}

	return chunks
}
