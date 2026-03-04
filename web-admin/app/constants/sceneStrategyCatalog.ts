export type SceneKey =
  | "product_specs"
  | "product_compat"
  | "product_selection"
  | "sop"
  | "contract_quote"
  | "research_longdoc"
  | "ledger_table"
  | "support_faq"
  | "support_policy"
  | "support_troubleshooting"
  | "eng_runbook"
  | "eng_incident"
  | "eng_change"
  | "api_reference"
  | "data_dictionary"
  | "sales_enablement"
  | "marketing_promo_rules"
  | "compliance_regulation"
  | "billing_pricing"
  | "meeting_minutes"
  | "project_docs"
  | "ticket_conversations"
  | "onboarding_training"
  | "sql_kg"
  | "custom_expert";

export type StrategyBundleKey = "p0_basic" | "p1_general" | "p2_high_accuracy" | "p3_kg_strong";

// RAG 模块（来自 docs/plan/AI_engineering/knowledge/rag.md 的策略段落；用于 L3 选择与映射）。
// 说明：这里的 key 代表“模块能力”，不是最终的“RAG Profile 版本号”。
export type RagModuleKey =
  | "A_simple"
  | "A1_routing"
  | "A2_time_aware"
  | "B_semantic_chunking"
  | "C_context_enriched"
  | "D_doc_augmentation"
  | "E_query_transform"
  | "F_rerank"
  | "H_fusion"
  | "J_hier"
  | "K_kg"
  | "L_feedback"
  | "O_crag";

export type IndexPrereqKey =
  | "index.dense"
  | "index.sparse"
  | "index.hier"
  | "index.kg"
  | "index.time_fields"
  | "index.structured_fields";

export type SceneStrategyCatalog = {
  ragModules: Record<
    RagModuleKey,
    {
      label: string;
      desc: string;
      guide?: string;
    }
  >;
  bundles: Record<
    StrategyBundleKey,
    {
      label: string;
      description: string;
      prerequisites: string[];
      defaultModules?: RagModuleKey[];
      guide?: string;
    }
  >;
  scenes: Record<
    SceneKey,
    {
      category: string;
      label: string;
      description: string;
      guide?: string;
      keywords?: string[];
      defaultBundle: StrategyBundleKey;
      allowedBundles: StrategyBundleKey[];
      defaultModules?: RagModuleKey[];
      rag?: {
        // L3：主策略（单选）+ 模块组合（最终生效以 modules 计算为准）
        defaultPrimary: RagModuleKey;
        allowedPrimary: RagModuleKey[];
      };
      prerequisites: {
        index: IndexPrereqKey[];
        assets: string[];
      };
      ingestionDefaults?: {
        chunking?: {
          mode: string;
          unit: string;
          chunkSize: number;
          overlap: number;
          separators: string[];
        };
      };
    }
  >;
};

