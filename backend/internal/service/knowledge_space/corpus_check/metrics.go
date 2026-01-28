package corpus_check

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gorm.io/datatypes"

	strategy_catalog "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/strategy_catalog"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

const defaultSceneStrategyCatalogPath = "backend/config/knowledge/scene_strategy_catalog.yaml"

// BuildMetrics 根据 ingestion_jobs 的 metrics_snapshot 生成最小体检指标与推荐卡片。
//
// T110：必须输出“推荐场景 + 推荐策略包 + 推荐理由 + 成本/风险提示”，并确保推荐只落在策略包映射的场景集合里。
func BuildMetrics(sampleJobs []models.IngestionJob) (metrics datatypes.JSON, recommendations datatypes.JSON) {
	type ocrStats struct {
		Required int `json:"required"`
		Needed   int `json:"needed"`
		Used     int `json:"used"`
	}
	type dist map[string]int

	formatDist := make(dist)
	sourceDist := make(dist)
	langDist := make(dist)
	ocr := ocrStats{}
	total := 0
	dupKey := make(map[string]int)
	dups := 0

	for _, job := range sampleJobs {
		total++
		format := strings.ToLower(strings.TrimSpace(job.SourceType))
		if format == "" {
			format = "unknown"
		}
		formatDist[format]++
		sourceDist[formatCategory(format)]++
		dupKey[job.SourceID]++
		if dupKey[job.SourceID] > 1 {
			dups++
		}

		var snap map[string]any
		_ = json.Unmarshal(job.MetricsSnapshot, &snap)
		if b, ok := snap["ocr_required"].(bool); ok && b {
			ocr.Required++
		}
		if b, ok := snap["ocr_needed"].(bool); ok && b {
			ocr.Needed++
		}
		if b, ok := snap["ocr_used"].(bool); ok && b {
			ocr.Used++
		}
		if lang, ok := snap["language"].(string); ok {
			lang = strings.ToLower(strings.TrimSpace(lang))
			if lang == "" {
				lang = "unknown"
			}
			langDist[lang]++
		}
	}

	formatRatio := make(map[string]float64, len(formatDist))
	typeRatio := make(map[string]float64, len(sourceDist))
	langRatio := make(map[string]float64, len(langDist))
	if total > 0 {
		for k, v := range formatDist {
			formatRatio[k] = float64(v) / float64(total)
		}
		for k, v := range sourceDist {
			typeRatio[k] = float64(v) / float64(total)
		}
		for k, v := range langDist {
			langRatio[k] = float64(v) / float64(total)
		}
		if len(langDist) == 0 {
			langRatio["unknown"] = 1.0
		}
	}

	ocrNeededRatio := 0.0
	tableLikeRatio := 0.0
	codeLikeRatio := 0.0
	duplicateRatio := 0.0
	pdfRatio := 0.0
	if total > 0 {
		ocrNeededRatio = float64(ocr.Needed) / float64(total)
		tableLikeRatio = float64(sourceDist["table_like"]) / float64(total)
		codeLikeRatio = float64(sourceDist["code_like"]) / float64(total)
		duplicateRatio = float64(dups) / float64(total)
		pdfRatio = formatRatio["pdf"]
	}

	metricsMap := map[string]any{
		"sample_total": total,
		"format_dist":  formatDist,
		"format_ratio": formatRatio,
		"type_dist":    sourceDist,
		"type_ratio":   typeRatio,
		"language_dist": func() dist {
			if len(langDist) == 0 {
				return dist{"unknown": total}
			}
			return langDist
		}(),
		"language_ratio": langRatio,
		"ocr":            ocr,
		"ratios": map[string]any{
			"ocr_needed": ocrNeededRatio,
			"table_like": tableLikeRatio,
			"code_like":  codeLikeRatio,
			"duplicate":  duplicateRatio,
			"pdf":        pdfRatio,
		},
		"duplicate": map[string]any{
			"count": dups,
			"ratio": duplicateRatio,
		},
	}

	catalog := loadCatalog()
	sceneKey, bundleKey, reason, risk, cost := recommendSceneBundle(total, ocrNeededRatio, tableLikeRatio, codeLikeRatio, pdfRatio)
	sceneKey, bundleKey, sceneLabel, bundleLabel := constrainToCatalog(catalog, sceneKey, bundleKey)

	pkgKey, pkgReason, pkgRisk, pkgCost := recommendStrategyPackage(total, ocrNeededRatio, tableLikeRatio, codeLikeRatio, pdfRatio)
	pkgKey, pkgLabel, pkgScenes, pkgProfile := constrainStrategyPackageToCatalog(catalog, pkgKey)
	primaryScene := sceneKey
	if len(pkgScenes) > 0 && !containsString(pkgScenes, primaryScene) {
		primaryScene = pkgScenes[0]
	}
	primarySceneLabel := primaryScene
	if catalog != nil {
		if sc, ok := catalog.Scenes[primaryScene]; ok && strings.TrimSpace(sc.Label) != "" {
			primarySceneLabel = sc.Label
		}
	}

	recs := make([]map[string]any, 0, 6)
	recs = append(recs, map[string]any{
		"key":         "scene_bundle",
		"type":        "scene_bundle",
		"title":       fmt.Sprintf("推荐：%s × %s", sceneLabel, bundleLabel),
		"sceneKey":    sceneKey,
		"sceneLabel":  sceneLabel,
		"bundleKey":   bundleKey,
		"bundleLabel": bundleLabel,
		"reason":      reason,
		"risk":        risk,
		"cost":        cost,
	})
	recs = append(recs, map[string]any{
		"key":                 "strategy_package",
		"type":                "strategy_package",
		"title":               fmt.Sprintf("推荐策略包：%s", pkgLabel),
		"strategyPackageKey":  pkgKey,
		"strategyPackageLabel": pkgLabel,
		"profileKey":          pkgProfile,
		"sceneKey":            primaryScene,
		"sceneLabel":          primarySceneLabel,
		"scenes":              pkgScenes,
		"reason":              pkgReason,
		"risk":                pkgRisk,
		"cost":                pkgCost,
	})

	if total > 0 && ocrNeededRatio >= 0.3 {
		recs = append(recs, map[string]any{
			"key":   "enable_ocr",
			"title": "扫描件占比偏高：建议启用 OCR",
			"reason": map[string]any{
				"ocr_needed_ratio": ocrNeededRatio,
			},
			"plugin": "com.powerx.plugin.data_forge",
			"risk":   "若不启用 OCR，检索召回与引用覆盖会显著下降。",
			"cost":   "OCR 会提升入库耗时与成本；可先对高价值文档启用。",
		})
	}
	if total > 0 && tableLikeRatio >= 0.3 {
		recs = append(recs, map[string]any{
			"key":   "table_heavy",
			"title": "表格/结构化内容占比偏高：建议调整分块与索引策略",
			"reason": map[string]any{
				"table_like_ratio": tableLikeRatio,
			},
			"risk": "不做表格感知分块可能导致问答命中不稳定。",
			"cost": "改用更细粒度 chunk 会增大向量写入量。",
		})
	}
	if total > 0 && codeLikeRatio >= 0.2 {
		recs = append(recs, map[string]any{
			"key":   "code_heavy",
			"title": "代码/SQL 内容占比偏高：建议启用 code-aware chunking 与更高阈值检索",
			"reason": map[string]any{
				"code_like_ratio": codeLikeRatio,
			},
			"risk": "代码块过长会导致向量语义漂移，低阈值 top_k 容易引入噪声。",
			"cost": "更细粒度切分会增加向量写入量；可通过索引分层缓解。",
		})
	}
	if total > 0 && langDist["zh"] > 0 && float64(langDist["zh"])/float64(total) >= 0.5 {
		recs = append(recs, map[string]any{
			"key":   "zh_corpus",
			"title": "中文语料占比偏高：建议选用中文优化 embedding/rerank 配置",
			"reason": map[string]any{
				"zh_ratio": float64(langDist["zh"]) / float64(total),
			},
			"risk": "若使用非中文优化模型，可能出现召回下降与引用不稳定。",
			"cost": "切换模型可能影响成本与延迟；建议先在 Playground 做 A/B 对比。",
		})
	}
	if total == 0 {
		recs = append(recs, map[string]any{
			"key":   "default",
			"title": "暂无入库样本：建议先导入少量代表性文档后再运行体检",
			"risk":  "未进行体检前，默认按通用策略运行。",
		})
	}

	metricsBytes, _ := json.Marshal(metricsMap)
	recsBytes, _ := json.Marshal(recs)
	return datatypes.JSON(metricsBytes), datatypes.JSON(recsBytes)
}

