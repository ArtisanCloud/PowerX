// services/agent/intent/embedding_strategy.go
package intent

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract/embed"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

	"math"
	"sort"
)

type EmbeddingStrategy struct {
	M          *agent.Manager
	Vec        embed.Vectorizer
	AgentID    string  // 针对哪个 agent 的路由做索引
	Threshold  float64 // 命中阈值（0~1）
	indexBuilt bool
	// 简单 centroids：每个 flow 用 positive examples 的平均向量
	centroids map[string][]float32 // flowID -> vector
	// ✨ 新增：负例中心（或直接存原向量做 maxNegSim）
	negCentroids map[string][]float32   // flowID -> avg(neg)
	negBanks     map[string][][]float32 // 可选：保存每个 flow 的所有负例向量
	Alpha        float64                // ✨ 负例惩罚系数，默认 0.5~0.8
	Margin       float64                // ✨ 第一名与第二名的最小分差，默认 0.05~0.08
}

func (s *EmbeddingStrategy) Name() string { return "embedding" }

// services/agent/intent/embedding_strategy.go
func (s *EmbeddingStrategy) buildIndex(ctx context.Context) error {
	if s.indexBuilt && len(s.centroids) > 0 {
		return nil
	}
	specs := s.M.ListFlowRoutesByAgent(s.AgentID)
	s.centroids = make(map[string][]float32, len(specs))
	s.negCentroids = make(map[string][]float32, len(specs))
	s.negBanks = make(map[string][][]float32, len(specs))

	built := 0
	for _, sp := range specs {
		// 正例
		if len(sp.Examples.Positive) > 0 {
			embs, err := s.Vec.Embed(ctx, sp.Examples.Positive)
			if err == nil && len(embs) > 0 {
				s.centroids[sp.FlowID] = avg(embs)
				built++
			}
		}
		// ✨ 负例
		if len(sp.Examples.Negative) > 0 {
			negs, err := s.Vec.Embed(ctx, sp.Examples.Negative)
			if err == nil && len(negs) > 0 {
				s.negCentroids[sp.FlowID] = avg(negs)
				s.negBanks[sp.FlowID] = negs
			}
		}
	}
	s.indexBuilt = built > 0
	// 缺省参数
	if s.Alpha <= 0 {
		s.Alpha = 0.6
	} // 负例惩罚力度
	if s.Margin <= 0 {
		s.Margin = 0.06
	} // 最小边际
	return nil
}

func (s *EmbeddingStrategy) Match(ctx context.Context, text string) (*schemas.IntentResult, error) {
	if err := s.buildIndex(ctx); err != nil {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "buildIndex error: " + err.Error()}, nil
	}
	if len(s.centroids) == 0 {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "centroids=0"}, nil
	}

	qs, err := s.Vec.Embed(ctx, []string{text})
	if err != nil || len(qs) == 0 {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "query embed failed"}, nil
	}
	q := qs[0]

	type cand struct {
		flow  string
		score float64
	}
	cands := make([]cand, 0, len(s.centroids))
	for fid, pos := range s.centroids {
		posSim := cosine(q, pos)

		// ✨ 负例惩罚：取该 flow 负例的“最大相似”或与负例中心的相似
		negSim := 0.0
		if nb, ok := s.negBanks[fid]; ok && len(nb) > 0 {
			// 用 max-neg similarity 更严格
			maxNeg := -1.0
			for _, nv := range nb {
				sc := cosine(q, nv)
				if sc > maxNeg {
					maxNeg = sc
				}
			}
			if maxNeg > 0 {
				negSim = maxNeg
			}
		} else if neg, ok := s.negCentroids[fid]; ok && len(neg) > 0 {
			negSim = cosine(q, neg)
		}

		final := posSim - s.Alpha*negSim
		cands = append(cands, cand{flow: fid, score: final})
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) == 0 {
		return &schemas.IntentResult{Matched: false, Strategy: s.Name(), Reason: "no candidates"}, nil
	}

	best := cands[0]
	// ✨ 边际：top1 与 top2 相差不足，判为不确定
	if len(cands) >= 2 && (best.score-cands[1].score) < s.Margin {
		return &schemas.IntentResult{
			Matched:  false,
			FlowID:   best.flow,
			Score:    best.score,
			Strategy: s.Name(),
			Reason:   "margin too small",
		}, nil
	}

	_, bestAgent := s.M.GetIntentSpecByFlow(best.flow)
	matched := (best.score >= s.Threshold)
	reason := "cosine-pos - alpha*neg"
	if !matched {
		reason = "below threshold with neg penalty"
	}

	return &schemas.IntentResult{
		Matched:  matched,
		FlowID:   best.flow,
		AgentID:  bestAgent,
		Score:    best.score,
		Strategy: s.Name(),
		Reason:   reason,
	}, nil
}

/* helpers */
func avg(vs [][]float32) []float32 {
	if len(vs) == 0 {
		return nil
	}
	n := len(vs[0])
	out := make([]float32, n)
	for _, v := range vs {
		for i := 0; i < n; i++ {
			out[i] += v[i]
		}
	}
	inv := 1.0 / float32(len(vs))
	for i := 0; i < n; i++ {
		out[i] *= float32(inv)
	}
	return out
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		ai := float64(a[i])
		bi := float64(b[i])
		dot += ai * bi
		na += ai * ai
		nb += bi * bi
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