// 前端临时 catalog（用于 T108：场景→策略包两层选择）。
// SSOT 在后端：backend/config/knowledge/scene_strategy_catalog.yaml（T109 会补一个 API 下发，届时替换掉这里）。
export const SCENE_STRATEGY_CATALOG: SceneStrategyCatalog = {
  ragModules: {
    A_simple: { label: "A Simple（最小闭环）", desc: "简单切块 + 向量召回（适合最小闭环验证）。" },
    A1_routing: { label: "A1 Routing（查询路由）", desc: "查询路由：按意图/领域把 query 路由到不同索引通道/空间。" },
    A2_time_aware: { label: "A2 Time-aware（时间/版本）", desc: "时间感知：版本/生效时间权重与冲突策略（报价/政策/合规常见）。" },
    B_semantic_chunking: { label: "B Semantic Chunking（语义切块）", desc: "语义切块：更适合论文/长报告/长方案。" },
    C_context_enriched: { label: "C Context Enriched（上下文增强）", desc: "上下文增强：召回后扩展同章节邻居/父摘要。" },
    D_doc_augmentation: { label: "D Doc Augmentation（离线增强）", desc: "离线增强：关键词/摘要/字段/实体等增强字段。" },
    E_query_transform: { label: "E Query Transform（查询转换）", desc: "查询转换：同义扩展、结构化抽取、纠错等。" },
    F_rerank: { label: "F Rerank（重排序）", desc: "重排序：降低相似候选的误命中。" },
    H_fusion: { label: "H Fusion（融合检索）", desc: "融合检索：dense+sparse（可选 hier/kg）多路召回融合。" },
    J_hier: { label: "J Hier（层次索引）", desc: "层次索引：先 doc/section 后 chunk 的多粒度召回。" },
    K_kg: { label: "K KG（知识图谱）", desc: "知识图谱：实体/关系召回与约束，适用于依赖/约束查询。" },
    L_feedback: { label: "L Feedback（反馈闭环）", desc: "反馈闭环：标注→重处理→回归评估。" },
    O_crag: { label: "O CRAG（证据纠错）", desc: "纠错：证据不足/不一致时触发更严检索与护栏。" },
  },
  bundles: {
    p0_basic: {
      label: "P0 基础（最小闭环）",
      description: "最少干预：仅 dense 召回 + 引用返回；不开 multiquery/HyDE/rerank/KG/self-check。",
      prerequisites: ["index.dense"],
      defaultModules: ["A_simple", "L_feedback"],
      guide: "PROFILE-P0_basic.md",
    },
    p1_general: {
      label: "P1 通用推荐（企业默认）",
      description: "hybrid + RRF + 轻量 rerank + contextual（按场景启用 hier）。",
      prerequisites: ["index.dense", "index.sparse"],
      defaultModules: ["H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      guide: "PROFILE-P1_general_recommended.md",
    },
    p2_high_accuracy: {
      label: "P2 高准确/合规（证据优先）",
      description: "sparse 优先 + hybrid + CRAG/可选 self-check + time-aware（按场景）。",
      prerequisites: ["index.dense", "index.sparse", "runtime.evidence_checker"],
      defaultModules: ["H_fusion", "O_crag", "F_rerank", "L_feedback"],
      guide: "PROFILE-P2_high_accuracy_compliance.md",
    },
    p3_kg_strong: {
      label: "P3 KG 约束（关系驱动）",
      description: "KG recall/filter + hybrid + cite，适用于依赖/约束/关系查询。",
      prerequisites: ["index.kg", "index.sparse", "index.dense"],
      defaultModules: ["K_kg", "H_fusion", "C_context_enriched", "L_feedback"],
      guide: "PROFILE-P3_kg_constrained.md",
    },
  },
  scenes: {
    product_specs: {
      category: "产品与商品（Catalog）",
      label: "产品库 / 规格参数查询",
      description: "参数/型号/价格/口径等事实精确型查询；混合数据源常见（DB+文档）。",
      guide: "SCN-070_product_specs.md",
      keywords: ["产品", "规格", "参数", "型号", "价格", "口径", "catalog", "specs"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "O_crag", "F_rerank", "E_query_transform", "L_feedback"],
      rag: {
        defaultPrimary: "H_fusion",
        allowedPrimary: ["H_fusion", "O_crag", "E_query_transform", "J_hier"],
      },
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.structured_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_fields_card",
          unit: "chars",
          chunkSize: 700,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "。", "；", ":", "："],
        },
      },
    },
    product_compat: {
      category: "产品与商品（Catalog）",
      label: "产品库 / 兼容性与配件关系",
      description: "兼容矩阵/配件关系/约束条件；可选 KG 强化解释与过滤。",
      guide: "SCN-071_product_compat.md",
      keywords: ["兼容", "配件", "关系", "约束", "compat", "accessory"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "O_crag", "F_rerank", "C_context_enriched", "L_feedback"],
      rag: {
        defaultPrimary: "K_kg",
        allowedPrimary: ["K_kg", "H_fusion", "O_crag", "E_query_transform"],
      },
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.structured_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_compat",
          unit: "chars",
          chunkSize: 800,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "兼容", "不兼容", "支持", "不支持", "：", ":"],
        },
      },
    },
    product_selection: {
      category: "产品与商品（Catalog）",
      label: "产品库 / 选型对比与推荐理由",
      description: "方案对比/选型决策；更偏解释归纳，但仍需可追溯引用。",
      guide: "SCN-072_product_selection.md",
      keywords: ["选型", "对比", "推荐", "理由", "selection", "compare"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      rag: {
        defaultPrimary: "C_context_enriched",
        allowedPrimary: ["C_context_enriched", "H_fusion", "F_rerank", "E_query_transform", "J_hier"],
      },
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "对比", "推荐", "理由", "。", "；"],
        },
      },
    },
    sop: {
      category: "制度与手册（Docs）",
      label: "SOP/制度/产品说明",
      description: "Markdown/Word 为主，结构清晰，常见查询为流程/解释归纳。",
      guide: "SCN-010_sop_manual.md",
      keywords: ["SOP", "制度", "手册", "说明", "流程", "规范"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "F_rerank", "C_context_enriched", "J_hier", "L_feedback"],
      rag: {
        defaultPrimary: "J_hier",
        allowedPrimary: ["J_hier", "C_context_enriched", "H_fusion", "F_rerank", "E_query_transform"],
      },
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 800,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "。", "！", "？", "；"],
        },
      },
    },
    contract_quote: {
      category: "合同与合规（Compliance）",
      label: "合同/报价",
      description: "PDF/Word/Excel 表格多；事实查找+合规风险高，默认证据优先。",
      guide: "SCN-020_contract_quote.md",
      keywords: ["合同", "报价", "条款", "合规", "风险", "金额", "日期"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "A2_time_aware", "O_crag", "F_rerank", "D_doc_augmentation", "L_feedback"],
      rag: {
        defaultPrimary: "O_crag",
        allowedPrimary: ["O_crag", "A2_time_aware", "H_fusion", "F_rerank", "J_hier"],
      },
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.time_fields", "index.structured_fields"],
        assets: ["asset.augmented_fields"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_clause_table",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "条", "款", "。", "；"],
        },
      },
    },
    research_longdoc: {
      category: "研究与资料（Research）",
      label: "论文/研究/长报告",
      description: "PDF 长文；需要语义切块与层次索引，支持 hier-first 与 HyDE（可选）。",
      guide: "SCN-030_research_report.md",
      keywords: ["论文", "研究", "报告", "长文", "总结", "引用"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["B_semantic_chunking", "J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      rag: {
        defaultPrimary: "B_semantic_chunking",
        allowedPrimary: ["B_semantic_chunking", "J_hier", "H_fusion", "F_rerank", "C_context_enriched"],
      },
      prerequisites: {
        index: ["index.dense", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "semantic",
          unit: "chars",
          chunkSize: 1200,
          overlap: 200,
          separators: ["\\n\\n", "\\n", "。", "！", "？"],
        },
      },
    },
    ledger_table: {
      category: "台账与清单（Ledger）",
      label: "台账/清单（表格）",
      description: "Excel/CSV 结构化行；以精确过滤/字段查询为主。",
      guide: "SCN-040_table_ledger.md",
      keywords: ["台账", "清单", "表格", "字段", "过滤", "CSV", "Excel"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "F_rerank", "O_crag", "L_feedback"],
      rag: {
        defaultPrimary: "H_fusion",
        allowedPrimary: ["H_fusion", "O_crag", "E_query_transform"],
      },
      prerequisites: {
        index: ["index.sparse", "index.structured_fields", "index.dense"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "row",
          unit: "records",
          chunkSize: 1,
          overlap: 0,
          separators: [],
        },
      },
    },
    support_faq: {
      category: "支持与客服（Support）",
      label: "客服 / FAQ 与使用说明",
      description: "问答粒度、同义表达多；强调命中率与可解释引用。",
      guide: "SCN-080_support_faq.md",
      keywords: ["客服", "FAQ", "问答", "使用", "运营", "同义"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "E_query_transform", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "qna",
          unit: "chars",
          chunkSize: 500,
          overlap: 80,
          separators: ["\\n\\n", "\\n", "Q:", "A:", "？", "。"],
        },
      },
    },
    support_policy: {
      category: "支持与客服（Support）",
      label: "客服 / 售后政策与规则",
      description: "退款/保修/条款政策；高风险答复需要强证据与纠错。",
      guide: "SCN-081_support_policy.md",
      keywords: ["售后", "政策", "规则", "退款", "保修", "条款"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "O_crag", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "policy_clause",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "条", "款", "。", "；"],
        },
      },
    },
    support_troubleshooting: {
      category: "支持与客服（Support）",
      label: "客服 / 故障现象与排查",
      description: "现象→原因→处理步骤；需要步骤链路与上下文补齐。",
      guide: "SCN-082_support_troubleshooting.md",
      keywords: ["故障", "现象", "排查", "错误码", "处理", "步骤"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_steps",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "现象", "原因", "处理", "步骤", "。", "；"],
        },
      },
    },
    eng_runbook: {
      category: "工程与运维（Engineering）",
      label: "工程 / Runbook 与标准操作",
      description: "标准操作/手册；章节与步骤链路清晰，强调上下文。",
      guide: "SCN-090_eng_runbook.md",
      keywords: ["运维", "Runbook", "标准操作", "手册", "步骤"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_steps",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "步骤", "Step", "。", "；"],
        },
      },
    },
    eng_incident: {
      category: "工程与运维（Engineering）",
      label: "工程 / 故障排查与应急响应",
      description: "应急响应/故障处理；需要时间线、步骤链路与降级策略（可选）。",
      guide: "SCN-091_eng_incident.md",
      keywords: ["应急", "故障", "排查", "incident", "恢复", "止血"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_steps",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "现象", "原因", "处理", "步骤", "。", "；"],
        },
      },
    },
    eng_change: {
      category: "工程与运维（Engineering）",
      label: "工程 / 变更与发布",
      description: "变更评审/发布/回滚；高风险需要证据优先与版本/时间过滤。",
      guide: "SCN-092_eng_change.md",
      keywords: ["变更", "发布", "回滚", "风险", "评审", "change"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "A2_time_aware", "O_crag", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.time_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "change_log",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "变更", "发布", "回滚", "风险", "。", "；"],
        },
      },
    },
    api_reference: {
      category: "数据与接口（Data & API）",
      label: "API/接口文档 / 参数与返回",
      description: "字段/参数/返回值/示例为主；事实精确优先，适合接口问答。",
      guide: "SCN-100_api_reference.md",
      keywords: ["API", "接口", "参数", "返回", "示例", "文档"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "O_crag", "E_query_transform", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.structured_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "api_section",
          unit: "chars",
          chunkSize: 800,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "参数", "字段", "返回", "示例", "：", ":"],
        },
      },
    },
    data_dictionary: {
      category: "数据与接口（Data & API）",
      label: "数据字典 / 表结构与字段口径",
      description: "表/字段/口径/示例；精确定位字段含义，支持结构化过滤。",
      guide: "SCN-101_data_dictionary.md",
      keywords: ["数据字典", "字段", "口径", "表结构", "schema"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "O_crag", "E_query_transform", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.structured_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "schema_table",
          unit: "chars",
          chunkSize: 800,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "表", "字段", "类型", "口径", "：", ":"],
        },
      },
    },
    sales_enablement: {
      category: "销售与市场（Go-to-Market）",
      label: "销售材料 / 话术与竞品",
      description: "销售话术、竞品对比、案例材料；偏解释归纳，但仍需可追溯引用。",
      guide: "SCN-110_sales_enablement.md",
      keywords: ["销售", "话术", "竞品", "对比", "案例", "enablement"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "对比", "优势", "劣势", "。", "；"],
        },
      },
    },
    marketing_promo_rules: {
      category: "销售与市场（Go-to-Market）",
      label: "市场活动 / 促销规则",
      description: "活动规则、优惠口径、例外条款；高风险答复需要强证据。",
      guide: "SCN-120_marketing_promo_rules.md",
      keywords: ["市场活动", "促销", "规则", "优惠", "例外", "promo"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "O_crag", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "policy_clause",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "规则", "例外", "不适用", "。", "；"],
        },
      },
    },
    compliance_regulation: {
      category: "合同与合规（Compliance）",
      label: "法规/监管政策 / 口径与约束",
      description: "法规/监管/政策口径；时效强，建议启用时间字段过滤。",
      guide: "SCN-130_compliance_regulation.md",
      keywords: ["法规", "监管", "政策", "合规", "口径", "regulation"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "A2_time_aware", "O_crag", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.time_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "policy_clause",
          unit: "chars",
          chunkSize: 1100,
          overlap: 180,
          separators: ["\\n\\n", "\\n", "条", "款", "。", "；"],
        },
      },
    },
    billing_pricing: {
      category: "计费与价格（Billing）",
      label: "计费/价格规则 / 口径与例外",
      description: "计费口径、价格规则、版本差异与例外；事实精确 + 时效要求高。",
      guide: "SCN-140_billing_pricing.md",
      keywords: ["计费", "价格", "规则", "口径", "版本", "例外", "billing"],
      defaultBundle: "p2_high_accuracy",
      allowedBundles: ["p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "A2_time_aware", "O_crag", "F_rerank", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.structured_fields", "index.time_fields"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured_fields_card",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "价格", "计费", "规则", "例外", "：", ":"],
        },
      },
    },
    meeting_minutes: {
      category: "协作与项目（Collaboration）",
      label: "会议纪要 / 决议与行动项",
      description: "会议纪要、决议、行动项；偏总结归纳并要求引用定位。",
      guide: "SCN-150_meeting_minutes.md",
      keywords: ["会议纪要", "决议", "行动项", "会议", "minutes"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "minutes",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "决议", "行动项", "Owner", "Due", "。", "；"],
        },
      },
    },
    project_docs: {
      category: "协作与项目（Collaboration）",
      label: "项目方案 / 交付文档",
      description: "项目方案、需求、设计与交付资料；章节上下文与引用定位重要。",
      guide: "SCN-160_project_docs.md",
      keywords: ["项目", "方案", "需求", "设计", "交付", "project"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 1000,
          overlap: 180,
          separators: ["\\n\\n", "\\n", "。", "！", "？", "；"],
        },
      },
    },
    ticket_conversations: {
      category: "支持与客服（Support）",
      label: "工单/聊天记录 / 问题追踪",
      description: "工单对话、聊天记录、问题追踪；需要按话题/时间窗聚合与可追溯引用。",
      guide: "SCN-170_ticket_conversations.md",
      keywords: ["工单", "聊天", "对话", "问题追踪", "ticket", "conversation"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["H_fusion", "E_query_transform", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "conversation_window",
          unit: "chars",
          chunkSize: 700,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "时间", "用户", "客服", "：", ":"],
        },
      },
    },
    onboarding_training: {
      category: "培训与入职（Enablement）",
      label: "入职/培训资料 / 学习路径",
      description: "培训课件、学习资料、入职手册；强调归纳总结与引用定位。",
      guide: "SCN-180_onboarding_training.md",
      keywords: ["入职", "培训", "学习", "课程", "手册", "training"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy"],
      defaultModules: ["J_hier", "H_fusion", "F_rerank", "C_context_enriched", "L_feedback"],
      prerequisites: {
        index: ["index.dense", "index.sparse", "index.hier"],
        assets: ["asset.section_summaries"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 900,
          overlap: 150,
          separators: ["\\n\\n", "\\n", "课程", "章节", "。", "；"],
        },
      },
    },
    sql_kg: {
      category: "关系与依赖（KG & Dependency）",
      label: "SQL/配置/依赖关系（KG 强）",
      description: "关系/依赖驱动：KG 为主召回/约束通道，dense 作为摘要补充。",
      guide: "SCN-050_sql_config_kg.md",
      keywords: ["SQL", "配置", "依赖", "KG", "关系", "拓扑"],
      defaultBundle: "p3_kg_strong",
      allowedBundles: ["p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["K_kg", "H_fusion", "C_context_enriched", "L_feedback"],
      rag: {
        defaultPrimary: "K_kg",
        allowedPrimary: ["K_kg", "H_fusion", "E_query_transform", "O_crag"],
      },
      prerequisites: {
        index: ["index.kg", "index.sparse", "index.dense"],
        assets: ["asset.kg_entities", "asset.kg_relations", "asset.kg_provenance"],
      },
      ingestionDefaults: {
        chunking: {
          mode: "ast_object",
          unit: "objects",
          chunkSize: 1,
          overlap: 0,
          separators: [],
        },
      },
    },
    custom_expert: {
      category: "自定义（Expert）",
      label: "自定义（专家）",
      description: "允许选择全部策略模块与策略包，但必须进行依赖校验与成本护栏。",
      guide: "SCN-060_custom_expert.md",
      keywords: ["自定义", "专家", "高级"],
      defaultBundle: "p1_general",
      allowedBundles: ["p0_basic", "p1_general", "p2_high_accuracy", "p3_kg_strong"],
      defaultModules: ["H_fusion", "L_feedback"],
      rag: {
        defaultPrimary: "H_fusion",
        allowedPrimary: [
          "A_simple",
          "A1_routing",
          "A2_time_aware",
          "B_semantic_chunking",
          "C_context_enriched",
          "D_doc_augmentation",
          "E_query_transform",
          "F_rerank",
          "H_fusion",
          "J_hier",
          "K_kg",
          "L_feedback",
          "O_crag",
        ],
      },
      prerequisites: {
        index: ["index.dense"],
        assets: [],
      },
      ingestionDefaults: {
        chunking: {
          mode: "structured",
          unit: "chars",
          chunkSize: 800,
          overlap: 120,
          separators: ["\\n\\n", "\\n", "。", "！", "？", "；"],
        },
      },
    },
  },
};
