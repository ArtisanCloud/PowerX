package knowledge_space

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	strategy_catalog "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/strategy_catalog"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func inferSceneAndBundle(space *models.KnowledgeSpace) (sceneKey string, bundleKey string) {
	if space != nil {
		flags := FeatureFlagsFromJSON(space.FeatureFlags)
		for _, f := range flags {
			f = strings.ToLower(strings.TrimSpace(f))
			if strings.HasPrefix(f, "rag.scene:") {
				sceneKey = strings.TrimSpace(strings.TrimPrefix(f, "rag.scene:"))
			}
			if strings.HasPrefix(f, "rag.bundle:") {
				bundleKey = strings.TrimSpace(strings.TrimPrefix(f, "rag.bundle:"))
			}
		}
		if bundleKey == "" {
			bundleKey = strings.TrimSpace(space.RAGProfileKey)
		}
	}
	if sceneKey == "" {
		sceneKey = "sop"
	}
	if bundleKey == "" {
		bundleKey = "p1_general"
	}
	return
}

func InferSceneAndBundleForSpace(space *models.KnowledgeSpace) (sceneKey string, bundleKey string) {
	return inferSceneAndBundle(space)
}

type ValidateStrategyInput struct {
	SceneKey  string
	BundleKey string
}

func (s *Service) ValidateStrategy(ctx context.Context, in ValidateStrategyInput) (*strategy_catalog.ValidationResult, error) {
	if s == nil || s.strategyCatalog == nil {
		return nil, dto.NewError(http.StatusInternalServerError, "strategy catalog unavailable", nil)
	}
	cat, err := s.strategyCatalog.Load()
	if err != nil {
		return nil, dto.NewError(http.StatusInternalServerError, "加载策略目录失败", err)
	}

	sceneKey := strings.TrimSpace(in.SceneKey)
	bundleKey := strings.TrimSpace(in.BundleKey)
	if sceneKey == "" {
		sceneKey = "sop"
	}
	scene, ok := cat.Scenes[sceneKey]
	if !ok {
		return nil, dto.NewBadRequest("未知场景", nil)
	}
	if bundleKey == "" {
		bundleKey = scene.DefaultBundle
	}
	bundle, ok := cat.Bundles[bundleKey]
	if !ok {
		// 兼容早期 space.rag_profile_key=default 的写法：将其映射为该场景的默认策略包。
		// 否则会在激活/校验阶段报“未知策略包”，导致历史空间与测试用例无法跑通。
		if strings.EqualFold(bundleKey, "default") {
			bundleKey = scene.DefaultBundle
			bundle, ok = cat.Bundles[bundleKey]
		}
	}
	if !ok {
		return nil, dto.NewBadRequest("未知策略包", nil)
	}
	if len(scene.AllowedBundles) > 0 {
		allowed := false
		for _, k := range scene.AllowedBundles {
			if k == bundleKey {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, dto.NewBadRequest("该场景不允许使用该策略包", nil)
		}
	}

	required := make([]string, 0, len(scene.Prerequisites.Index)+len(bundle.Prerequisites))
	required = append(required, scene.Prerequisites.Index...)
	required = append(required, bundle.Prerequisites...)

	caps := computeStrategyCapabilities()
	missing := computeMissingPrereqs(required, caps)

	res := &strategy_catalog.ValidationResult{
		OK:              len(missing) == 0,
		SceneKey:        sceneKey,
		BundleKey:       bundleKey,
		EnabledChannels: computeEnabledChannels(required),
		Missing:         missing,
		Capabilities:    caps,
		CheckedAt:       time.Now(),
	}
	return res, nil
}

func (s *Service) EnforceStrategyPrereqsOnActivate(sceneKey, bundleKey string) error {
	res, err := s.ValidateStrategy(context.Background(), ValidateStrategyInput{SceneKey: sceneKey, BundleKey: bundleKey})
	if err != nil {
		return err
	}
	if res == nil || res.OK {
		return nil
	}
	first := "strategy_prereq_failed"
	if len(res.Missing) > 0 && strings.TrimSpace(res.Missing[0].Code) != "" {
		first = res.Missing[0].Code
	}
	return &dto.AppError{
		HTTPCode: http.StatusBadRequest,
		Message:  "策略依赖未满足，无法激活",
		Code:     first,
		Err:      ErrStrategyPrereqFailed,
		Details: map[string]interface{}{
			"sceneKey":        res.SceneKey,
			"bundleKey":       res.BundleKey,
			"enabledChannels": res.EnabledChannels,
			"missing":         res.Missing,
			"capabilities":    res.Capabilities,
		},
	}
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func computeStrategyCapabilities() map[string]bool {
	caps := map[string]bool{}

	caps["index.dense"] = envBool("PX_KNOWLEDGE_INDEX_DENSE", true)
	caps["index.sparse"] = envBool("PX_KNOWLEDGE_INDEX_SPARSE", true)
	caps["index.hier"] = envBool("PX_KNOWLEDGE_INDEX_HIER", true)
	caps["index.kg"] = envBool("PX_KNOWLEDGE_INDEX_KG", false)
	caps["index.time_fields"] = envBool("PX_KNOWLEDGE_FIELDS_TIME", true)
	caps["index.structured_fields"] = envBool("PX_KNOWLEDGE_FIELDS_STRUCTURED", true)

	ai := agentcfg.GetGlobalAIConfig()
	llmOk := false
	if ai != nil {
		provider := strings.TrimSpace(ai.Defaults.LLM.Provider)
		apiKey := strings.TrimSpace(ai.Defaults.LLM.APIKey)
		endpoint := strings.TrimSpace(ai.Defaults.LLM.Endpoint)
		model := strings.TrimSpace(ai.Defaults.LLM.Model)
		llmOk = provider != "" && model != "" && (apiKey != "" || endpoint != "")
	}
	caps["runtime.llm"] = llmOk
	caps["runtime.evidence_checker"] = envBool("PX_RUNTIME_EVIDENCE_CHECKER", llmOk)
	caps["runtime.rerank"] = envBool("PX_RUNTIME_RERANK", true)

	return caps
}

func computeEnabledChannels(required []string) []string {
	seen := map[string]struct{}{}
	order := []string{"dense", "sparse", "hier", "kg", "time", "structured"}
	for _, k := range required {
		switch k {
		case "index.dense":
			seen["dense"] = struct{}{}
		case "index.sparse":
			seen["sparse"] = struct{}{}
		case "index.hier":
			seen["hier"] = struct{}{}
		case "index.kg":
			seen["kg"] = struct{}{}
		case "index.time_fields":
			seen["time"] = struct{}{}
		case "index.structured_fields":
			seen["structured"] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for _, k := range order {
		if _, ok := seen[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func computeMissingPrereqs(required []string, caps map[string]bool) []strategy_catalog.MissingPrereq {
	missing := make([]strategy_catalog.MissingPrereq, 0)
	seen := map[string]struct{}{}
	for _, k := range required {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		if ok := caps[k]; ok {
			continue
		}
		code, msg, remediation := prereqToError(k)
		missing = append(missing, strategy_catalog.MissingPrereq{
			Code:        code,
			Key:         k,
			Message:     msg,
			Remediation: remediation,
		})
	}
	return missing
}

func prereqToError(key string) (code string, msg string, remediation []string) {
	switch key {
	case "index.kg":
		return "kg_required", "当前策略包需要 KG（知识图谱）索引能力", []string{
			"启用 KG 索引：设置 PX_KNOWLEDGE_INDEX_KG=1 并完成相关迁移/索引构建",
			"或切换到不依赖 KG 的策略包（例如 P1/P2）",
		}
	case "index.sparse":
		return "sparse_required", "当前策略包需要 Sparse(BM25) 召回通道", []string{
			"启用 Sparse：设置 PX_KNOWLEDGE_INDEX_SPARSE=1 并完成倒排索引构建",
			"或切换到仅 dense 的策略包（P0）",
		}
	case "index.hier":
		return "hier_required", "当前场景需要层次索引（Hier）支持", []string{
			"启用 Hier：设置 PX_KNOWLEDGE_INDEX_HIER=1 并在入库时生成 section 摘要",
			"或切换到不依赖 Hier 的场景/策略包",
		}
	case "index.time_fields":
		return "time_fields_required", "当前策略包需要时间字段（time_fields）支持", []string{
			"在入库 processor 中抽取并写入时间字段（如生效/签署/更新时间）",
			"执行 Corpus Check，确认 time 字段覆盖率与解析质量",
		}
	case "index.structured_fields":
		return "structured_fields_required", "当前场景需要结构化字段（structured_fields）支持", []string{
			"启用表格/结构化抽取 processor（或安装数据处理插件）",
			"执行 Corpus Check，确认结构化字段覆盖率与字段映射",
		}
	case "runtime.evidence_checker":
		return "evidence_checker_required", "当前策略包需要证据校验（evidence checker）能力", []string{
			"到「AI 设置」配置可用的 Provider/账号（LLM）后重试",
			"或切换到不依赖证据校验的策略包（P1/P0）",
		}
	default:
		return "prereq_required", "缺少前置依赖：" + key, []string{"补齐依赖后重试"}
	}
}
