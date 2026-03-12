package intent

import (
	"math"
	"sort"
	"strings"
	"time"
)

// SkillCandidate represents one candidate skill item for recall/rerank.
type SkillCandidate struct {
	SkillID   string
	Version   string
	Source    string
	UpdatedAt time.Time
}

// SkillCandidateScore is the scored output after recall/rerank.
type SkillCandidateScore struct {
	SkillCandidate
	Score float64
}

// RecallAndRerankSkills performs a lightweight lexical recall and scoring.
// It is deterministic and intentionally cheap for high-cardinality routing.
func RecallAndRerankSkills(query string, candidates []SkillCandidate, topK int) []SkillCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	if topK <= 0 {
		topK = 8
	}
	if topK > 64 {
		topK = 64
	}

	queryTokens := tokenizeSkillText(query)
	scored := make([]SkillCandidateScore, 0, len(candidates))
	for _, c := range candidates {
		score := sourcePrior(c.Source)
		if len(queryTokens) > 0 {
			text := strings.ToLower(strings.TrimSpace(c.SkillID + " " + c.Version))
			match := 0
			for _, tok := range queryTokens {
				if strings.Contains(text, tok) {
					match++
				}
			}
			score += float64(match) / float64(len(queryTokens))
		}
		score += recencyBonus(c.UpdatedAt)
		scored = append(scored, SkillCandidateScore{
			SkillCandidate: c,
			Score:          score,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if math.Abs(scored[i].Score-scored[j].Score) > 1e-9 {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].UpdatedAt.After(scored[j].UpdatedAt)
	})
	if len(scored) > topK {
		scored = scored[:topK]
	}
	return scored
}

func tokenizeSkillText(s string) []string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func sourcePrior(source string) float64 {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "builtin":
		return 0.30
	case "plugin":
		return 0.15
	default:
		return 0
	}
}

func recencyBonus(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	age := time.Since(t)
	if age <= 24*time.Hour {
		return 0.10
	}
	if age <= 7*24*time.Hour {
		return 0.05
	}
	return 0
}