func recommendSceneBundle(total int, ocrNeededRatio, tableLikeRatio, codeLikeRatio, pdfRatio float64) (sceneKey string, bundleKey string, reason map[string]any, risk string, cost string) {
	sceneKey = "sop"
	bundleKey = "p1_general"
	reason = map[string]any{
		"signals": map[string]any{
			"sample_total":     total,
			"ocr_needed_ratio": ocrNeededRatio,
			"table_like_ratio": tableLikeRatio,
			"code_like_ratio":  codeLikeRatio,
			"pdf_ratio":        pdfRatio,
		},
		"summary": "默认推荐：SOP/制度 × P1 通用（企业默认）",
	}
	risk = "建议在 Playground 做一次 A/B 检索对比，确认召回与引用覆盖。"
	cost = "P1 默认开 hybrid + rerank（轻量），成本/延迟适中。"

	if total > 0 && tableLikeRatio >= 0.3 {
		sceneKey = "ledger_table"
		bundleKey = "p2_high_accuracy"
		reason["summary"] = "表格/结构化占比偏高：偏向台账场景，并建议证据优先策略包。"
		risk = "结构化字段抽取不足会导致过滤/命中不稳定。"
		cost = "行级/字段级切分会增加索引与存储成本。"
		return
	}
	if total > 0 && codeLikeRatio >= 0.2 {
		sceneKey = "sql_kg"
		bundleKey = "p3_kg_strong"
		reason["summary"] = "代码/SQL 占比偏高：偏向依赖关系查询，建议 KG 约束策略包。"
		risk = "若缺少 KG/依赖抽取，容易出现‘看似相关但不可执行’的回答。"
		cost = "KG 构建与维护有额外成本；建议先小范围试点。"
		return
	}
	if total > 0 && ocrNeededRatio >= 0.3 {
		sceneKey = "contract_quote"
		bundleKey = "p2_high_accuracy"
		reason["summary"] = "扫描件/图片占比偏高：偏向合同/报价类证据查找，建议证据优先策略包。"
		risk = "未启用 OCR/证据链会导致引用覆盖下降与合规风险。"
		cost = "OCR + 证据校验会提升入库与推理成本。"
		return
	}
	if total > 0 && pdfRatio >= 0.6 {
		sceneKey = "research_longdoc"
		bundleKey = "p1_general"
		reason["summary"] = "PDF 长文占比偏高：偏向论文/长报告，建议层次索引 + 通用策略包。"
		risk = "若切分过粗，长文回答可能遗漏关键段落。"
		cost = "层次索引需要额外摘要/结构化产物。"
		return
	}
	return
}

