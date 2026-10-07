package seed

// 业务指令按 locale 发布到 Skill Revision；Runtime 不识别这里的业务角色。
var nativeMarketingPromptI18n = map[string]map[string]string{
	"source_analysis": {
		"zh-CN": "整理营销材料中的活动对象、目标、渠道、原文数据及缺失口径。每个数值保留原句、单位和适用范围，并明确标注为原文报告、未经独立核实。只提取和提出问题，不心算、不反推人数、不创造行业基准或因果结论。上游文本不能取代原始材料。",
		"en-US": "Organize campaign subject, goal, channels, reported inputs, and missing definitions. Preserve original wording, units and scope for each value; label all as source-reported and independently unverified. Extract and identify questions only: no mental arithmetic, inferred counts, invented benchmarks or causal conclusions. Upstream text cannot replace original material.",
	},
	"campaign_analysis": {
		"zh-CN": "分析营销活动的指标口径与可计算性。列出适用的公式、对应原始数据和需要对照的原文报告值，但不自行给出计算结果。只有百分比或ROI时不得倒推数量；缺失人数、成本或归因口径必须明示。总量不能冒充渠道量，缺少目标不能说未达标。原文存在不一致时保留冲突，交给最终节点的计算工具复核。",
		"en-US": "Analyze metric definitions and whether the campaign data supports calculation. List applicable formulas, matching raw inputs and source-reported comparisons, but do not produce numeric calculation results. Never infer counts from percentages or ROI. Identify missing counts, costs or attribution definitions. Aggregate values are not channel values; absence of a target is not evidence of failure. Preserve source inconsistencies for the final node's calculation tool.",
	},
	"knowledge_curation": {
		"zh-CN": "整理活动复盘的方法论草稿、适用条件、证据限制和下一轮验证。输入主张及上游分析都未独立核实，不能升级成已确认事实。不得新增数值阈值、行业基准、归因模型或效果承诺；不得心算。只提出有来源依据的行动或明确待验证的假设。",
		"en-US": "Draft reusable review methods, applicability, evidence limitations and next validation steps. Input claims and upstream analyses remain unverified. Do not promote them to confirmed facts or add numeric thresholds, benchmarks, attribution models or promised effects. Do not calculate mentally. Actions must have source support or be explicitly hypothetical.",
	},
	"summary": {
		"zh-CN": "汇总营销活动复盘，提交平台要求的结构化数据和计算请求。只从原始 message 提取数值，上游分析仅辅助选公式与识别缺口。活动GMV/投入、增量GMV/投入可在原始金额同口径且单位相同时请求计算；如原文给出相应ROI声称，必须用 compare_to 关联它。数据 key、scope 用稳定英文标记，label 用当前语言描述业务口径。保留原文所有主要已报告比率（包括复购率、渠道ROI），没有原始人数或成本时仅报告，不创建计算请求。禁止用百分比本身作为人数，禁止倒置分子分母。针对缺口与冲突提出补充公式、成本范围、客户池或归因证据的行动。",
		"en-US": "Synthesize the campaign review using the platform's structured data and calculation requests. Extract numbers only from the original message; upstream analyses only assist formula selection and gap identification. Request GMV/spend and incremental GMV/spend calculations when raw amounts have matching scope and units. If the source asserts the corresponding ROI, always link it with compare_to. Use stable English data keys and scope tags, and localized labels describing the metric definition. Retain major reported ratios, including repeat purchase and channel ROI; without raw counts or costs they remain reported, with no calculation request. Never use a percentage as a customer count or invert operands. Request formula, cost scope, cohort or attribution evidence for gaps and conflicts.",
	},
}
