package knowledge_space

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type ChunkingOptions struct {
	// Mode controls how to split the unit content before windowing.
	// Supported: unit|heading|clause|semantic|table_row|code_block|conversation
	Mode string
	// ChunkSize is measured in runes (approx chars). 0 keeps legacy behavior (one chunk per unit).
	ChunkSize int
	// ChunkOverlap is measured in runes (approx chars). Only applies when ChunkSize > 0.
	ChunkOverlap int
	// Separators are preferred boundaries applied after mode splitting and before windowing.
	// It supports punctuation and newline tokens (e.g. "\n\n", "。", ";").
	Separators []string
	Anchors     ChunkAnchors
}

type ChunkAnchors struct {
	HeadingPath  bool
	ClauseID     bool
	RowNumber    bool
	Speaker      bool
	SentenceIndex bool
}

// ChunkDocument converts processed document units into multi-granularity chunks.
// It emits at least: doc_summary, section_summary, chunk.
func ChunkDocument(spaceID uuid.UUID, format string, sourceURI string, units []DocumentUnit, opts ChunkingOptions) []IngestionChunk {
	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	src := strings.TrimSpace(sourceURI)
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "unit"
	}

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

		parts := splitByModeWithMeta(content, mode)
		if len(parts) == 0 {
			parts = []segmentPart{{Text: content}}
		}

		chunkCounter := 0
		for partIdx, part := range parts {
			partText := strings.TrimSpace(part.Text)
			partMeta := part.Meta
			if partText == "" {
				continue
			}

			subParts := []segmentPart{{Text: partText, Meta: partMeta}}
			if mode != "table_row" && len(opts.Separators) > 0 {
				subParts = splitByCustomSeparatorsWithMeta(partText, partMeta, opts.Separators)
			}

			for subIdx, sub := range subParts {
				subText := strings.TrimSpace(sub.Text)
				if subText == "" {
					continue
				}

				// If chunkSize <= 0, keep each part as one chunk (no windowing).
				emitOne := opts.ChunkSize <= 0 || utf8.RuneCountInString(subText) <= opts.ChunkSize
				if emitOne {
					chunkCounter++
					contentKey := fmt.Sprintf("chunk|%s|%s|%d|%d", normalizedFormat, src, idx+1, chunkCounter)
					meta := map[string]any{
						"format":       normalizedFormat,
						"source_uri":   src,
						"provenance":   prov,
						"section":      idx + 1,
						"chunk_idx":    chunkCounter,
						"segment_mode": mode,
						"segment_part": partIdx + 1,
					}
					if len(subParts) > 1 {
						meta["segment_subpart"] = subIdx + 1
					}
					applyAnchors(meta, sub.Meta, opts.Anchors)
					chunks = append(chunks, IngestionChunk{
						ID:      uuid.NewSHA1(spaceID, []byte(contentKey)),
						Kind:    "chunk",
						Content:  subText,
						Metadata: meta,
						Confidence: unit.Confidence,
					})
					continue
				}

				overlap := opts.ChunkOverlap
				if overlap < 0 {
					overlap = 0
				}
				if overlap >= opts.ChunkSize {
					overlap = opts.ChunkSize / 4
				}
				step := opts.ChunkSize - overlap
				if step <= 0 {
					step = opts.ChunkSize
				}

				segments := splitByRuneWindow(subText, opts.ChunkSize, step)
				for _, seg := range segments {
					chunkCounter++
					contentKey := fmt.Sprintf("chunk|%s|%s|%d|%d", normalizedFormat, src, idx+1, chunkCounter)
					meta := map[string]any{
						"format":       normalizedFormat,
						"source_uri":   src,
						"provenance":   prov,
						"section":      idx + 1,
						"chunk_idx":    chunkCounter,
						"segment_mode": mode,
						"segment_part": partIdx + 1,
						"chunk_size":   opts.ChunkSize,
						"overlap":      overlap,
					}
					if len(subParts) > 1 {
						meta["segment_subpart"] = subIdx + 1
					}
					applyAnchors(meta, sub.Meta, opts.Anchors)
					chunks = append(chunks, IngestionChunk{
						ID:      uuid.NewSHA1(spaceID, []byte(contentKey)),
						Kind:    "chunk",
						Content: seg,
						Metadata: meta,
						Confidence: unit.Confidence,
					})
				}
			}
		}
	}

	return chunks
}

func splitByRuneWindow(s string, window int, step int) []string {
	if window <= 0 || step <= 0 {
		return []string{strings.TrimSpace(s)}
	}
	rs := []rune(s)
	out := make([]string, 0, (len(rs)/step)+1)
	for start := 0; start < len(rs); start += step {
		end := start + window
		if end > len(rs) {
			end = len(rs)
		}
		seg := strings.TrimSpace(string(rs[start:end]))
		if seg != "" {
			out = append(out, seg)
		}
		if end >= len(rs) {
			break
		}
	}
	return out
}