func recommendStrategyPackage(total int, ocrNeededRatio, tableLikeRatio, codeLikeRatio, pdfRatio float64) (pkgKey string, reason map[string]any, risk string, cost string) {
	pkgKey = "H_fusion"
	reason = map[string]any{
		"signals": map[string]any{
			"sample_total":     total,
			"ocr_needed_ratio": ocrNeededRatio,
			"table_like_ratio": tableLikeRatio,
			"code_like_ratio":  codeLikeRatio,
			"pdf_ratio":        pdfRatio,
		},
		"summary": "默认推荐：融合检索（H），平衡成本与命中率。",
	}
	risk = "建议在 Playground 做一次 A/B 检索对比，确认召回与引用覆盖。"
	cost = "融合检索需要同时维护 dense + sparse 索引。"

	if total > 0 && codeLikeRatio >= 0.2 {
		pkgKey = "K_kg"
		reason["summary"] = "代码/SQL 占比偏高：优先推荐知识图谱（K）。"
		risk = "若缺少实体/关系抽取与 KG 索引，回答会缺少依赖链路。"
		cost = "KG 构建与维护有额外成本；建议先小范围试点。"
		return
	}
	if total > 0 && tableLikeRatio >= 0.3 {
		pkgKey = "D_doc_augmentation"
		reason["summary"] = "表格/结构化占比偏高：优先推荐文档增强（D）。"
		risk = "字段抽取不足会导致过滤/命中不稳定。"
		cost = "离线增强会增加入库耗时与存储成本。"
		return
	}
	if total > 0 && ocrNeededRatio >= 0.3 {
		pkgKey = "O_crag"
		reason["summary"] = "扫描件/图片占比偏高：优先推荐纠错（O）。"
		risk = "未启用 OCR/证据链会导致引用覆盖下降与合规风险。"
		cost = "证据校验会提升检索与推理成本。"
		return
	}
	if total > 0 && pdfRatio >= 0.6 {
		pkgKey = "B_semantic_chunking"
		reason["summary"] = "PDF 长文占比偏高：优先推荐语义切块（B）。"
		risk = "若切分过粗，长文回答可能遗漏关键段落。"
		cost = "语义边界检测会增加入库耗时。"
		return
	}
	return
}

