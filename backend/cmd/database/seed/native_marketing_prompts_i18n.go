package seed

// 业务指令按 locale 发布到 Skill Revision；Runtime 不识别这里的业务角色。
var nativeMarketingPromptI18n = map[string]map[string]string{
	"source_analysis": {
		"zh-CN": "整理营销材料中的活动对象、目标、渠道、原文数据及缺失口径。每个数值保留原句、单位和适用范围，并标注为原文报告、未经独立核实。保留老客激活等业务目标和原文关于渠道、人群匹配的解释，后者明确为待验证主张。不能把已出现的金额或比率说成缺失；不能因为渠道比率不同就称其矛盾。只提取和提出问题，不心算、不反推人数、不创造行业基准或因果结论。上游文本不能取代原始材料。",
		"en-US": "Organize campaign subject, goal, channels, reported inputs, and missing definitions. Preserve wording, units and scope; all values remain source-reported and unverified. Retain goals such as existing-customer activation and source explanations of channel/audience fit as unverified claims. Never call supplied amounts or rates missing; different channel ratios alone are not contradictions. No mental arithmetic, inferred counts, invented benchmarks or causal conclusions. Upstream text cannot replace the source.",
	},
	"campaign_analysis": {
		"zh-CN": "分析营销活动指标的口径与可计算性。逐项列出原文已有操作数、适用公式和待对照报告值，不自行计算。缺少原始人数只限制复算，不抹去已有比率。金额齐全时可交计算工具执行声明公式，归因、收益或成本范围未核实属于口径核实事项，不能说金额缺失。总量不能冒充渠道量，渠道比率差异本身不是矛盾；没有基准不能判断高低或未达标。保留同公式同口径下的疑似不一致，交最终计算工具复核；同时整理原文人群匹配解释及验证需要。",
		"en-US": "Assess metric definitions and calculability. List the supplied raw operands, applicable formulas and reported comparisons without calculating. Missing raw counts limit recalculation, not preservation of reported rates. Supplied amounts permit declared tool calculations; unverified attribution, return or cost definitions are definition checks, not missing amounts. Aggregates are not channel values, different channel ratios are not contradictions, and absent benchmarks cannot imply failure. Submit suspected same-formula mismatches to the final tool and preserve source audience-fit explanations with validation needs.",
	},
	"knowledge_curation": {
		"zh-CN": "整理活动复盘的方法论草稿、适用条件、证据限制和下一轮验证。以原文复核上游判断，纠正把已有数据说成缺失、把渠道差异说成矛盾的意见。保留活动目标、人群匹配解释和可迁移的方法论，但把未核实的因果、盈利和高复购判断明确列为待验证。不得新增阈值、基准、归因模型或效果承诺，不得心算。合并重复补数要求，每项行动说明要解决的未决口径或验证问题。",
		"en-US": "Draft review methods, applicability, evidence limitations and validation steps. Check upstream claims against the source and correct claims of missing supplied data or contradictions between different channels. Retain campaign goals, audience-fit explanations and reusable methods, explicitly treating causality, profitability and high repurchase claims as unverified. No invented thresholds, benchmarks, attribution models, promises or mental arithmetic. Combine duplicate data requests and tie each action to an unresolved definition or validation question.",
	},
	"summary": {
		"zh-CN": "综合营销活动复盘的原文和上游意见。数值及计算请求由已声明策略和工具负责，说明只解释业务含义。hypotheses 必须保留原文已有的活动目标（如老客激活而非拉新）、渠道和人群匹配解释（如围观流量、低转化），标明原文陈述或待验证判断。已有金额和比率不能说成缺失；产投比不能证明盈利，高复购需基准和客户池，渠道ROI不同本身不是矛盾。已执行且对照一致的算术无需再次要求用户核对。对复算冲突优先核对成本分母、收益定义、归因和观察周期，不能直接判定原文错误；渠道原始计数缺失只限制验证。actions 按未解决问题给出具体验证对象，合并重复复购补数和泛化核对建议。数值只引用表格指标名称，禁止心算、反推或重复输出数字。",
		"en-US": "Synthesize the original campaign material and upstream opinions. Declared policy and tools own numbers and calculation requests; notes explain business meaning. Hypotheses must retain source campaign goals (such as existing-customer activation rather than acquisition), channel and audience-fit explanations (such as spectator traffic and low conversion), explicitly identified as source claims or unverified interpretations. Supplied amounts and rates are not missing. Return multiples do not prove profitability; high repurchase needs a cohort and baseline; different channel ROI values are not contradictions. Do not ask users to recheck matched tool arithmetic. For mismatches, prioritize cost denominator, return, attribution and observation-window definitions without declaring the source wrong. Missing channel counts limit verification only. Actions must address specific unresolved questions and combine duplicate cohort requests. Refer to metric labels, never output or infer numbers.",
	},
}