func splitByCustomSeparatorsWithMeta(s string, meta map[string]any, separators []string) []segmentPart {
	parts := splitByCustomSeparators(s, separators)
	if len(parts) == 0 {
		return []segmentPart{{Text: strings.TrimSpace(s), Meta: meta}}
	}
	out := make([]segmentPart, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, segmentPart{Text: p, Meta: meta})
	}
	if len(out) == 0 {
		return []segmentPart{{Text: strings.TrimSpace(s), Meta: meta}}
	}
	return out
}

func splitByCustomSeparators(s string, separators []string) []string {
	raw := strings.TrimSpace(s)
	if raw == "" || len(separators) == 0 {
		return []string{raw}
	}
	seps := make([]string, 0, len(separators))
	for _, sep := range separators {
		sep = strings.TrimSpace(sep)
		if sep == "" {
			continue
		}
		seps = append(seps, sep)
		if len(seps) >= 32 {
			break
		}
	}
	if len(seps) == 0 {
		return []string{raw}
	}

	out := make([]string, 0, 16)
	rest := raw
	for {
		bestIdx := -1
		bestSep := ""
		for _, sep := range seps {
			i := strings.Index(rest, sep)
			if i < 0 {
				continue
			}
			if bestIdx < 0 || i < bestIdx || (i == bestIdx && len(sep) > len(bestSep)) {
				bestIdx = i
				bestSep = sep
			}
		}
		if bestIdx < 0 {
			rest = strings.TrimSpace(rest)
			if rest != "" {
				out = append(out, rest)
			}
			break
		}
		cut := bestIdx + len(bestSep)
		part := strings.TrimSpace(rest[:cut])
		if part != "" {
			out = append(out, part)
		}
		rest = rest[cut:]
		if len(out) >= 512 {
			rest = strings.TrimSpace(rest)
			if rest != "" {
				out = append(out, rest)
			}
			break
		}
	}
	if len(out) == 0 {
		return []string{raw}
	}
	return out
}

var (
	reHeadingMD   = regexp.MustCompile(`(?m)^#{1,6}\s+.+$`)
	reHeadingLine = regexp.MustCompile(`^#{1,6}\s+.+$`)
	reHeadingParse = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	reClauseNum   = regexp.MustCompile(`(?m)^(?:\d+(?:\.\d+)*|第[一二三四五六七八九十百千万]+条)[\s、.．)]`)
	reClauseParse = regexp.MustCompile(`^(?:(\d+(?:\.\d+)*)|(第[一二三四五六七八九十百千万]+条))[\s、.．)]`)
	reSpeakerLine = regexp.MustCompile(`(?m)^(?:[\\p{L}0-9_\\-]{1,20})[:：]`)
)

type segmentPart struct {
	Text string
	Meta map[string]any
}

func splitByModeWithMeta(content string, mode string) []segmentPart {
	switch mode {
	case "heading":
		return splitByMarkdownHeadingsWithMeta(content)
	case "clause":
		return splitByClauseWithMeta(content)
	case "semantic":
		return splitBySentencesWithMeta(content)
	case "table_row":
		return splitByLinesWithMeta(content)
	case "code_block":
		return splitByCodeBlocksWithMeta(content)
	case "conversation":
		return splitByConversationTurnsWithMeta(content)
	case "unit":
		fallthrough
	default:
		return []segmentPart{{Text: content}}
	}
}