func loadCatalog() *strategy_catalog.Catalog {
	path := strings.TrimSpace(os.Getenv("PX_SCENE_STRATEGY_CATALOG_PATH"))
	if path == "" {
		path = defaultSceneStrategyCatalogPath
	}
	loader := strategy_catalog.NewLoader(path)
	cat, err := loader.Load()
	if err != nil {
		return nil
	}
	return cat
}

func constrainToCatalog(cat *strategy_catalog.Catalog, sceneKey, bundleKey string) (outSceneKey, outBundleKey, sceneLabel, bundleLabel string) {
	outSceneKey = strings.TrimSpace(sceneKey)
	outBundleKey = strings.TrimSpace(bundleKey)
	if outSceneKey == "" {
		outSceneKey = "sop"
	}
	if outBundleKey == "" {
		outBundleKey = "p1_general"
	}
	if cat == nil {
		return outSceneKey, outBundleKey, outSceneKey, outBundleKey
	}

	sc, ok := cat.Scenes[outSceneKey]
	if !ok {
		outSceneKey = "sop"
		sc = cat.Scenes[outSceneKey]
	}

	// bundle 必须存在
	if _, ok := cat.Bundles[outBundleKey]; !ok {
		outBundleKey = sc.DefaultBundle
	}

	// bundle 必须在 allowed 内
	if len(sc.AllowedBundles) > 0 {
		allowed := false
		for _, k := range sc.AllowedBundles {
			if k == outBundleKey {
				allowed = true
				break
			}
		}
		if !allowed {
			if sc.DefaultBundle != "" {
				outBundleKey = sc.DefaultBundle
			} else {
				outBundleKey = sc.AllowedBundles[0]
			}
		}
	}

	sceneLabel = sc.Label
	if sceneLabel == "" {
		sceneLabel = outSceneKey
	}
	bundleLabel = outBundleKey
	if b, ok := cat.Bundles[outBundleKey]; ok {
		if strings.TrimSpace(b.Label) != "" {
			bundleLabel = b.Label
		}
	}
	return
}

func constrainStrategyPackageToCatalog(cat *strategy_catalog.Catalog, pkgKey string) (outKey, label string, scenes []string, profileKey string) {
	outKey = strings.TrimSpace(pkgKey)
	if outKey == "" {
		outKey = "H_fusion"
	}
	if cat == nil {
		return outKey, outKey, nil, ""
	}
	if _, ok := cat.StrategyPackages[outKey]; !ok {
		outKey = "H_fusion"
	}
	pkg, ok := cat.StrategyPackages[outKey]
	if !ok {
		return outKey, outKey, nil, ""
	}
	label = pkg.Label
	if strings.TrimSpace(label) == "" {
		label = outKey
	}
	profileKey = strings.TrimSpace(pkg.RecommendedProfileKey)
	if profileKey == "" {
		profileKey = "p1_general"
	}
	scenes = filterSceneKeys(cat, pkg.RecommendedScenes)
	return outKey, label, scenes, profileKey
}

func filterSceneKeys(cat *strategy_catalog.Catalog, scenes []string) []string {
	out := make([]string, 0, len(scenes))
	if cat == nil {
		return out
	}
	for _, k := range scenes {
		if _, ok := cat.Scenes[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func formatCategory(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "xlsx", "csv", "table":
		return "table_like"
	case "sql":
		return "code_like"
	case "image":
		return "image"
	case "pdf", "docx", "markdown", "html":
		return "doc"
	default:
		return "other"
	}
}