func splitByMarkdownHeadingsWithMeta(s string) []segmentPart {
	if !reHeadingMD.MatchString(s) {
		return splitByParagraphsWithMeta(s)
	}
	lines := strings.Split(s, "\n")
	var out []segmentPart
	var buf []string
	var sectionMeta map[string]any
	var headingStack []struct {
		level int
		title string
	}

	flush := func() {
		txt := strings.TrimSpace(strings.Join(buf, "\n"))
		if txt == "" {
			buf = nil
			return
		}
		meta := map[string]any{}
		for k, v := range sectionMeta {
			meta[k] = v
		}
		out = append(out, segmentPart{Text: txt, Meta: meta})
		buf = nil
	}

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if reHeadingLine.MatchString(trim) {
			if len(buf) > 0 {
				flush()
			}
			m := reHeadingParse.FindStringSubmatch(trim)
			level := 1
			title := trim
			if len(m) == 3 {
				level = len(m[1])
				title = strings.TrimSpace(m[2])
			}
			// adjust stack to current level
			for len(headingStack) > 0 && headingStack[len(headingStack)-1].level >= level {
				headingStack = headingStack[:len(headingStack)-1]
			}
			headingStack = append(headingStack, struct {
				level int
				title string
			}{level: level, title: title})
			path := make([]string, 0, len(headingStack))
			for _, h := range headingStack {
				path = append(path, h.title)
			}
			sectionMeta = map[string]any{
				"heading_level": level,
				"heading_title": title,
				"heading_path":  path,
			}
			buf = append(buf, line)
			continue
		}
		if len(buf) == 0 && strings.TrimSpace(line) != "" && sectionMeta == nil {
			// preface text before first heading
			sectionMeta = map[string]any{}
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

func splitByClauseWithMeta(s string) []segmentPart {
	if !reClauseNum.MatchString(s) {
		return splitByParagraphsWithMeta(s)
	}
	lines := strings.Split(s, "\n")
	var out []segmentPart
	var buf []string
	var clauseID string
	flush := func() {
		txt := strings.TrimSpace(strings.Join(buf, "\n"))
		if txt == "" {
			buf = nil
			return
		}
		meta := map[string]any{}
		if clauseID != "" {
			meta["clause_id"] = clauseID
		}
		out = append(out, segmentPart{Text: txt, Meta: meta})
		buf = nil
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if reClauseNum.MatchString(trim) {
			if len(buf) > 0 {
				flush()
			}
			m := reClauseParse.FindStringSubmatch(trim)
			switch {
			case len(m) == 3 && m[1] != "":
				clauseID = m[1]
			case len(m) == 3 && m[2] != "":
				clauseID = m[2]
			default:
				clauseID = strings.TrimSpace(reClauseNum.FindString(trim))
			}
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

func splitByConversationTurnsWithMeta(s string) []segmentPart {
	if !reSpeakerLine.MatchString(s) {
		return splitByParagraphsWithMeta(s)
	}
	lines := strings.Split(s, "\n")
	var out []segmentPart
	var buf []string
	var speaker string
	flush := func() {
		txt := strings.TrimSpace(strings.Join(buf, "\n"))
		if txt == "" {
			buf = nil
			return
		}
		meta := map[string]any{}
		if speaker != "" {
			meta["speaker"] = speaker
		}
		out = append(out, segmentPart{Text: txt, Meta: meta})
		buf = nil
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if reSpeakerLine.MatchString(trim) {
			if len(buf) > 0 {
				flush()
			}
			// speaker: content
			if idx := strings.IndexAny(trim, ":："); idx > 0 {
				speaker = strings.TrimSpace(trim[:idx])
			} else {
				speaker = ""
			}
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

func splitBySentencesWithMeta(s string) []segmentPart {
	// Simple heuristic: split by Chinese/English sentence punctuation.
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil
	}
	seps := func(r rune) bool {
		switch r {
		case '。', '！', '？', '.', '!', '?', ';', '；':
			return true
		default:
			return false
		}
	}
	var out []segmentPart
	var buf []rune
	sentenceIdx := 0
	for _, r := range []rune(raw) {
		buf = append(buf, r)
		if seps(r) {
			txt := strings.TrimSpace(string(buf))
			if txt != "" {
				sentenceIdx++
				out = append(out, segmentPart{Text: txt, Meta: map[string]any{"sentence_idx": sentenceIdx}})
			}
			buf = nil
		}
	}
	if len(buf) > 0 {
		txt := strings.TrimSpace(string(buf))
		if txt != "" {
			sentenceIdx++
			out = append(out, segmentPart{Text: txt, Meta: map[string]any{"sentence_idx": sentenceIdx}})
		}
	}
	// If too few sentences, fallback to paragraphs.
	if len(out) <= 1 {
		return splitByParagraphsWithMeta(raw)
	}
	return out
}

func splitByLinesWithMeta(s string) []segmentPart {
	lines := strings.Split(s, "\n")
	out := make([]segmentPart, 0, len(lines))
	row := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		row++
		out = append(out, segmentPart{Text: line, Meta: map[string]any{"row_number": row}})
	}
	if len(out) == 0 {
		return splitByParagraphsWithMeta(s)
	}
	return out
}

func splitByCodeBlocksWithMeta(s string) []segmentPart {
	// Very lightweight: split by blank lines, keep blocks.
	return splitByParagraphsWithMeta(s)
}

func splitByParagraphsWithMeta(s string) []segmentPart {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "\n\n")
	out := make([]segmentPart, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, segmentPart{Text: p})
		}
	}
	if len(out) == 0 {
		return []segmentPart{{Text: raw}}
	}
	return out
}

func applyAnchors(meta map[string]any, partMeta map[string]any, anchors ChunkAnchors) {
	if meta == nil || partMeta == nil {
		return
	}
	out := map[string]any{}
	if anchors.HeadingPath {
		if v, ok := partMeta["heading_path"]; ok {
			out["heading_path"] = v
		}
		if v, ok := partMeta["heading_level"]; ok {
			out["heading_level"] = v
		}
		if v, ok := partMeta["heading_title"]; ok {
			out["heading_title"] = v
		}
	}
	if anchors.ClauseID {
		if v, ok := partMeta["clause_id"]; ok {
			out["clause_id"] = v
		}
	}
	if anchors.RowNumber {
		if v, ok := partMeta["row_number"]; ok {
			out["row_number"] = v
		}
	}
	if anchors.Speaker {
		if v, ok := partMeta["speaker"]; ok {
			out["speaker"] = v
		}
	}
	if anchors.SentenceIndex {
		if v, ok := partMeta["sentence_idx"]; ok {
			out["sentence_idx"] = v
		}
	}
	if len(out) > 0 {
		meta["anchors"] = out
	}
}
